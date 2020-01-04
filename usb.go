// USB access

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
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
	dev            *gousb.Device // Underlying USB device
	ifaddrs        []*usbIfAddr  // IPP interfaces
	dialSem        chan struct{}
}

// Create new http.RoundTripper backed by IPP-over-USB
func NewUsbTransport() (http.RoundTripper, error) {
	// Open the device
	dev, err := usbOpenDevice()
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
		dev:     dev,
		ifaddrs: ifaddrs,
		dialSem: make(chan struct{}, 1),
	}

	transport.DialContext = transport.dialContect
	transport.DialTLS = func(network, addr string) (net.Conn, error) {
		return nil, errors.New("No TLS over USB")
	}

	transport.dialSemSignal()

	for _, ifaddr := range transport.ifaddrs {
		log_debug("+ %s", ifaddr)
	}

	return transport, nil
}

// Dial new connection
func (transport *UsbTransport) dialContect(ctx context.Context,
	network, addr string) (net.Conn, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-transport.dialSem:
			for i := range transport.ifaddrs {
				if !transport.ifaddrs[i].Busy {
					conn, err := openUsbConn(transport,
						transport.ifaddrs[i])
					transport.dialSemSignal()
					return conn, err
				}
			}
		}
	}
}

// Signal transport.dialSem
func (transport *UsbTransport) dialSemSignal() {
	select {
	case transport.dialSem <- struct{}{}:
	default:
	}
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
	return conn.in.Read(b)
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
	conn.transport.dialSemSignal()
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
		ifaddr.DevDesc.Device,
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

					log_debug("%d found %s %v", len(ifaddrs), desc, alt.Endpoints)
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

	return devs[0], nil
}
