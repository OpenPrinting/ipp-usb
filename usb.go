/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * USB access
 */

package main

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/gousb"
)

var (
	usbCtx = gousb.NewContext()
)

// ----- UsbTransport -----
// Type UsbTransport implements http.RoundTripper over USB
type UsbTransport struct {
	http.Transport               // Underlying http.Transport
	addr           UsbAddr       // Device address
	info           UsbDeviceInfo // USB device info
	log            *Logger       // Device's own logger
	dev            *gousb.Device // Underlying USB device
	rqPending      int32         // Count or pending requests
	rqPendingDone  chan struct{} // Notified when pending request finished
	connInUse      int32         // Count of connections in use
	connections    chan *usbConn // Pool of connections
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
		Transport: http.Transport{
			MaxConnsPerHost:     len(ifaddrs),
			MaxIdleConnsPerHost: len(ifaddrs),
			//IdleConnTimeout:     time.Second,
		},
		addr:          addr,
		log:           NewLogger(),
		dev:           dev,
		rqPendingDone: make(chan struct{}),
		connections:   make(chan *usbConn, len(ifaddrs)),
	}

	transport.fillInfo()
	transport.log.Cc(LogDebug, ColorConsole) // FIXME -- make configurable
	transport.log.ToDevFile(transport.info)

	transport.DialContext = transport.dialContect
	transport.DialTLS = func(network, addr string) (net.Conn, error) {
		return nil, errors.New("No TLS over USB")
	}

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

// usbResponseBodyWrapper wraps http.Response.Body and guarantees
// that connection will be always drained before closed
type usbResponseBodyWrapper struct {
	io.ReadCloser // Underlying http.Response.Body
}

// usbResponseBodyWrapper Close method
func (w *usbResponseBodyWrapper) Close() error {
	go func() {
		io.Copy(ioutil.Discard, w.ReadCloser)
		w.ReadCloser.Close()
	}()

	return nil
}

// RoundTrip executes a single HTTP transaction, returning
// a Response for the provided Request.
func (transport *UsbTransport) RoundTrip(rq *http.Request) (*http.Response, error) {
	// Prevent request from being canceled from outside
	// We cannot do it on USB: closing USB connection
	// doesn't drain buffered data that server is
	// about to send to client
	outreq := rq.Clone(context.Background())
	outreq.Cancel = nil

	atomic.AddInt32(&transport.rqPending, 1)

	resp, err := transport.Transport.RoundTrip(outreq)

	atomic.AddInt32(&transport.rqPending, -1)
	select {
	case transport.rqPendingDone <- struct{}{}:
	default:
	}

	if err == nil {
		resp.Body = &usbResponseBodyWrapper{resp.Body}
	}

	return resp, err
}

// Dial new connection
func (transport *UsbTransport) dialContect(ctx context.Context,
	network, addr string) (net.Conn, error) {

	// Wait for available connection
	select {
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

// ----- usbConn -----
// Type usbConn implements net.Conn over USB
type usbConn struct {
	transport  *UsbTransport      // Transport that owns the connection
	index      int                // Connection index (for logging)
	iface      *gousb.Interface   // Underlying interface
	in         *gousb.InEndpoint  // Input endpoint
	out        *gousb.OutEndpoint // Output endpoint
	closeCtx   context.Context    // Canceled by (*usbConn) Close()
	cancelFunc context.CancelFunc // closeCtx's cancel function
}

var _ = net.Conn(&usbConn{})

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
	ctx := conn.closeCtx

	backoff := time.Millisecond * 100
	for {
		n, err := conn.in.ReadContext(ctx, b)
		if n != 0 || err != nil {
			return n, err
		}
		time.Sleep(backoff)
		backoff *= 2
		if backoff > time.Millisecond*1000 {
			backoff = time.Millisecond * 1000
		}
	}
}

// Write to USB
func (conn *usbConn) Write(b []byte) (n int, err error) {
	return conn.out.WriteContext(conn.closeCtx, b)
}

// Close USB connection
//
// It actually doesn't close a connection, but returns it
// to the pool of available connections
func (conn *usbConn) Close() error {
	transport := conn.transport

	transport.log.Debug(' ', "USB[%d]: connection released, in use: %d",
		conn.index, atomic.AddInt32(&transport.connInUse, -1))

	conn.transport.connections <- conn
	conn.cancelFunc()

	return nil
}

// Destroy USB connection
func (conn *usbConn) destroy() {
	conn.iface.Close()
}

// LocalAddr returns the local network address.
func (conn *usbConn) LocalAddr() net.Addr {
	return nil
}

// RemoteAddr returns the remote network address.
func (conn *usbConn) RemoteAddr() net.Addr {
	return nil
}

// Set read and write deadlines
func (conn *usbConn) SetDeadline(t time.Time) error {
	return nil
}

// Set read deadline
func (conn *usbConn) SetReadDeadline(t time.Time) error {
	return nil
}

// Set write deadline
func (conn *usbConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// Check if device implements IPP over USB
func usbIsIppUsbDevice(desc *gousb.DeviceDesc) bool {
	return len(GetUsbIfAddrs(desc)) >= 2
}
