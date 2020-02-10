/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Device object brings all parts together
 */

package main

import (
	"context"
	"net"
	"net/http"
)

// Device object brings all parts together, namely:
//   * HTTP proxy server
//   * USB-backed http.Transport
//   * DNS-SD advertiser
//
// There is one instance of Device object per USB device
type Device struct {
	UsbAddr        UsbAddr         // Device's USB address
	State          *DevState       // Persistent state
	HTTPClient     *http.Client    // HTTP client for internal queries
	HTTPProxy      *HTTPProxy      // HTTP proxy
	UsbTransport   *UsbTransport   // Backing USB transport
	DNSSdPublisher *DNSSdPublisher // DNS-SD publisher
	Log            *Logger         // Device's logger
}

// NewDevice creates new Device object
func NewDevice(desc UsbDeviceDesc) (*Device, error) {
	dev := &Device{
		UsbAddr: desc.UsbAddr,
	}

	var err error
	var info UsbDeviceInfo
	var listener net.Listener
	var ippinfo IppPrinterInfo
	var dnssdServices DNSSdServices
	var log *LogMessage

	// Create USB transport
	dev.UsbTransport, err = NewUsbTransport(desc)
	if err != nil {
		goto ERROR
	}

	// Obtain device's logger
	info = dev.UsbTransport.UsbDeviceInfo()
	dev.Log = dev.UsbTransport.Log()

	// Load persistent state
	dev.State = LoadDevState(info.Ident())

	// Update comment
	dev.State.SetComment(info.Comment())

	// Create HTTP client for local queries
	dev.HTTPClient = &http.Client{
		Transport: &HTTPLoggingRoundTripper{
			Log:       dev.Log,
			transport: dev.UsbTransport,
		},
	}

	// Create net.Listener
	listener, err = dev.State.HTTPListen()
	if err != nil {
		goto ERROR
	}

	// Create HTTP server
	dev.HTTPProxy = NewHTTPProxy(dev.Log, listener, dev.UsbTransport)

	// Obtain DNS-SD info for IPP, this is required, we are
	// IPP-USB gate, after all :-)
	log = dev.Log.Begin()
	defer log.Commit()

	ippinfo, err = IppService(log, &dnssdServices,
		dev.State.HTTPPort, info, dev.HTTPClient)

	log.Flush()

	if err != nil {
		goto ERROR
	}

	// Update device state, if name changed
	if ippinfo.DNSSdName != dev.State.DNSSdName {
		dev.State.DNSSdName = ippinfo.DNSSdName
		dev.State.DNSSdOverride = ippinfo.DNSSdName
		dev.State.Save()
	}

	// Obtain DNS-SD info for eSCL, this is optional
	err = EsclService(log, &dnssdServices, dev.State.HTTPPort, info, ippinfo,
		dev.HTTPClient)

	log.Flush()

	if err != nil {
		dev.Log.Error('!', "%s", err)
	}

	// Advertise Web service. Assume it always exist
	dnssdServices.Add(DNSSdSvcInfo{Type: "_http._tcp", Port: dev.State.HTTPPort})

	// Start DNS-SD publisher
	for _, svc := range dnssdServices {
		dev.Log.Debug('>', "%s: %s TXT record:", ippinfo.DNSSdName, svc.Type)
		for _, txt := range svc.Txt {
			dev.Log.Debug(' ', "  %s=%s", txt.Key, txt.Value)
		}
	}

	if Conf.DNSSdEnable {
		dev.DNSSdPublisher = NewDNSSdPublisher(dev.Log, dev.State,
			dnssdServices)
		err = dev.DNSSdPublisher.Publish()
		if err != nil {
			goto ERROR
		}
	}

	return dev, nil

ERROR:
	if dev.HTTPProxy != nil {
		dev.HTTPProxy.Close()
	}

	if dev.UsbTransport != nil {
		dev.UsbTransport.Close()
	}

	if listener != nil {
		listener.Close()
	}

	return nil, err
}

// Shutdown gracefully shuts down the device. If provided context
// expires before the shutdown is complete, Shutdown returns the
// context's error
func (dev *Device) Shutdown(ctx context.Context) error {
	if dev.DNSSdPublisher != nil {
		dev.DNSSdPublisher.Unpublish()
		dev.DNSSdPublisher = nil
	}

	if dev.HTTPProxy != nil {
		dev.HTTPProxy.Close()
		dev.HTTPProxy = nil
	}

	if dev.UsbTransport != nil {
		return dev.UsbTransport.Shutdown(ctx)
	}

	return nil
}

// Close the Device
func (dev *Device) Close() {
	if dev.DNSSdPublisher != nil {
		dev.DNSSdPublisher.Unpublish()
		dev.DNSSdPublisher = nil
	}

	if dev.HTTPProxy != nil {
		dev.HTTPProxy.Close()
		dev.HTTPProxy = nil
	}

	if dev.UsbTransport != nil {
		dev.UsbTransport.Close()
		dev.UsbTransport = nil
	}
}
