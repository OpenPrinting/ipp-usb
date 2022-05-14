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
	"fmt"
	"os"
	"os/signal"
	"sort"
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

// pnpUpdateStatusFile updates ipp-usb status file
func pnpUpdateStatusFile(content map[UsbAddr]error,
	dev_descs map[UsbAddr]UsbDeviceDesc) {
	// Sort output by address
	devs := make([]struct {
		addr UsbAddr
		err  error
	}, len(content))

	i := 0
	for addr, err := range content {
		devs[i].addr = addr
		devs[i].err = err
	}

	sort.Slice(devs, func(i, j int) bool {
		return devs[i].addr.Less(devs[j].addr)
	})

	// Open and lock the status file
	os.MkdirAll(PathLockDir, 0755)
	file, err := os.OpenFile(PathStatusFile,
		os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return
	}

	defer file.Close()

	err = FileLock(file, FileLockWait)
	if err != nil {
		return
	}

	defer FileUnlock(file)

	err = file.Truncate(0)
	if err != nil {
		return
	}

	// Dump to file
	if len(devs) == 0 {
		return
	}

	fmt.Fprintf(file, " Num  Device              Vndr:Prod  Model\n")
	for i = range devs {
		addr := devs[i].addr
		desc := dev_descs[addr]
		info, _ := desc.GetUsbDeviceInfo()

		fmt.Fprintf(file, " %3d. %s  %4.4x:%.4x  %q\n",
			i+1, addr,
			info.Vendor, info.Product, info.MfgAndProduct)

		status := "OK"
		if devs[i].err != nil {
			status = devs[i].err.Error()
		}

		fmt.Fprintf(file, "      status: %s", status)
	}
}

// PnPStart start PnP manager
//
// If exitWhenIdle is true, PnP manager will exit, when there is no more
// devices to serve
func PnPStart(exitWhenIdle bool) PnPExitReason {
	devices := UsbAddrList{}
	devByAddr := make(map[UsbAddr]*Device)
	retryByAddr := make(map[UsbAddr]time.Time)
	statusByAddr := make(map[UsbAddr]error)
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
				statusByAddr[addr] = err

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
				delete(statusByAddr, addr)

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
				statusByAddr[addr] = err

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

		// Update status file
		pnpUpdateStatusFile(statusByAddr, dev_descs)

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
