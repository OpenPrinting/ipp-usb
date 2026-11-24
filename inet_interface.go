//go:build linux || freebsd

/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * INET interface index discovery
 */

package main

// #cgo pkg-config: avahi-client
//
// #include <stdlib.h>
// #include <avahi-client/publish.h>
// #include <avahi-common/error.h>
// #include <avahi-common/thread-watch.h>
// #include <avahi-common/watch.h>
import "C"

import (
	"errors"
	"fmt"
	"net"
)

// InetInterface returns index of named interface
func InetInterface(name string) (int, error) {
	switch name {
	case "all":
		return C.AVAHI_IF_UNSPEC, nil
	case "lo":
	case "loopback":
		return Loopback()
	default:
		break
	}

	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			if iface.Name == name {
				return iface.Index, nil
			}
		}
		err = errors.New("not found")
	}

	return 0, fmt.Errorf("Inet interface discovery: %s", err)
}
