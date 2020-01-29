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

// Start PnP manager
func PnPStart() {
	devices := UsbAddrList{}
	devByAddr := make(map[string]*Device)
	sigChan := make(chan os.Signal, 1)

	signal.Notify(sigChan,
		os.Signal(syscall.SIGINT),
		os.Signal(syscall.SIGTERM),
		os.Signal(syscall.SIGHUP))

	// Serve PnP events until terminated
loop:
	for {
		newdevices := BuildUsbAddrList()
		added, removed := devices.Diff(newdevices)
		devices = newdevices

		for _, addr := range added {
			Log.Debug('+', "PNP %s: added", addr)
			dev, err := NewDevice(addr)
			if err == nil {
				devByAddr[addr.MapKey()] = dev
			} else {
				Log.Error('!', "PNP %s: %s", addr, err)
			}
		}

		for _, addr := range removed {
			Log.Debug('-', "PNP %s: removed", addr)
			dev, ok := devByAddr[addr.MapKey()]
			if ok {
				dev.Close()
				delete(devByAddr, addr.MapKey())
			}
		}

		select {
		case <-UsbHotPlugChan:
		case sig := <-sigChan:
			Log.Info(' ', "%s signal received, exiting", sig)
			break loop
		}
	}

	// Close remaining devices
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
}
