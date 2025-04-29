//go:build !linux
// +build !linux

/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * UID discovery for TCP connection over loopback -- default version
 *
 * If you've have added support for yet another platform, please don't
 * forget to update build tag at the top of this file to exclude your
 * platform
 */

package main

import (
	"net"
)

// TCPClientUIDSupported tells if TCPClientUID supported on this platform
//
// If this function returns false, TCPClientUID should never be called
func TCPClientUIDSupported() bool {
	return false
}

// TCPClientUID obtains UID of client process that created
// TCP connection over the loopback interface
func TCPClientUID(client, server *net.TCPAddr) (int, error) {
	// Note, TCPClientUID should never be called, if
	// TCPClientUIDSupported returns false
	panic("TCPClientUID not supported")
}
