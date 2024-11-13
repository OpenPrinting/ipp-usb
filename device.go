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
	"fmt"
	"net"
	"net/http"
	"time"
)

// Device object brings all parts together, namely:
//   - HTTP proxy server
//   - USB-backed http.Transport
//   - DNS-SD advertiser
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
	var ippinfo *IppPrinterInfo
	var dnssdName string
	var dnssdServices DNSSdServices
	var log *LogMessage
	var hwid string

	// Create USB transport
	dev.UsbTransport, err = NewUsbTransport(desc)
	if err != nil {
		goto ERROR
	}

	// Obtain device's logger
	dev.Log = dev.UsbTransport.Log()

	// Obtain device info and derived information.
	info = dev.UsbTransport.UsbDeviceInfo()
	hwid = fmt.Sprintf("%4.4x&%4.4x", info.Vendor, info.Product)

	// Load persistent state
	dev.State = LoadDevState(info.Ident(), info.Comment())

	// Create HTTP client for local queries
	dev.HTTPClient = &http.Client{
		Transport: dev.UsbTransport,
	}

	// Create net.Listener
	listener, err = dev.State.HTTPListen()
	if err != nil {
		goto ERROR
	}

	// Create HTTP server
	dev.UsbTransport.SetDeadline(
		time.Now().
			Add(DevInitTimeout).
			Add(dev.UsbTransport.Quirks().GetInitDelay()))

	dev.HTTPProxy = NewHTTPProxy(dev.Log, listener, dev.UsbTransport)

	// Obtain DNS-SD info for IPP
	log = dev.Log.Begin()
	defer log.Commit()

	ippinfo, err = IppService(log, &dnssdServices,
		dev.State.HTTPPort, info, dev.UsbTransport.Quirks(),
		dev.HTTPClient)

	if err != nil {
		dev.Log.Error('!', "IPP: %s", err)
	}

	log.Flush()

	if dev.UsbTransport.DeadlineExpired() {
		err = ErrInitTimedOut
		goto ERROR
	}

	// Obtain DNS-SD name
	if ippinfo != nil {
		dnssdName = ippinfo.DNSSdName
	} else {
		dnssdName = info.DNSSdName()
	}

	// Update device state, if name changed
	if dnssdName != dev.State.DNSSdName {
		dev.State.DNSSdName = dnssdName
		dev.State.DNSSdOverride = dnssdName
		dev.State.Save()
	}

	// Obtain DNS-SD info for eSCL
	err = EsclService(log, &dnssdServices, dev.State.HTTPPort, info,
		ippinfo, dev.HTTPClient)

	if err != nil {
		dev.Log.Error('!', "ESCL: %s", err)
	}

	log.Flush()

	if dev.UsbTransport.DeadlineExpired() {
		err = ErrInitTimedOut
		goto ERROR
	}

	// Update IPP service advertising for scanner presence
	if ippinfo != nil {
		if ippSvc := &dnssdServices[ippinfo.IppSvcIndex]; err == nil {
			ippSvc.Txt.Add("Scan", "T")
		} else {
			ippSvc.Txt.Add("Scan", "F")
		}
	}

	// Skip the device, if it cannot do something useful
	//
	// Some devices (so far, only HP-rebranded Samsung devices
	// known to have such a defect) offer 7/1/4 interfaces, but
	// actually provide no functionality behind these interfaces
	// and respond with `HTTP 404 Not found` to all the HTTP
	// requests sent to USB
	//
	// ipp-usb ignores such devices to let a chance for
	// legacy/proprietary drivers to work with them
	if len(dnssdServices) == 0 {
		err = ErrUnusable
		goto ERROR
	}

	// Add common TXT records:
	//   - usb_SER=VCF9192281  ; Device USB serial number
	//   - usb_HWID=0482&069d  ; Its vendor and device ID
	for i := range dnssdServices {
		svc := &dnssdServices[i]
		svc.Txt.Add("usb_SER", info.SerialNumber)
		svc.Txt.Add("usb_HWID", hwid)
	}

	// Advertise Web service. Assume it always exists
	dnssdServices.Add(DNSSdSvcInfo{Type: "_http._tcp", Port: dev.State.HTTPPort})

	// Advertise service with the following parameters:
	//   Instance: "BBPP", where BB and PP are bus and port numbers in hex
	//   Type:     "_ipp-usb._tcp"
	//
	// The purpose of this advertising is to help legacy drivers to
	// easily check for devices, handled by ipp-usb
	//
	// See the following for details:
	//     https://github.com/OpenPrinting/ipp-usb/issues/28
	dnssdServices.Add(DNSSdSvcInfo{
		Instance: fmt.Sprintf("%.2X%.2x", desc.Bus, info.PortNum),
		Type:     "_ipp-usb._tcp",
		Port:     dev.State.HTTPPort,
		Loopback: true,
	})

	// Enable handling incoming requests
	dev.UsbTransport.SetDeadline(time.Time{})
	dev.HTTPProxy.Enable()

	// Start DNS-SD publisher
	for _, svc := range dnssdServices {
		dev.Log.Debug('>', "%s: %s TXT record:", dnssdName, svc.Type)
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
		dev.UsbTransport.Close(true)
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
		dev.UsbTransport.Close(false)
		dev.UsbTransport = nil
	}
}
