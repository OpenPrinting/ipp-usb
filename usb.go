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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"time"
)

// UsbTransport implements HTTP transport functionality over USB
type UsbTransport struct {
	addr          UsbAddr       // Device address
	info          UsbDeviceInfo // USB device info
	log           *Logger       // Device's own logger
	dev           *UsbDevHandle // Underlying USB device
	rqPending     int32         // Count or pending requests
	rqPendingDone chan struct{} // Notified when pending request finished
	connections   chan *usbConn // Pool of connections
	shutdown      chan struct{} // Closed by Shutdown()
	connstate     *usbConnState // Connections state tracker
}

// Create new http.RoundTripper backed by IPP-over-USB
func NewUsbTransport(desc UsbDeviceDesc) (*UsbTransport, error) {
	// Open the device
	dev, err := UsbOpenDevice(desc)
	if err != nil {
		return nil, err
	}

	// Create UsbTransport
	transport := &UsbTransport{
		addr:          desc.UsbAddr,
		log:           NewLogger(),
		dev:           dev,
		rqPendingDone: make(chan struct{}),
		connections:   make(chan *usbConn, len(desc.IfAddrs)),
		shutdown:      make(chan struct{}),
		connstate:     newUsbConnState(len(desc.IfAddrs)),
	}

	// Obtain device info
	transport.info, err = dev.UsbDeviceInfo()
	if err != nil {
		dev.Close()
		return nil, err
	}

	transport.log.Cc(LogDebug, ColorConsole) // FIXME -- make configurable
	transport.log.ToDevFile(transport.info)

	// Write device info to the log
	transport.log.Begin().
		Debug(' ', "===============================").
		Info('+', "%s: added %s", transport.addr, transport.info.ProductName).
		Debug(' ', "Device info:").
		Debug(' ', "  Ident:        %s", transport.info.Ident()).
		Debug(' ', "  Manufacturer: %s", transport.info.Manufacturer).
		Debug(' ', "  Product:      %s", transport.info.ProductName).
		Nl(LogDebug).
		Commit()

	transport.log.Debug(' ', "USB interfaces:")
	transport.log.Debug(' ', "  Config Interface Alt Class Proto")
	for _, ifdesc := range desc.IfDescs {
		transport.log.Debug(' ', "     %-3d     %-3d    %-3d %-3d   %-3d",
			ifdesc.Config, ifdesc.IfNum,
			ifdesc.Alt, ifdesc.Class, ifdesc.Proto)
	}
	transport.log.Nl(LogDebug)

	// Open connections
	for i, ifaddr := range desc.IfAddrs {
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
				transport.addr, transport.info.ProductName)
			return ctx.Err()
		}
	}

	return nil
}

// Close the transport
func (transport *UsbTransport) Close() {
	if atomic.LoadInt32(&transport.rqPending) > 0 {
		transport.log.Info('-', "%s: resetting %s",
			transport.addr, transport.info.ProductName)
		transport.dev.Reset()
	}

	transport.dev.Close()
	transport.log.Info('-', "%s: removed %s",
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

	outreq.Header.Set("Connection", "close")
	outreq.Close = true

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

// usbConn implements an USB connection
type usbConn struct {
	transport *UsbTransport // Transport that owns the connection
	index     int           // Connection index (for logging)
	iface     *UsbInterface // Underlying interface
	reader    *bufio.Reader // For http.ReadResponse
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
	conn.iface, err = dev.OpenUsbInterface(ifaddr)
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
	conn.transport.connstate.beginRead(conn)
	defer conn.transport.connstate.doneRead(conn)

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

	backoff := time.Millisecond * 100
	for {
		n, err := conn.iface.Recv(b, 0)
		if err != nil {
			conn.transport.log.Error('!',
				"USB[%d]: recv: %s", conn.index, err)
		}

		if n != 0 || err != nil {
			return n, err
		}
		conn.transport.log.Error('!',
			"USB[%d]: zero-size read", conn.index)

		time.Sleep(backoff)
		backoff *= 2
		if backoff > time.Millisecond*1000 {
			backoff = time.Millisecond * 1000
		}
	}
}

// Write to USB
func (conn *usbConn) Write(b []byte) (n int, err error) {
	conn.transport.connstate.beginWrite(conn)
	defer conn.transport.connstate.doneWrite(conn)

	n, err = conn.iface.Send(b, 0)
	if err != nil {
		conn.transport.log.Error('!',
			"USB[%d]: send: %s", conn.index, err)
	}
	return
}

// Allocate a connection
func (transport *UsbTransport) usbConnGet(ctx context.Context) (*usbConn, error) {
	select {
	case <-transport.shutdown:
		return nil, ErrShutdown
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn := <-transport.connections:
		transport.connstate.gotConn(conn)
		transport.log.Debug(' ', "USB[%d]: connection allocated, %s",
			conn.index, transport.connstate)

		return conn, nil
	}
}

// Release the connection
func (conn *usbConn) put() error {
	transport := conn.transport

	conn.reader.Reset(conn)

	transport.connstate.putConn(conn)
	transport.log.Debug(' ', "USB[%d]: connection released, %s",
		conn.index, transport.connstate)

	conn.transport.connections <- conn

	return nil
}

// Destroy USB connection
func (conn *usbConn) destroy() {
	conn.iface.Close()
}

// usbConnState tracks connections state, for logging
type usbConnState struct {
	alloc []int32
	read  []int32
	write []int32
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
