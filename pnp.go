/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * PnP manager
 */

package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// PnPExitReason explains why PnP manager has exited
type PnPExitReason int

// PnPExitReason constants
const (
	PnPIdle PnPExitReason = iota // No more connected devices
	PnPTerm                      // Terminating signal received
)

// pnpUpdateStateRequestChan used to send pnpUpdateStateRequest
// messages to the PnP manager.
var pnpUpdateStateRequestChan = make(chan pnpUpdateStateRequest)

// pnpUpdateStateRequest is the synchronous request to update
// state of running device.
type pnpUpdateStateRequest struct {
	state    *DevState
	response chan error
}

// PnPUpdateState updates state of running device, such as
func PnPUpdateState(state *DevState) error {
	rq := pnpUpdateStateRequest{
		state:    state,
		response: make(chan error),
	}

	pnpUpdateStateRequestChan <- rq
	err := <-rq.response

	return err
}

// PnPStart start PnP manager
//
// If exitWhenIdle is true, PnP manager will exit, when there is no more
// devices to serve
func PnPStart(exitWhenIdle bool) PnPExitReason {
	devices := UsbAddrList{}
	var descByAddr map[UsbAddr]UsbDeviceDesc
	devByAddr := make(map[UsbAddr]*Device)
	retryByAddr := make(map[UsbAddr]time.Time)
	addrByURI := make(map[URI]UsbAddr)
	uriByAddr := make(map[UsbAddr]URI)

	sigChan := make(chan os.Signal, 1)
	ticker := time.NewTicker(DevInitRetryInterval / 4)
	tickerRunning := true

	signal.Notify(sigChan,
		os.Signal(syscall.SIGINT),
		os.Signal(syscall.SIGTERM),
		os.Signal(syscall.SIGHUP))

	// Start control socket server
	err := CtrlsockStart()
	if err == nil {
		defer CtrlsockStop()
	}

	// addDevice handles adding a device
	addDevice := func(addr UsbAddr, state *DevState) error {
		desc := descByAddr[addr]
		dev, uri, err := NewDevice(desc, state)

		if uri != nil {
			addrByURI[*uri] = addr
			uriByAddr[addr] = *uri
		}

		port := 0
		if dev != nil {
			port = dev.UsbTransport.HTTPPort()
		}
		StatusSet(addr, desc, port, err)

		if err == nil {
			devByAddr[addr] = dev
			delete(retryByAddr, addr)
		} else {
			Log.Error('!', "PNP %s: %s", addr, err)
			if err != ErrDisabled {
				retryByAddr[addr] = pnpRetryTime(err)
			}
		}

		return err
	}

	// delDevice handles removal of the device
	//
	// Device can be removed either due to the
	// configuration change or physical disconnect.
	//
	// If disconnect flag is set, which indicates the
	// physical removal of device, it will be removed
	// from the uriByAddr and addrByURI tables.
	delDevice := func(addr UsbAddr, disconnect bool) {
		delete(retryByAddr, addr)

		if disconnect {
			if uri, found := uriByAddr[addr]; found {
				delete(uriByAddr, addr)
				delete(addrByURI, uri)
			}
			StatusDel(addr)
		} else {
			StatusUpdateInitError(addr, ErrDisabled)
		}

		dev, ok := devByAddr[addr]
		if ok {
			dev.Close()
			delete(devByAddr, addr)
		}
	}

	// updateDeviceState handles DevState update request on
	// a context of the PnP manager
	updateDeviceState := func(state *DevState) error {
		addr, found := addrByURI[*state.URI()]
		if !found {
			return nil
		}

		dev := devByAddr[addr]
		switch {
		case dev != nil && state.Disabled:
			Log.Debug('-', "PNP %s: disabled", addr)
			delDevice(addr, false)
			return nil
		case dev == nil && !state.Disabled:
			Log.Debug('+', "PNP %s: enabled", addr)
			err = addDevice(addr, state)
			return err
		}

		return nil
	}

	// Serve PnP events until terminated
loop:
	for {
		descByAddr, err = UsbGetIppOverUsbDeviceDescs()

		if err == nil {
			newdevices := UsbAddrList{}
			for _, desc := range descByAddr {
				newdevices.Add(desc.UsbAddr)
			}

			added, removed := devices.Diff(newdevices)
			devices = newdevices

			// Handle added devices
			for _, addr := range added {
				Log.Debug('+', "PNP %s: added", addr)
				addDevice(addr, nil)
			}

			// Handle removed devices
			for _, addr := range removed {
				Log.Debug('-', "PNP %s: removed", addr)
				delDevice(addr, true)
			}

			// Handle devices, waiting for retry
			for addr, tm := range retryByAddr {
				if !pnpRetryExpired(tm) {
					continue
				}

				Log.Debug('+', "PNP %s: retry", addr)
				addDevice(addr, nil)
			}
		}

		// Handle exit when idle
		if exitWhenIdle && len(devices) == 0 {
			Log.Info(' ', "No IPP-over-USB devices present, exiting")
			return PnPIdle
		}

		// Update ticker
		switch {
		case tickerRunning && len(retryByAddr) == 0:
			ticker.Stop()
			tickerRunning = false
		case !tickerRunning && len(retryByAddr) != 0:
			ticker = time.NewTicker(DevInitRetryInterval / 4)
			tickerRunning = true
		}

		// Wait for the next event
		select {
		case <-UsbHotPlugChan:
		case <-ticker.C:
		case rq := <-pnpUpdateStateRequestChan:
			Log.Debug('+', "PNP %s: state update: requested",
				rq.state.URIString())
			err := updateDeviceState(rq.state)
			if err != nil {
				Log.Debug('+', "PNP %s: state update: %s",
					rq.state.URIString(), err)
				rq.response <- err
			} else {
				Log.Debug('+', "PNP %s: state update: OK",
					rq.state.URIString())
			}
			close(rq.response)
		case sig := <-sigChan:
			Log.Info(' ', "%s signal received, exiting", sig)
			break loop
		}
	}

	// Close remaining devices
	ctx, cancel := context.WithTimeout(context.Background(),
		DevShutdownTimeout)
	defer cancel()

	var done sync.WaitGroup

	for _, dev := range devByAddr {
		done.Add(1)
		go func(dev *Device) {
			dev.Shutdown(ctx)
			dev.Close()
			done.Done()
		}(dev)
	}

	done.Wait()
	return PnPTerm
}

// pnpRetryTime returns time of next retry of failed device initialization
func pnpRetryTime(err error) time.Time {
	if err == ErrBlackListed || err == ErrUnusable {
		// These errors are unrecoverable.
		// Forget about device for the next million hours :-)
		return time.Now().Add(time.Hour * 1e6)
	}

	return time.Now().Add(DevInitRetryInterval)
}

// pnpRetryExpired checks if device initialization retry time expired
func pnpRetryExpired(tm time.Time) bool {
	return !time.Now().Before(tm)
}
