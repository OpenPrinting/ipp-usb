/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * USB transport for HTTP
 */

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/OpenPrinting/goipp"
)

// UsbTransport implements HTTP transport functionality over USB
type UsbTransport struct {
	addr           UsbAddr       // Device address
	info           UsbDeviceInfo // USB device info
	log            *Logger       // Device's own logger
	dev            *UsbDevHandle // Underlying USB device
	doneHardReset  bool          // True, if done hard reset
	connPool       chan *usbConn // Pool of idle connections
	connList       []*usbConn    // List of all connections
	connReleased   chan struct{} // Signalled when connection released
	shutdown       chan struct{} // Closed by Shutdown()
	connstate      *usbConnState // Connections state tracker
	quirks         *Quirks       // Device quirks
	timeout        time.Duration // Timeout for requests (0 is none)
	timeoutExpired uint32        // Atomic non-zero, if timeout expired
}

// NewUsbTransport creates new http.RoundTripper backed by IPP-over-USB
func NewUsbTransport(desc UsbDeviceDesc) (*UsbTransport, error) {
	// Open the device
	dev, err := UsbOpenDevice(desc)
	if err != nil {
		return nil, err
	}

	// Create UsbTransport
	transport := &UsbTransport{
		addr:         desc.UsbAddr,
		log:          NewLogger(),
		dev:          dev,
		connReleased: make(chan struct{}, 1),
		shutdown:     make(chan struct{}),
	}

	// Setup logging.
	//
	// At this stage, device identification is not yet available,
	// so we cannot redirect logs to the device-specific log file.
	// The logging will remain in buffered mode temporarily.
	//
	// Once device identification becomes available,
	// the log will be redirected to the device's log file.
	//
	// If initialization fails before device identification is obtained,
	// all buffered logs will be flushed to the main log.
	transport.log.Cc(Console)
	transport.log.SetLevels(Conf.LogDevice)

	defer func() {
		if !transport.log.HasDestination() {
			transport.log.Cc(Log)
			transport.log.ToNowhere()
			transport.log.Flush()
		}
	}()

	transport.log.Debug(' ', "===============================")
	transport.log.Info('+', "Found new device. VID:PID = %4.4x:%4.4x",
		desc.Vendor, desc.Product)

	// Obtain quirks by HWID.
	//
	// Do it early, so we can reset the device before querying
	// its UsbDeviceInfo. Some devices are not reliable on
	// returning UsbDeviceInfo before reset.
	quirks := NewQuirks()
	quirks.PullByHWID(Conf.Quirks, desc.Vendor, desc.Product)
	quirks.WriteLog("HWID quirks", transport.log)
	transport.log.Nl(LogDebug)

	if quirks.GetBlacklist() {
		err = ErrBlackListed
		dev.Close()
		return nil, err
	}

	if quirks.GetInitReset() == QuirkResetHard {
		transport.hardReset("init-reset = hard", false)
	}

	// Obtain device info
	transport.info, err = dev.UsbDeviceInfo()

	if err == nil && transport.info.CheckMissed() != nil {
		// Some devices do not always reliably report USB
		// string parameters. If this happens, try to
		// reset the device and re-read them.
		missed := transport.info.CheckMissed()
		if missed != nil {
			transport.hardReset(missed.Error(), true)
			transport.info, err = dev.UsbDeviceInfo()
		}

		if err == nil {
			err = transport.info.CheckMissed()
		}
	}

	if err != nil {
		dev.Close()
		return nil, err
	}

	// Honor mfg and model parameters from the HWID quirks, if present.
	if mfg := quirks.GetMfg(); mfg != "" {
		transport.info.Manufacturer = mfg
	}

	if model := quirks.GetModel(); model != "" {
		transport.info.ProductName = model
	}

	// Load match-by-model quirks
	model := transport.info.MakeAndModel()
	transport.log.Debug(' ', "Loading quirks for model: %q", model)
	quirks.PullByModelName(Conf.Quirks, model)
	transport.quirks = quirks

	transport.quirks.WriteLog("Device quirks", transport.log)
	transport.log.Nl(LogDebug)

	// Write device info to the log
	transport.log.Begin().
		Info('+', "%s: opened %s", transport.addr, transport.info.ProductName).
		Debug(' ', "Device info:").
		Debug(' ', "  USB Port:      %d", transport.info.PortNum).
		Debug(' ', "  Ident:         %s", transport.info.Ident()).
		Debug(' ', "  Manufacturer:  %s", transport.info.Manufacturer).
		Debug(' ', "  Product:       %s", transport.info.ProductName).
		Debug(' ', "  SerialNumber:  %s", transport.info.SerialNumber).
		Debug(' ', "  BasicCaps:     %s", transport.info.BasicCaps).
		Nl(LogDebug).
		Commit()

	transport.dumpUSBparams(transport.log)
	transport.log.Nl(LogDebug)

	transport.log.Debug(' ', "USB interfaces:")
	transport.log.Debug(' ', "  Config Interface Alt Class SubClass Proto")
	for _, ifdesc := range desc.IfDescs {
		prefix := byte(' ')
		if ifdesc.IsIppOverUsb() {
			prefix = '*'
		}

		transport.log.Debug(prefix,
			"     %-3d     %-3d    %-3d %-3d    %-3d     %-3d",
			ifdesc.Config, ifdesc.IfNum,
			ifdesc.Alt, ifdesc.Class, ifdesc.SubClass, ifdesc.Proto)
	}
	transport.log.Nl(LogDebug)

	// Finish with logging initialization
	transport.log.ToDevFile(transport.info)
	transport.log.Flush()

	// We will need this variable a dozen of lines later,
	// but have to declare it now, so we can goto ERROR
	var maxconn uint

	// The 'blacklist' and 'init-reset' quirks were already
	// applied by HWID, but now we have loaded quirks by
	// model name so need to re-check and, possibly, apply.
	//
	// Note, transport.hardReset will prevent us from
	// issuing an unneeded second hard-reset, but if device
	// is blacklisted here but previously reset by the HWID,
	// we cannot prevent that.
	if transport.quirks.GetBlacklist() {
		err = ErrBlackListed
		goto ERROR
	}

	if transport.quirks.GetInitReset() == QuirkResetHard {
		transport.hardReset("init-reset = hard", false)
	}

	// Configure the device
	err = dev.Configure(desc)
	if err != nil {
		goto ERROR
	}

	// Open connections
	maxconn = transport.quirks.GetUsbMaxInterfaces()
	if maxconn == 0 {
		maxconn = math.MaxUint32
	}

	for i, ifaddr := range desc.IfAddrs {
		var conn *usbConn
		conn, err = transport.openUsbConn(i, ifaddr, transport.quirks)
		if err != nil {
			goto ERROR
		}

		transport.connList = append(transport.connList, conn)

		maxconn--
		if maxconn == 0 {
			break
		}
	}

	transport.connPool = make(chan *usbConn, len(transport.connList))
	transport.connstate = newUsbConnState(len(desc.IfAddrs))

	for _, conn := range transport.connList {
		transport.connPool <- conn
	}

	return transport, nil

	// Error: cleanup and exit
ERROR:
	for _, conn := range transport.connList {
		conn.destroy()
	}

	dev.Close()
	return nil, err
}

// hardReset performs device hard reset.
func (transport *UsbTransport) hardReset(reason string, force bool) {
	if !transport.doneHardReset || force {
		transport.log.Debug(' ', "Doing USB HARD RESET: %s", reason)
		transport.dev.Reset()
		transport.doneHardReset = true
	}
}

// Dump USB stack parameters to the UsbTransport's log
func (transport *UsbTransport) dumpUSBparams(log *Logger) {
	const usbParamsDir = "/sys/module/usbcore/parameters"

	// Obtain list of parameter names (file names)
	dir, err := os.Open(usbParamsDir)
	if err != nil {
		return
	}

	files, err := dir.Readdirnames(-1)
	dir.Close()
	if err != nil {
		return
	}

	sort.Strings(files)
	if len(files) == 0 {
		return
	}

	// Compute max width of parameter names
	wid := 0
	for _, file := range files {
		if wid < len(file) {
			wid = len(file)
		}
	}

	wid++

	// Write the table
	log.Debug(' ', "USB stack parameters")

	for _, file := range files {
		p, _ := ioutil.ReadFile(usbParamsDir + "/" + file)
		if p == nil {
			p = []byte("-")
		} else {
			p = bytes.TrimSpace(p)
		}

		log.Debug(' ', "  %*s  %s", -wid, file+":", p)
	}
}

// Get count of connections still in use
func (transport *UsbTransport) connInUse() int {
	return cap(transport.connPool) - len(transport.connPool)
}

// SetTimeout sets the timeout for all subsequent requests.
//
// This is useful only at initialization time and if some requests
// were failed due to timeout, device reset is required, because
// at this case synchronization with device will probably be lost.
//
// A zero value for t means no timeout
func (transport *UsbTransport) SetTimeout(t time.Duration) {
	transport.timeout = t
}

// TimeoutExpired returns true if one or more of the preceding HTTP request
// has failed due to timeout.
func (transport *UsbTransport) TimeoutExpired() bool {
	return atomic.LoadUint32(&transport.timeoutExpired) != 0
}

// closeShutdownChan closes the transport.shutdown, which effectively
// disables connections allocation (usbConnGet will return ErrShutdown)
//
// This function can be safely called multiple times (only the first
// call closes the channel)
//
// Note, this function cannot be called simultaneously from
// different threads. However, it's not a problem, because it
// is only called from (*UsbTransport) Shutdown() and
// (*UsbTransport) Close(), and both of these functions are
// only called from the PnP thread context.
func (transport *UsbTransport) closeShutdownChan() {
	select {
	case <-transport.shutdown:
		// Channel already closed
	default:
		close(transport.shutdown)
	}
}

// Shutdown gracefully shuts down the transport. If provided
// context expires before shutdown completion, Shutdown
// returns the Context's error
func (transport *UsbTransport) Shutdown(ctx context.Context) error {
	transport.closeShutdownChan()

	for {
		n := transport.connInUse()
		if n == 0 {
			break
		}

		transport.log.Info('-', "%s: shutdown: %d connections still in use",
			transport.addr, n)

		select {
		case <-transport.connReleased:
		case <-ctx.Done():
			transport.log.Error('-', "%s: %s: shutdown timeout expired",
				transport.addr, transport.info.ProductName)
			return ctx.Err()
		}
	}

	return nil
}

// Close the transport
func (transport *UsbTransport) Close(reset bool) {
	// Reset the device, if required
	if transport.connInUse() > 0 || reset {
		transport.log.Info('-', "%s: resetting %s",
			transport.addr, transport.info.ProductName)
		transport.dev.Reset()
	}

	// Wait until all connections become inactive
	transport.Shutdown(context.Background())

	// Destroy all connections and close the USB device
	for _, conn := range transport.connList {
		conn.destroy()
	}

	transport.dev.Close()
	transport.log.Info('-', "%s: closed %s",
		transport.addr, transport.info.ProductName)
}

// Log returns device's own logger
func (transport *UsbTransport) Log() *Logger {
	return transport.log
}

// UsbDeviceInfo returns USB device information for the device
// behind the transport
func (transport *UsbTransport) UsbDeviceInfo() UsbDeviceInfo {
	return transport.info
}

// Quirks returns device's quirks
func (transport *UsbTransport) Quirks() *Quirks {
	return transport.quirks
}

// RoundTrip implements http.RoundTripper interface
func (transport *UsbTransport) RoundTrip(r *http.Request) (
	*http.Response, error) {
	session := int(atomic.AddInt32(&httpSessionID, 1)-1) % 1000

	return transport.RoundTripWithSession(session, r)
}

// RoundTripWithSession executes a single HTTP transaction, returning
// a Response for the provided Request. Session number, for logging,
// provided as a separate parameter
func (transport *UsbTransport) RoundTripWithSession(session int,
	rq *http.Request) (*http.Response, error) {

	// Log the request
	transport.log.HTTPRqParams(LogDebug, '>', session, rq)

	// Prevent request from being canceled from outside
	// We cannot do it on USB: closing USB connection
	// doesn't drain buffered data that server is
	// about to send to client
	outreq := rq.WithContext(context.Background())
	outreq.Cancel = nil

	// Remove Expect: 100-continue, if any
	outreq.Header.Del("Expect")

	// Apply quirks
	for name, value := range transport.quirks.HTTPHeaders {
		if value != "" {
			outreq.Header.Set(name, value)
		} else {
			outreq.Header.Del(name)
		}
	}

	// Don't let Go's stdlib to add Connection: close header
	// automatically
	outreq.Close = false

	// Add User-Agent, if missed. It is just cosmetic
	if _, found := outreq.Header["User-Agent"]; !found {
		outreq.Header["User-Agent"] = []string{"ipp-usb"}
	}

	// Wrap request body
	if outreq.Body != nil {
		outreq.Body = &usbRequestBodyWrapper{
			log:     transport.log,
			session: session,
			body:    outreq.Body,
		}
	}

	// Prepare to correctly handle HTTP transaction, in a case
	// client drops request in a middle of reading body
	switch {
	case outreq.ContentLength <= 0:
		// Nothing to do
		if outreq.ContentLength < 0 {
			transport.log.HTTPDebug('>', session,
				"body is chunked, sending as is")
		} else {
			transport.log.HTTPDebug('>', session,
				"body is empty, sending as is")
		}

	case outreq.ContentLength < 16384:
		// Body is small, prefetch it before sending to USB
		buf := &bytes.Buffer{}
		_, err := io.CopyN(buf, outreq.Body, outreq.ContentLength)
		if err != nil {
			return nil, err
		}

		outreq.Body.Close()
		outreq.Body = ioutil.NopCloser(buf)

		transport.log.HTTPDebug('>', session,
			"body is small (%d bytes), prefetched before sending",
			buf.Len())

	default:
		// Force chunked encoding, so if client drops request,
		// we still be able to correctly handle HTTP transaction
		transport.log.HTTPDebug('>', session,
			"body is large (%d bytes), sending as chunked",
			outreq.ContentLength)

		outreq.ContentLength = -1
	}

	// Log request details
	transport.log.Begin().
		HTTPRequest(LogTraceHTTP, '>', session, outreq).
		Commit()

	// Allocate USB connection
	conn, err := transport.usbConnGet(rq.Context())
	if err != nil {
		return nil, err
	}

	transport.log.HTTPDebug(' ', session, "connection %d allocated", conn.index)

	// Make an inter-request (or initial) delay, if needed
	if delay := conn.delayUntil.Sub(time.Now()); delay > 0 {
		transport.log.HTTPDebug(' ', session, "Pausing for %s", delay)
		time.Sleep(delay)
	}

	// Set read/write Context. This effectively sets request timeout.
	//
	// This is important that context is is set after inter-request
	// or initial delay is already done, so we don't need to bother
	// with adjusting the timeout.
	//
	// The context cancel function is called from many places and
	// not always used, so for simplicity I'd better initialize it
	// to the dummy function rather that to compare it with nil
	// every time it is called.
	rwctx := context.Background()
	cleanupCtx := context.CancelFunc(func() {})

	if transport.timeout != 0 {
		rwctx, cleanupCtx = context.WithTimeout(rwctx,
			transport.timeout)
	}

	conn.setRWCtx(rwctx)

	// Send request and receive a response
	err = outreq.Write(conn)
	if err != nil {
		transport.log.HTTPError('!', session, "%s", err)
		conn.put()
		cleanupCtx()
		return nil, err
	}

	resp, err := http.ReadResponse(conn.reader, outreq)
	if err != nil {
		// If the latest conn.Read has returned io.EOF, the only
		// reason it could happen is that the zlp-recv-hack
		// quirk has triggered.
		//
		// The stdlib HTTP stack will wrap the io.EOF error into
		// its own error message. Here we force error condition
		// back to io.EOF so it cleanly can be detected and handled
		// by the initialization retry logic at the upper level
		if conn.EOFSeen() {
			err = io.EOF
		}

		transport.log.HTTPError('!', session, "%s", err)
		conn.put()
		cleanupCtx()
		return nil, err
	}

	// Wrap response body
	resp.Body = &usbResponseBodyWrapper{
		log:        transport.log,
		session:    session,
		body:       resp.Body,
		conn:       conn,
		cleanupCtx: cleanupCtx,
	}

	// Optionally sanitize IPP response
	if transport.quirks.GetBuggyIppRsp() == QuirkBuggyIppRspSanitize &&
		resp.Header.Get("Content-Type") == "application/ipp" {
		transport.sanitizeIppResponse(session, resp)
	}

	// Log the response
	if resp != nil {
		transport.log.Begin().
			HTTPRspStatus(LogDebug, '<', session, outreq, resp).
			HTTPResponse(LogTraceHTTP, '<', session, resp).
			Commit()
	}

	return resp, nil
}

// sanitizeIppResponse attempts to sanitize IPP response from device
func (transport *UsbTransport) sanitizeIppResponse(session int,
	resp *http.Response) {
	// Try to prefetch IPP part of message
	buf := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}

	tee := io.TeeReader(resp.Body, buf)
	msg := goipp.Message{}
	err := msg.DecodeEx(tee, goipp.DecoderOptions{EnableWorkarounds: true})
	if err != nil {
		transport.log.HTTPDebug(' ', session,
			"IPP sanitize: decode: %s", err)
		goto REPLACE
	}

	// If backup copy decodes without any options, no need to sanitize
	if msg2 := (goipp.Message{}); msg2.DecodeBytes(buf.Bytes()) == nil {
		transport.log.HTTPDebug(' ', session,
			"IPP sanitize: not needed")
		goto REPLACE
	}

	// Re-encode the message correctly
	err = msg.Encode(buf2)
	if err != nil {
		transport.log.HTTPDebug(' ', session,
			"IPP sanitize: encode: %s", err)
		goto REPLACE
	}

	// Replace buffer, adjust resp.ContentLength
	if resp.ContentLength != -1 {
		resp.ContentLength += int64(buf2.Len() - buf.Len())

		resp.Header.Set("Content-Length",
			strconv.FormatInt(resp.ContentLength, 10))

		transport.log.HTTPDebug(' ', session,
			"IPP sanitize: %d bytes replaced with %d",
			buf.Len(), buf2.Len())
	}

	buf = buf2

	// Replace consumed part of message with re-coded or
	// saved backup copy
REPLACE:
	wrap := resp.Body.(*usbResponseBodyWrapper)
	wrap.preBody = buf
}

// usbRequestBodyWrapper wraps http.Request.Body, adding
// data path instrumentation
type usbRequestBodyWrapper struct {
	log     *Logger       // Device's logger
	session int           // HTTP session, for logging
	count   int           // Total count of received bytes
	body    io.ReadCloser // Request.body
	drained bool          // EOF or error has been seen
}

// Read from usbRequestBodyWrapper
func (wrap *usbRequestBodyWrapper) Read(buf []byte) (int, error) {
	n, err := wrap.body.Read(buf)
	wrap.count += n

	if err != nil {
		wrap.log.HTTPDebug('>', wrap.session,
			"request body: got %d bytes; %s", wrap.count, err)
		err = io.EOF
		wrap.drained = true
	}

	return n, err
}

// Close usbRequestBodyWrapper
func (wrap *usbRequestBodyWrapper) Close() error {
	if !wrap.drained {
		wrap.log.HTTPDebug('>', wrap.session,
			"request body: got %d bytes; closed", wrap.count)
	}

	return wrap.body.Close()
}

// usbResponseBodyWrapper wraps http.Response.Body and guarantees
// that connection will be always drained before closed
type usbResponseBodyWrapper struct {
	log        *Logger            // Device's logger
	session    int                // HTTP session, for logging
	preBody    *bytes.Buffer      // Data inserted before body, if not nil
	body       io.ReadCloser      // Response.body
	conn       *usbConn           // Underlying USB connection
	count      int                // Total count of received bytes
	drained    bool               // EOF or error has been seen
	cleanupCtx context.CancelFunc // Cancel function for I/O Context
}

// Read from usbResponseBodyWrapper
func (wrap *usbResponseBodyWrapper) Read(buf []byte) (int, error) {
	if wrap.preBody != nil && wrap.preBody.Len() > 0 {
		return wrap.preBody.Read(buf)
	}

	n, err := wrap.body.Read(buf)
	wrap.count += n

	if err != nil {
		wrap.log.HTTPDebug('<', wrap.session,
			"response body: got %d bytes; %s", wrap.count, err)
		wrap.drained = true
	}
	return n, err
}

// Close usbResponseBodyWrapper
func (wrap *usbResponseBodyWrapper) Close() error {
	// If EOF or error seen, we can close synchronously
	if wrap.drained {
		wrap.cleanup()
		return nil
	}

	// Otherwise, we need to drain USB connection
	wrap.log.HTTPDebug('<', wrap.session, "client has gone; draining response from USB")
	go func() {
		defer func() {
			v := recover()
			if v != nil {
				Log.Panic(v)
			}
		}()

		io.Copy(ioutil.Discard, wrap.body)
		wrap.cleanup()
	}()

	return nil
}

// cleanup performs the final cleanup of the usbResponseBodyWrapper
// after use.
func (wrap *usbResponseBodyWrapper) cleanup() {
	wrap.body.Close()
	wrap.conn.put()

	// Cleanup I/O context.Context, if any
	if wrap.cleanupCtx != nil {
		wrap.cleanupCtx()
	}

	wrap.log.HTTPDebug('<', wrap.session, "done with response body")
}

// usbConn implements an USB connection
type usbConn struct {
	transport     *UsbTransport   // Transport that owns the connection
	index         int             // Connection index (for logging)
	iface         *UsbInterface   // Underlying interface
	reader        *bufio.Reader   // For http.ReadResponse
	rwctx         context.Context // For usbConn.Read and usbConn.Write
	delayUntil    time.Time       // Delay till this time before next request
	delayInterval time.Duration   // Pause between requests
	cntRecv       int             // Total bytes received
	cntSent       int             // Total bytes sent
	eofSeen       bool            // Last usbConn.Read has returned io.EOF
}

// Open usbConn
func (transport *UsbTransport) openUsbConn(
	index int, ifaddr UsbIfAddr, quirks *Quirks) (*usbConn, error) {

	dev := transport.dev

	transport.log.Debug(' ', "USB[%d]: open: %s", index, ifaddr)

	// Initialize connection structure
	conn := &usbConn{
		transport:     transport,
		index:         index,
		delayUntil:    time.Now().Add(quirks.GetInitDelay()),
		delayInterval: quirks.GetRequestDelay(),
	}

	conn.reader = bufio.NewReader(conn)

	// Obtain interface
	var err error
	conn.iface, err = dev.OpenUsbInterface(ifaddr, quirks)
	if err != nil {
		goto ERROR
	}

	// Soft-reset interface, if needed
	if quirks.GetInitReset() == QuirkResetSoft {
		transport.log.Debug(' ', "USB[%d]: doing SOFT_RESET", index)
		err = conn.iface.SoftReset()
		if err != nil {
			// Don't treat it too seriously
			transport.log.Info('?', "USB[%d]: SOFT_RESET: %s", index, err)
		}
	}

	return conn, nil

	// Error: cleanup and exit
ERROR:
	transport.log.Error('!', "USB[%d]: %s", index, err)
	if conn.iface != nil {
		conn.iface.Close()
	}

	return nil, err
}

// setRWCtx sets context.Context for subsequent Read and Write operations
func (conn *usbConn) setRWCtx(ctx context.Context) {
	conn.rwctx = ctx
}

// Read from USB
func (conn *usbConn) Read(b []byte) (int, error) {
	conn.transport.connstate.beginRead(conn)
	defer conn.transport.connstate.doneRead(conn)

	// Drop conn.eofSeenn flag
	conn.eofSeen = false

	// Note, to avoid LIBUSB_TRANSFER_OVERFLOW errors
	// from libusb, input buffer size must always
	// be aligned by 1024 bytes for USB 3.0, 512 bytes
	// for USB 2.0, so 1024 bytes alignment is safe for
	// both
	//
	// However if caller requests less that 1024 bytes, we
	// can't align here simply by shrinking the buffer,
	// because it will result a zero-size buffer. At
	// this case we assume caller knows what it is
	// doing (actually bufio never behaves this way)
	if n := len(b); n >= 1024 {
		n &= ^1023
		b = b[0:n]
	}

	// zlp-recv-hack handling
	zlpRecvHack := conn.transport.quirks.GetZlpRecvHack()
	zlpRecv := false

	// Setup deadline
	backoff := time.Millisecond * 10
	for {
		n, err := conn.iface.Recv(conn.rwctx, b)
		conn.cntRecv += n

		conn.transport.log.Add(LogTraceHTTP, '<',
			"USB[%d]: read: wanted %d got %d total %d",
			conn.index, len(b), n, conn.cntRecv)

		conn.transport.log.HexDump(LogTraceUSB, '<', b[:n])

		if err != nil {
			conn.transport.log.Error('!',
				"USB[%d]: recv: %s", conn.index, err)

			if err == context.DeadlineExceeded {
				// If we've got read timeout preceded
				// by the zero-length packet, interpret
				// is as body EOF condition
				if zlpRecvHack && zlpRecv {
					conn.eofSeen = true
					return 0, io.EOF
				}

				atomic.StoreUint32(
					&conn.transport.timeoutExpired, 1)
			}
		}

		if n != 0 || err != nil {
			return n, err
		}

		zlpRecv = true
		conn.transport.log.Debug(' ',
			"USB[%d]: zero-size read", conn.index)

		time.Sleep(backoff)
		backoff += backoff / 4 // The same as backoff *= 1.25
		if backoff > time.Millisecond*1000 {
			backoff = time.Millisecond * 1000
		}
	}
}

// Write to USB
func (conn *usbConn) Write(b []byte) (int, error) {
	conn.transport.connstate.beginWrite(conn)
	defer conn.transport.connstate.doneWrite(conn)

	n, err := conn.iface.Send(conn.rwctx, b)
	conn.cntSent += n

	conn.transport.log.Add(LogTraceHTTP, '>',
		"USB[%d]: write: wanted %d sent %d total %d",
		conn.index, len(b), n, conn.cntSent)

	conn.transport.log.HexDump(LogTraceUSB, '>', b[:n])

	if err != nil {
		conn.transport.log.Error('!',
			"USB[%d]: send: %s", conn.index, err)

		if err == context.DeadlineExceeded {
			atomic.StoreUint32(
				&conn.transport.timeoutExpired, 1)
		}
	}

	return n, err
}

// EOFSeen reports of the latest usbConn.Read has returned io.EOF
func (conn *usbConn) EOFSeen() bool {
	return conn.eofSeen
}

// Allocate a connection
func (transport *UsbTransport) usbConnGet(ctx context.Context) (*usbConn, error) {
	select {
	case <-transport.shutdown:
		return nil, ErrShutdown
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn := <-transport.connPool:
		transport.connstate.gotConn(conn)
		transport.log.Debug(' ', "USB[%d]: connection allocated, %s",
			conn.index, transport.connstate)

		return conn, nil
	}
}

// Release the connection
func (conn *usbConn) put() {
	transport := conn.transport

	conn.reader.Reset(conn)
	conn.delayUntil = time.Now().Add(conn.delayInterval)
	conn.cntRecv = 0
	conn.cntSent = 0

	transport.connstate.putConn(conn)
	transport.log.Debug(' ', "USB[%d]: connection released, %s",
		conn.index, transport.connstate)

	transport.connPool <- conn

	select {
	case transport.connReleased <- struct{}{}:
	default:
	}
}

// Destroy USB connection
func (conn *usbConn) destroy() {
	conn.transport.log.Debug(' ', "USB[%d]: closed", conn.index)
	conn.iface.Close()
}

// usbConnState tracks connections state, for logging
type usbConnState struct {
	alloc []int32 // Per-connection "allocated" flag
	read  []int32 // Per-connection "reading" flag
	write []int32 // Per-connection "writing" flag
}

// newUsbConnState creates a new usbConnState for given
// number of connections
func newUsbConnState(cnt int) *usbConnState {
	return &usbConnState{
		alloc: make([]int32, cnt),
		read:  make([]int32, cnt),
		write: make([]int32, cnt),
	}
}

// gotConn notifies usbConnState, that connection is allocated
func (state *usbConnState) gotConn(conn *usbConn) {
	atomic.AddInt32(&state.alloc[conn.index], 1)
}

// putConn notifies usbConnState, that connection is released
func (state *usbConnState) putConn(conn *usbConn) {
	atomic.AddInt32(&state.alloc[conn.index], -1)
}

// beginRead notifies usbConnState, that read is started
func (state *usbConnState) beginRead(conn *usbConn) {
	atomic.AddInt32(&state.read[conn.index], 1)
}

// doneRead notifies usbConnState, that read is done
func (state *usbConnState) doneRead(conn *usbConn) {
	atomic.AddInt32(&state.read[conn.index], -1)
}

// beginWrite notifies usbConnState, that write is started
func (state *usbConnState) beginWrite(conn *usbConn) {
	atomic.AddInt32(&state.write[conn.index], 1)
}

// doneWrite notifies usbConnState, that write is done
func (state *usbConnState) doneWrite(conn *usbConn) {
	atomic.AddInt32(&state.write[conn.index], -1)
}

// String returns a string, representing connections state
func (state *usbConnState) String() string {
	buf := make([]byte, 0, 64)
	used := 0

	for i := range state.alloc {
		a := atomic.LoadInt32(&state.alloc[i])
		r := atomic.LoadInt32(&state.read[i])
		w := atomic.LoadInt32(&state.write[i])

		if len(buf) != 0 {
			buf = append(buf, ' ')
		}

		if a|r|w == 0 {
			buf = append(buf, '-', '-', '-')
		} else {
			used++

			if a != 0 {
				buf = append(buf, 'a')
			} else {
				buf = append(buf, '-')
			}

			if r != 0 {
				buf = append(buf, 'r')
			} else {
				buf = append(buf, '-')
			}

			if w != 0 {
				buf = append(buf, 'w')
			} else {
				buf = append(buf, '-')
			}
		}
	}

	return fmt.Sprintf("%d in use: %s", used, buf)
}
