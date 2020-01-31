/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * USB access
 */

package main

import (
	"bufio"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/gousb"
)

var (
	usbCtx = gousb.NewContext()
)

// ----- UsbTransport -----
// UsbTransport implements HTTP transport functionality over USB
type UsbTransport struct {
	addr          UsbAddr       // Device address
	info          UsbDeviceInfo // USB device info
	log           *Logger       // Device's own logger
	dev           *gousb.Device // Underlying USB device
	rqPending     int32         // Count or pending requests
	rqPendingDone chan struct{} // Notified when pending request finished
	connInUse     int32         // Count of connections in use
	connections   chan *usbConn // Pool of connections
	shutdown      chan struct{} // Closed by Shutdown()
}

// Type UsbDeviceInfo represents USB device information
type UsbDeviceInfo struct {
	Vendor       gousb.ID
	SerialNumber string
	Manufacturer string
	Product      string
	DeviceId     string
}

// Ident returns device identification string, suitable as
// persistent state identifier
func (info UsbDeviceInfo) Ident() string {
	id := info.Vendor.String() + "-" + info.SerialNumber + "-" + info.Product
	id = strings.Map(func(c rune) rune {
		switch {
		case '0' <= c && c <= '9':
		case 'a' <= c && c <= 'z':
		case 'A' <= c && c <= 'Z':
		case c == '-' || c == '_':
		default:
			c = '-'
		}
		return c
	}, id)
	return id
}

// Comment returns a short comment, describing a device
func (info UsbDeviceInfo) Comment() string {
	c := ""

	if !strings.HasPrefix(info.Product, info.Manufacturer) {
		c += info.Manufacturer + " " + info.Product
	} else {
		c = info.Product
	}

	c += " serial=" + info.SerialNumber

	return c
}

// Fetch IEEE 1284.4 DEVICE_ID
func usbGetDeviceId(dev *gousb.Device) string {
	buf := make([]byte, 2048)

	for cfgNum, conf := range dev.Desc.Configs {
		for ifNum, iface := range conf.Interfaces {
			for altNum, alt := range iface.AltSettings {
				if alt.Class == gousb.ClassPrinter &&
					alt.SubClass == 1 {

					n, err := dev.Control(
						gousb.ControlClass|gousb.ControlIn|gousb.ControlInterface,
						0,
						uint16(cfgNum),
						uint16((ifNum<<8)|altNum),
						buf,
					)

					if err == nil && n >= 2 {
						buf2 := make([]byte, n-2)
						copy(buf2, buf[2:n])
						return string(buf2)
					}

				}
			}
		}
	}

	return ""
}

// Create new http.RoundTripper backed by IPP-over-USB
func NewUsbTransport(addr UsbAddr) (*UsbTransport, error) {
	// Open the device
	dev, err := addr.Open()
	if err != nil {
		return nil, err
	}

	// Create UsbTransport
	ifaddrs := GetUsbIfAddrs(dev.Desc)

	transport := &UsbTransport{
		addr:          addr,
		log:           NewLogger(),
		dev:           dev,
		rqPendingDone: make(chan struct{}),
		connections:   make(chan *usbConn, len(ifaddrs)),
		shutdown:      make(chan struct{}),
	}

	transport.fillInfo()
	transport.log.Cc(LogDebug, ColorConsole) // FIXME -- make configurable
	transport.log.ToDevFile(transport.info)

	// Write device info to the log
	transport.log.Begin().
		Debug(' ', "===============================").
		Info('+', "%s: added %s", addr, transport.info.Product).
		Debug(' ', "Device info:").
		Debug(' ', "  Ident:        %s", transport.info.Ident()).
		Debug(' ', "  Manufacturer: %s", transport.info.Manufacturer).
		Debug(' ', "  Product:      %s", transport.info.Product).
		Debug(' ', "  DeviceId:     %s", transport.info.DeviceId).
		Commit()

	// Open connections
	for i, ifaddr := range ifaddrs {
		var conn *usbConn
		conn, err = transport.openUsbConn(i, ifaddr)
		if err != nil {
			goto ERROR
		}
		transport.connections <- conn
	}

	return transport, nil

	// Error: cleanup and exit
ERROR:
	for conn := range transport.connections {
		conn.destroy()
	}

	dev.Close()
	return nil, err
}

// Shutdown gracefully shuts down the transport. If provided
// context expires before shutdown completion, Shutdown
// returns the Context's error
func (transport *UsbTransport) Shutdown(ctx context.Context) error {
	close(transport.shutdown)

	for {
		cnt := atomic.LoadInt32(&transport.rqPending)
		if cnt == 0 {
			break
		}

		transport.log.Info('-', "%s: shutdown: %d requests still pending",
			transport.addr, cnt)

		select {
		case <-transport.rqPendingDone:
		case <-ctx.Done():
			transport.log.Error('-', "%s: %s: shutdown timeout expired",
				transport.addr, transport.info.Product)
			return ctx.Err()
		}
	}

	return nil
}

// Close the transport
func (transport *UsbTransport) Close() {
	if atomic.LoadInt32(&transport.rqPending) > 0 {
		transport.log.Info('-', "%s: resetting %s", transport.addr, transport.info.Product)
		transport.dev.Reset()
	}

	transport.dev.Close()
	transport.log.Info('-', "%s: removed %s", transport.addr, transport.info.Product)
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

// fillUsbDeviceInfo fills transport.info
func (transport *UsbTransport) fillInfo() {
	dev := transport.dev

	ok := func(s string, err error) string {
		if err == nil {
			return s
		} else {
			return ""
		}
	}

	transport.info = UsbDeviceInfo{
		Vendor:       dev.Desc.Vendor,
		SerialNumber: ok(dev.SerialNumber()),
		Manufacturer: ok(dev.Manufacturer()),
		Product:      ok(dev.Product()),
		DeviceId:     usbGetDeviceId(dev),
	}
}

// RoundTrip executes a single HTTP transaction, returning
// a Response for the provided Request.
func (transport *UsbTransport) RoundTripSession(session int, rq *http.Request) (*http.Response, error) {
	transport.rqPendingInc()
	defer transport.rqPendingDec()

	conn, err := transport.usbConnGet(rq.Context())
	if err != nil {
		return nil, err
	}

	transport.log.HttpDebug(' ', session, "connection %d allocated", conn.index)

	// Prevent request from being canceled from outside
	// We cannot do it on USB: closing USB connection
	// doesn't drain buffered data that server is
	// about to send to client
	outreq := rq.Clone(context.Background())
	outreq.Cancel = nil

	// Remove Expect: 100-continue, if any
	outreq.Header.Del("Expect")

	// Wrap request body
	if outreq.Body != nil {
		outreq.Body = &usbRequestBodyWrapper{
			log:     transport.log,
			session: session,
			body:    outreq.Body,
		}
	}

	// Send request and receive a response
	err = outreq.Write(conn)
	if err != nil {
		return nil, err
	}

	resp, err := http.ReadResponse(conn.reader, outreq)
	if resp != nil {
		resp.Body = &usbResponseBodyWrapper{
			log:     transport.log,
			session: session,
			body:    resp.Body,
			conn:    conn,
		}
	}

	return resp, err
}

// rqPendingInc atomically increments transport.rqPending
func (transport *UsbTransport) rqPendingInc() {
	atomic.AddInt32(&transport.rqPending, 1)
}

// rqPendingDec atomically decrements transport.rqPending
// and wakes sleeping Shutdown thread, if any
func (transport *UsbTransport) rqPendingDec() {
	atomic.AddInt32(&transport.rqPending, -1)
	select {
	case transport.rqPendingDone <- struct{}{}:
	default:
	}
}

// usbRequestBodyWrapper wraps http.Request.Body, adding
// data path instrumentation
type usbRequestBodyWrapper struct {
	log     *Logger       // Device's logger
	session int           // HTTP session, for logging
	body    io.ReadCloser // Request.body
}

// Read from usbRequestBodyWrapper
func (wrap *usbRequestBodyWrapper) Read(buf []byte) (int, error) {
	n, err := wrap.body.Read(buf)
	if err != nil {
		wrap.log.HttpDebug('>', wrap.session, "request body read: %s", err)
		err = io.EOF
	}
	return n, err
}

// Close usbRequestBodyWrapper
func (wrap *usbRequestBodyWrapper) Close() error {
	return wrap.body.Close()
}

// usbResponseBodyWrapper wraps http.Response.Body and guarantees
// that connection will be always drained before closed
type usbResponseBodyWrapper struct {
	log     *Logger       // Device's logger
	session int           // HTTP session, for logging
	body    io.ReadCloser // Response.body
	conn    *usbConn      // Underlying USB connection
	drained bool          // EOF or error has been seen
}

// Read from usbResponseBodyWrapper
func (wrap *usbResponseBodyWrapper) Read(buf []byte) (int, error) {
	n, err := wrap.body.Read(buf)
	if err != nil {
		wrap.log.HttpDebug('<', wrap.session, "response body read: %s", err)
		wrap.drained = true
	}
	return n, err
}

// Close usbResponseBodyWrapper
func (wrap *usbResponseBodyWrapper) Close() error {
	// If EOF or error seen, we can close synchronously
	if wrap.drained {
		wrap.body.Close()
		wrap.conn.put()
		return nil
	}

	// Otherwise, we need to drain USB connection
	wrap.log.HttpDebug('<', wrap.session, "client has gone; draining response from USB")
	go func() {
		io.Copy(ioutil.Discard, wrap.body)
		wrap.body.Close()
		wrap.conn.put()
	}()

	return nil
}

// ----- usbConn -----
// usbConn implements an USB connection
type usbConn struct {
	transport  *UsbTransport      // Transport that owns the connection
	index      int                // Connection index (for logging)
	iface      *gousb.Interface   // Underlying interface
	in         *gousb.InEndpoint  // Input endpoint
	out        *gousb.OutEndpoint // Output endpoint
	closeCtx   context.Context    // Canceled by (*usbConn) put()
	cancelFunc context.CancelFunc // closeCtx's cancel function
	doneIO     sync.WaitGroup     // put() waits for Read/Write cancelation
	reader     *bufio.Reader      // For http.ReadResponse
}

// Open usbConn
func (transport *UsbTransport) openUsbConn(
	index int, ifaddr UsbIfAddr) (*usbConn, error) {

	dev := transport.dev

	transport.log.Debug(' ', "USB[%d]: open: %s", index, ifaddr)

	// Initialize connection structure
	conn := &usbConn{
		transport: transport,
		index:     index,
	}

	conn.reader = bufio.NewReader(conn)

	// Obtain interface
	var err error
	conn.iface, err = ifaddr.Open(dev)
	if err != nil {
		goto ERROR
	}

	// Obtain endpoints
	conn.in, err = conn.iface.InEndpoint(ifaddr.In.Number)
	if err != nil {
		goto ERROR
	}

	conn.out, err = conn.iface.OutEndpoint(ifaddr.Out.Number)
	if err != nil {
		goto ERROR
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

// Read from USB
func (conn *usbConn) Read(b []byte) (n int, err error) {
	// Note, to avoid LIBUSB_TRANSFER_OVERFLOW erros
	// from libusb, input buffer size must always
	// be aligned by 512 bytes
	//
	// However if caller requests less that 512 bytes, we
	// can't align here simply by shrinking the buffer,
	// because it will result a zero-size buffer. At
	// this case we assume caller knows what it
	// doing (actually bufio never behaves this way)
	if n := len(b); n >= 512 {
		n &= ^511
		b = b[0:n]
	}

	conn.doneIO.Add(1)
	defer conn.doneIO.Done()

	ctx := conn.closeCtx

	backoff := time.Millisecond * 100
	for {
		n, err := conn.in.ReadContext(ctx, b)
		if n != 0 || err != nil {
			return n, err
		}
		conn.transport.log.Error(' ', "USB[%d]: zero-size read")

		time.Sleep(backoff)
		backoff *= 2
		if backoff > time.Millisecond*1000 {
			backoff = time.Millisecond * 1000
		}
	}
}

// Write to USB
func (conn *usbConn) Write(b []byte) (n int, err error) {
	conn.doneIO.Add(1)
	defer conn.doneIO.Done()

	return conn.out.WriteContext(conn.closeCtx, b)
}

// Allocate a connection
func (transport *UsbTransport) usbConnGet(ctx context.Context) (*usbConn, error) {
	select {
	case <-transport.shutdown:
		return nil, ErrShutdown
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn := <-transport.connections:
		transport.log.Debug(' ', "USB[%d]: connection allocated, in use: %d",
			conn.index, atomic.AddInt32(&transport.connInUse, 1))

		conn.closeCtx, conn.cancelFunc =
			context.WithCancel(context.Background())

		return conn, nil
	}
}

// Release the connection
func (conn *usbConn) put() error {
	transport := conn.transport

	conn.cancelFunc()
	conn.doneIO.Wait()
	conn.reader.Reset(conn)

	transport.log.Debug(' ', "USB[%d]: connection released, in use: %d",
		conn.index, atomic.AddInt32(&transport.connInUse, -1))

	conn.transport.connections <- conn

	return nil
}

// Destroy USB connection
func (conn *usbConn) destroy() {
	conn.iface.Close()
}
