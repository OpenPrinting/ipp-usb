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
	UsbAddr        UsbAddr
	State          *DevState
	HttpClient     *http.Client
	HttpServer     *http.Server
	UsbTransport   *UsbTransport
	DnsSdPublisher *DnsSdPublisher
}

// NewIppUsb creates new IppUsb object
func NewDevice(addr UsbAddr) (*Device, error) {
	dev := &Device{
		UsbAddr: addr,
	}

	var err error
	var info UsbDeviceInfo
	var listener net.Listener
	var dnssd_ipp DnsSdInfo
	var dnssd_name string

	// Create USB transport
	dev.UsbTransport, err = NewUsbTransport(addr)
	if err != nil {
		goto ERROR
	}

	// Obtain device info
	info = dev.UsbTransport.UsbDeviceInfo()
	log_debug("+ %s: device info", addr)
	log_debug("  Ident:        %s", info.Ident())
	log_debug("  Manufacturer: %s", info.Manufacturer)
	log_debug("  Product:      %s", info.Product)
	log_debug("  DeviceId:     %s", info.DeviceId)

	// Load persistent state
	dev.State = LoadDevState(info.Ident())

	// Update comment
	dev.State.SetComment(info.Comment())

	// Create HTTP client for local queries
	dev.HttpClient = &http.Client{
		Transport: dev.UsbTransport,
	}

	// Create net.Listener
	listener, err = dev.State.HttpListen()
	if err != nil {
		goto ERROR
	}

	// Create HTTP server
	dev.HttpServer = NewHttpServer(listener, dev.UsbTransport)

	// Setup DNS-SD
	dnssd_name, dnssd_ipp, err = IppService(dev.HttpClient)
	if err != nil {
		goto ERROR
	}

	// Start DNS-SD published
	dev.DnsSdPublisher, err = NewDnsSdPublisher(dnssd_name, dev.State.HttpPort)
	if err != nil {
		goto ERROR
	}

	err = dev.DnsSdPublisher.Add(dnssd_ipp)
	if err != nil {
		goto ERROR
	}

	err = dev.DnsSdPublisher.Publish()
	if err != nil {
		goto ERROR
	}

	return dev, nil
ERROR:
	if dev.DnsSdPublisher != nil {
		dev.DnsSdPublisher.Close()
	}

	if dev.HttpServer != nil {
		dev.HttpServer.Close()
	}

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
	dev.DnsSdPublisher.Close()
	dev.HttpServer.Close()
	dev.UsbTransport.Close()
}
