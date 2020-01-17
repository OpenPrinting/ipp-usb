/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Device object brings all parts together
 */

package main

import (
	"net"
	"net/http"
)

// IppUsb object brings all parts together, namely:
//   * HTTP proxy server
//   * USB-backed http.Transport
//   * DNS-SD advertiser
//
// There is one instance of IppUsb object per USB device
type Device struct {
	UsbAddr      UsbAddr
	State        *DevState
	HttpServer   *http.Server
	UsbTransport *UsbTransport
}

// NewIppUsb creates new IppUsb object
func NewDevice(addr UsbAddr) (*Device, error) {
	dev := &Device{
		UsbAddr: addr,
	}

	var err error
	var info UsbDeviceInfo
	var listener net.Listener

	// Create USB transport
	dev.UsbTransport, err = NewUsbTransport(addr)
	if err != nil {
		goto ERROR
	}

	// Load persistent state
	info = dev.UsbTransport.UsbDeviceInfo()
	dev.State = LoadDevState(info.Ident())

	// Create net.Listener
	listener, err = dev.State.HttpListen()
	if err != nil {
		goto ERROR
	}

	// Create HTTP server
	dev.HttpServer = NewHttpServer(listener, dev.UsbTransport)
	return dev, nil

ERROR:
	if dev.UsbTransport != nil {
		dev.UsbTransport.Close()
	}

	if listener != nil {
		listener.Close()
	}

	return nil, err
}

// Close the Device
func (dev *Device) Close() {
	dev.HttpServer.Close()
	dev.UsbTransport.Close()
}
