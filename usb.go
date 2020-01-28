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
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
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
	ifaddrs        []*usbIfAddr  // IPP interfaces
	dialSem        chan struct{} // Counts available connections
	dialLock       sync.Mutex    // Protects access to ifaddrs
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
	ifaddrs := usbGetIppIfAddrs(dev.Desc)

	transport := &UsbTransport{
		Transport: http.Transport{
			MaxConnsPerHost:     len(ifaddrs),
			MaxIdleConnsPerHost: len(ifaddrs),
		},
		addr:    addr,
		log:     NewLogger(),
		dev:     dev,
		ifaddrs: ifaddrs,
		dialSem: make(chan struct{}, len(ifaddrs)),
	}

	transport.fillInfo()
	transport.log.Cc(LogDebug, ColorConsole) // FIXME -- make configurable
	transport.log.ToDevFile(transport.info)

	transport.DialContext = transport.dialContect
	transport.DialTLS = func(network, addr string) (net.Conn, error) {
		return nil, errors.New("No TLS over USB")
	}

	for i := 0; i < len(ifaddrs); i++ {
		transport.dialSem <- struct{}{}
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

	transport.log.Debug(' ', "IPP-USB Interfaces:")
	for _, ifaddr := range transport.ifaddrs {
		transport.log.Debug(' ', "  %s", ifaddr)
	}

	return transport, nil
}

// Close the transport
func (transport *UsbTransport) Close() {
	transport.log.Info('-', "%s: removed %s", transport.addr, transport.info.Product)
	// FIXME
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

	resp, err := transport.Transport.RoundTrip(outreq)
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
	case <-transport.dialSem:
	}

	// Acquire a connection
	transport.dialLock.Lock()
	defer transport.dialLock.Unlock()

	for _, ifaddr := range transport.ifaddrs {
		if !ifaddr.Busy {
			conn, err := openUsbConn(transport, ifaddr)
			if err != nil {
				transport.dialSem <- struct{}{}
			}
			return conn, err
		}
	}

	panic("internal error")
}

// ----- usbConn -----
// Type usbConn implements net.Conn over USB
type usbConn struct {
	transport *UsbTransport      // Transport that owns the connection
	ifaddr    *usbIfAddr         // Interface address
	iface     *gousb.Interface   // Underlying interface
	in        *gousb.InEndpoint  // Input endpoint
	out       *gousb.OutEndpoint // Output endpoint
}

var _ = net.Conn(&usbConn{})

// Open usbConn
func openUsbConn(transport *UsbTransport, ifaddr *usbIfAddr) (*usbConn, error) {
	dev := transport.dev

	transport.log.Debug('+', "USB OPEN: %s", ifaddr)

	// Obtain interface
	iface, err := ifaddr.Interface(dev)
	if err != nil {
		transport.log.Error('!', "USB ERROR: %s", err)
		return nil, err
	}

	// Initialize connection structure
	conn := &usbConn{
		transport: transport,
		ifaddr:    ifaddr,
		iface:     iface,
	}

	// Obtain endpoints
	for _, ep := range iface.Setting.Endpoints {
		switch {
		case ep.Direction == gousb.EndpointDirectionIn && conn.in == nil:
			conn.in, err = iface.InEndpoint(ep.Number)
		case ep.Direction == gousb.EndpointDirectionOut && conn.out == nil:
			conn.out, err = iface.OutEndpoint(ep.Number)
		}

		if err != nil {
			transport.log.Error('!', "USB ERROR: %s", err)
			break
		}
	}

	if err == nil && (conn.in == nil || conn.out == nil) {
		err = errors.New("Missed input or output endpoint")
	}

	if err != nil {
		conn.Close()
		transport.log.Error('!', "USB ERROR: %s", err)
		return nil, err
	}

	return conn, nil
}

// Read from USB
func (conn *usbConn) Read(b []byte) (n int, err error) {
	backoff := time.Millisecond * 100
	for {
		n, err := conn.in.Read(b)
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
	return conn.out.Write(b)
}

// Close USB connection
func (conn *usbConn) Close() error {
	conn.transport.log.Debug('-', "USB CLOSE: %s", conn.ifaddr)

	conn.iface.Close()
	conn.ifaddr.Busy = false
	conn.transport.dialSem <- struct{}{}
	return nil
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

// ----- usbIfAddr -----
// Type usbIfAddr represents a full interface "address" within device
type usbIfAddr struct {
	Busy    bool              // Address is in use
	DevDesc *gousb.DeviceDesc // Put it here for easy access
	CfgNum  int               // Config number within device
	Num     int               // Interface number within Config
	Alt     int               // Number of alternate setting
}

// String represents a human readable short representation of usbIfAddr
func (ifaddr *usbIfAddr) String() string {
	return fmt.Sprintf("Bus %.3d Device %.3d Config %d Interface %d Alt %d",
		ifaddr.DevDesc.Bus,
		ifaddr.DevDesc.Address,
		ifaddr.CfgNum,
		ifaddr.Num,
		ifaddr.Alt,
	)
}

// Open the particular interface on device. Marks address as busy
func (ifaddr *usbIfAddr) Interface(dev *gousb.Device) (*gousb.Interface, error) {
	if ifaddr.Busy {
		panic("internal error")
	}

	conf, err := dev.Config(ifaddr.CfgNum)
	if err != nil {
		return nil, err
	}

	iface, err := conf.Interface(ifaddr.Num, ifaddr.Alt)
	if err != nil {
		return nil, err
	}

	ifaddr.Busy = true
	return iface, nil
}

// Collect IPP over USB interfaces on device
func usbGetIppIfAddrs(desc *gousb.DeviceDesc) []*usbIfAddr {
	var ifaddrs []*usbIfAddr

	for cfgNum, conf := range desc.Configs {
		for ifNum, iface := range conf.Interfaces {
			for altNum, alt := range iface.AltSettings {
				if alt.Class == gousb.ClassPrinter &&
					alt.SubClass == 1 &&
					alt.Protocol == 4 {
					addr := &usbIfAddr{
						DevDesc: desc,
						CfgNum:  cfgNum,
						Num:     ifNum,
						Alt:     altNum,
					}

					ifaddrs = append(ifaddrs, addr)
				}
			}
		}
	}

	return ifaddrs
}

// Check if device implements IPP over USB
func usbIsIppUsbDevice(desc *gousb.DeviceDesc) bool {
	return len(usbGetIppIfAddrs(desc)) >= 2
}
