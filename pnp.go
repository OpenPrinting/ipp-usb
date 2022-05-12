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

const (
	PnPIdle PnPExitReason = iota // No more connected devices
	PnPTerm                      // Terminating signal received
)

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

// PnPStart start PnP manager
//
// If exitWhenIdle is true, PnP manager will exit, when there is no more
// devices to serve
func PnPStart(exitWhenIdle bool) PnPExitReason {
	devices := UsbAddrList{}
	devByAddr := make(map[UsbAddr]*Device)
	retryByAddr := make(map[UsbAddr]time.Time)
	sigChan := make(chan os.Signal, 1)
	ticker := time.NewTicker(DevInitRetryInterval / 4)
	tickerRunning := true

	signal.Notify(sigChan,
		os.Signal(syscall.SIGINT),
		os.Signal(syscall.SIGTERM),
		os.Signal(syscall.SIGHUP))

	// Serve PnP events until terminated
loop:
	for {
		dev_descs, err := UsbGetIppOverUsbDeviceDescs()

		if err == nil {
			newdevices := UsbAddrList{}
			for _, desc := range dev_descs {
				newdevices.Add(desc.UsbAddr)
			}

			added, removed := devices.Diff(newdevices)
			devices = newdevices

			// Handle added devices
			for _, addr := range added {
				Log.Debug('+', "PNP %s: added", addr)
				dev, err := NewDevice(dev_descs[addr])
				if err == nil {
					devByAddr[addr] = dev
				} else {
					Log.Error('!', "PNP %s: %s", addr, err)
					retryByAddr[addr] = pnpRetryTime(err)
				}
			}

			// Handle removed devices
			for _, addr := range removed {
				Log.Debug('-', "PNP %s: removed", addr)
				delete(retryByAddr, addr)

				dev, ok := devByAddr[addr]
				if ok {
					dev.Close()
					delete(devByAddr, addr)
				}
			}

			// Handle devices, waiting for retry
			for addr, tm := range retryByAddr {
				if !pnpRetryExpired(tm) {
					continue
				}

				Log.Debug('+', "PNP %s: retry", addr)
				dev, err := NewDevice(dev_descs[addr])
				if err == nil {
					devByAddr[addr] = dev
					delete(retryByAddr, addr)
				} else {
					Log.Error('!', "PNP %s: %s", addr, err)
					retryByAddr[addr] = pnpRetryTime(err)
				}
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

		select {
		case <-UsbHotPlugChan:
		case <-ticker.C:
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
