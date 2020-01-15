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
	"sync"
	"time"

	"github.com/google/gousb"
)

var (
	usbCtx = gousb.NewContext()
)

// UsbAddr represents an USB device address
type UsbAddr struct {
	Bus     int // The bus on which the device was detected
	Address int // The address of the device on the bus
}

// String returns a human-readable representation of UsbAddr
func (addr UsbAddr) String() string {
	return fmt.Sprintf("Bus %.3d Device %.3d", addr.Bus, addr.Address)
}

// Open device by address
func (addr UsbAddr) Open() (*gousb.Device, error) {
	found := false
	devs, err := usbCtx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		if found {
			return false
		}

		return addr.Bus == desc.Bus && addr.Address == desc.Address
	})

	if len(devs) != 0 {
		return devs[0], nil
	}

	if err == nil {
		err = gousb.ErrorNotFound
	}

	return nil, fmt.Errorf("%s: %s", addr, err)
}

// ----- UsbTransport -----
// Type UsbTransport implements http.RoundTripper over USB
type UsbTransport struct {
	http.Transport               // Underlying http.Transport
	dev            *gousb.Device // Underlying USB device
	ifaddrs        []*usbIfAddr  // IPP interfaces
	dialSem        chan struct{} // Counts available connections
	dialLock       sync.Mutex    // Protects access to ifaddrs
}

// Type UsbDeviceInfo represents device description suitable
// for DNS-SD registration purposes
type UsbDeviceInfo struct {
	Manufacturer string
	Product      string
	SerialNumber string
	DeviceId     string
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
func NewUsbTransport() (http.RoundTripper, *UsbDeviceInfo, error) {
	// Open the device
	dev, err := usbOpenDevice()
	if err != nil {
		return nil, nil, err
	}

	// Create UsbTransport
	ifaddrs := usbGetIppIfAddrs(dev.Desc)

	transport := &UsbTransport{
		Transport: http.Transport{
			MaxConnsPerHost:     len(ifaddrs),
			MaxIdleConnsPerHost: len(ifaddrs),
		},
		dev:     dev,
		ifaddrs: ifaddrs,
		dialSem: make(chan struct{}, len(ifaddrs)),
	}

	transport.DialContext = transport.dialContect
	transport.DialTLS = func(network, addr string) (net.Conn, error) {
		return nil, errors.New("No TLS over USB")
	}

	for i := 0; i < len(ifaddrs); i++ {
		transport.dialSem <- struct{}{}
	}

	// Fill UsbDeviceInfo
	ok := func(s string, err error) string {
		if err == nil {
			return s
		} else {
			return ""
		}
	}

	info := &UsbDeviceInfo{
		Manufacturer: ok(dev.Manufacturer()),
		Product:      ok(dev.Product()),
		SerialNumber: ok(dev.SerialNumber()),
		DeviceId:     usbGetDeviceId(dev),
	}

	log_debug("Manufacturer: %s", info.Manufacturer)
	log_debug("Product:      %s", info.Product)
	log_debug("SerialNumber: %s", info.SerialNumber)
	log_debug("DeviceId:     %s", info.DeviceId)

	for _, ifaddr := range transport.ifaddrs {
		log_debug("+ %s", ifaddr)
	}

	return transport, info, nil
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
	if err != nil {
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

	log_debug("+ USB OPEN: %s", ifaddr)

	// Obtain interface
	iface, err := ifaddr.Interface(dev)
	if err != nil {
		log_debug("! USB ERROR: %s", err)
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
			log_debug("! USB ERROR: %s", err)
			break
		}
	}

	if err == nil && (conn.in == nil || conn.out == nil) {
		err = errors.New("Missed input or output endpoint")
	}

	if err != nil {
		conn.Close()
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
	log_debug("+ USB CLOSE: %s", conn.ifaddr)

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

// Find and open IPP-over-USB device
func usbOpenDevice() (*gousb.Device, error) {
	// Open confirming devices
	devs, err := usbCtx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return usbIsIppUsbDevice(desc)
	})

	if err != nil {
		return nil, err
	}

	if len(devs) == 0 {
		return nil, errors.New("IPP-over-USB device not found")
	}

	// We are only interested in a first device
	for _, dev := range devs[1:] {
		dev.Close()
	}

	devs[0].SetAutoDetach(true)

	return devs[0], nil
}
