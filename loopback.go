/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Loopback interface index discovery
 */

package main

import (
	"errors"
	"fmt"
	"net"
)

// Loopback returns index of loopback interface
func Loopback() (int, error) {
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			if (iface.Flags & net.FlagLoopback) != 0 {
				return iface.Index, nil
			}
		}
	}

	if err == nil {
		err = errors.New("not found")
	}

	return 0, fmt.Errorf("Loopback discovery: %s", err)
}
