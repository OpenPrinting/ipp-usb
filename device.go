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
	Log            *Logger
}

// NewIppUsb creates new IppUsb object
func NewDevice(addr UsbAddr) (*Device, error) {
	dev := &Device{
		UsbAddr: addr,
	}

	var err error
	var info UsbDeviceInfo
	var listener net.Listener
	var dnssd_name string
	var dnssd_services DnsSdServices
	var log *LogMessage

	// Create USB transport
	dev.UsbTransport, err = NewUsbTransport(addr)
	if err != nil {
		goto ERROR
	}

	// Obtain device info and create logger
	info = dev.UsbTransport.UsbDeviceInfo()
	dev.Log = NewDeviceLogger(info)

	dev.Log.Begin().
		Debug(' ', "===============================").
		Debug('+', "%s: device info", addr).
		Debug(' ', "Ident:        %s", info.Ident()).
		Debug(' ', "Manufacturer: %s", info.Manufacturer).
		Debug(' ', "Product:      %s", info.Product).
		Debug(' ', "DeviceId:     %s", info.DeviceId).
		Commit()

	// Write log messages
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

	// Obtain DNS-SD info for IPP, this is required, we are
	// IPP-USB gate, after all :-)
	log = dev.Log.Begin()
	defer log.Commit()

	dnssd_name, err = IppService(log, &dnssd_services,
		dev.State.HttpPort, info, dev.HttpClient)

	if err != nil {
		goto ERROR
	}

	// Update device state, if name changed
	if dnssd_name != dev.State.DnsSdName {
		dev.State.DnsSdName = dnssd_name
		dev.State.DnsSdOverride = dnssd_name
		dev.State.Save()
	}

	// Obtain DNS-SD info for eSCL, this is optional
	err = EsclService(log, &dnssd_services, dev.State.HttpPort, info, dev.HttpClient)
	if err != nil {
		log_debug("! %s", err)
	}

	// Start DNS-SD publisher
	for _, svc := range dnssd_services {
		log_debug("> %s: %s TXT record:", dnssd_name, svc.Type)
		for _, txt := range svc.Txt {
			log_debug("    %s=%s", txt.Key, txt.Value)
		}
	}

	dev.DnsSdPublisher = NewDnsSdPublisher(dev.State, dnssd_services)
	err = dev.DnsSdPublisher.Publish()
	if err != nil {
		goto ERROR
	}

	return dev, nil

ERROR:
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
	dev.DnsSdPublisher.Unpublish()
	dev.HttpServer.Close()
	dev.UsbTransport.Close()
	dev.Log.Debug(' ', "device closed")
	dev.Log.Close()
}
