/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * ipp-usb status support
 */

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
	"sync"
)

// statusOfDevice represents a status of the particular device
type statusOfDevice struct {
	desc     UsbDeviceDesc // Device descriptor
	init     error         // Initialization error, nil if none
	HTTPPort int           // Assigned http port for the device
}

var (
	// statusTable maintains a per-device status,
	// indexed by the UsbAddr
	statusTable = make(map[UsbAddr]*statusOfDevice)

	// statusLock protects access to the statusTable
	statusLock sync.RWMutex
)

// StatusRetrieve connects to the running ipp-usb daemon, retrieves
// its status and returns retrieved status as a printable text
func StatusRetrieve() ([]byte, error) {
	t := &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			return CtrlsockDial()
		},
	}

	c := &http.Client{
		Transport: t,
	}

	rsp, err := c.Get("http://localhost/status")
	if err != nil {
		return nil, err
	}

	defer rsp.Body.Close()

	return ioutil.ReadAll(rsp.Body)
}

// StatusFormat formats ipp-usb status as a text
func StatusFormat() []byte {
	buf := &bytes.Buffer{}

	// Lock the statusTable
	statusLock.RLock()
	defer statusLock.RUnlock()

	// Dump ipp-usb daemon status. If we are here, we are
	// definitely running :-)
	fmt.Fprintf(buf, "ipp-usb daemon %s: running\n", Version)

	// Sort devices by address
	devs := make([]*statusOfDevice, len(statusTable))

	i := 0
	for _, status := range statusTable {
		devs[i] = status
		i++
	}

	sort.Slice(devs, func(i, j int) bool {
		return devs[i].desc.UsbAddr.Less(devs[j].desc.UsbAddr)
	})

	// Format per-device status
	buf.WriteString("ipp-usb devices:")
	if len(statusTable) == 0 {
		buf.WriteString(" not found\n")
	} else {
		buf.WriteString("\n")
		fmt.Fprintf(buf, " Num  Device              Vndr:Prod  Port  Model\n")
		for i, status := range devs {
			info, _ := status.desc.GetUsbDeviceInfo()

			fmt.Fprintf(buf, " %3d. %s  %4.4x:%.4x  %-5d %q\n",
				i+1, status.desc.UsbAddr,
				info.Vendor, info.Product, status.HTTPPort,
				info.MfgAndProduct)

			s := "OK"
			if status.init != nil {
				s = devs[i].init.Error()
			}

			fmt.Fprintf(buf, "      status: %s\n", s)
		}
	}

	return buf.Bytes()
}

// StatusSet adds device to the status table or updates status
// of the already known device
func StatusSet(addr UsbAddr, desc UsbDeviceDesc, HTTPPort int, init error) {
	statusLock.Lock()
	statusTable[addr] = &statusOfDevice{
		desc:     desc,
		init:     init,
		HTTPPort: HTTPPort,
	}
	statusLock.Unlock()
}

// StatusDel deletes device from the status table
func StatusDel(addr UsbAddr) {
	statusLock.Lock()
	delete(statusTable, addr)
	statusLock.Unlock()
}
