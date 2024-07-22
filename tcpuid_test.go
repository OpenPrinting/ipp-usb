/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Tests for TCPClientUID
 */

package main

import (
	"net"
	"os"
	"testing"
)

// doTestTCPClientUID performs TCPClientUID for the specified
// network and loopback address
func doTestTCPClientUID(t *testing.T, ip4 bool) {
	// Do nothing if TCPClientUID is not supported by the platform
	if !TCPClientUIDSupported() {
		return
	}

	// Log local addresses. Check that we have appropriate
	// address family support, configured in the system.
	var haveIP4, haveIP6 bool

	if ift, err := net.Interfaces(); err == nil {
		for _, ifi := range ift {
			if addrs, err := ifi.Addrs(); err == nil {
				t.Logf("%s:", ifi.Name)
				for _, addr := range addrs {
					t.Logf("  %s", addr)

					if ipnet, ok := addr.(*net.IPNet); ok {
						if ipnet.IP.To4() != nil {
							haveIP4 = true
						} else {
							haveIP6 = true
						}
					}
				}
			}
		}
	}

	// Skip incompatible address families
	if ip4 && !haveIP4 {
		return
	}

	if !ip4 && !haveIP6 {
		return
	}

	// Create loopback listener -- it gives us a port
	network := "tcp4"
	loopback := "127.0.0.1"
	if !ip4 {
		loopback = "[::1]"
		network = "tcp6"
	}

	l, err := net.Listen(network, loopback+":")
	if err != nil {
		t.Fatalf("net.Listen(%q,%q): %s", network, loopback+":", err)
	}

	defer l.Close()

	// Dial client connection
	addr := l.Addr()
	clnt, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatalf("net.Dial(%q,%q): %s", network, addr, err)
	}

	defer clnt.Close()

	// Accept server connection
	srv, err := l.Accept()
	if err != nil {
		t.Fatalf("net.Accept(%q,%q): %s", network, addr, err)
	}

	defer srv.Close()

	// Get and check Client UID
	uid, err := TCPClientUID(clnt.LocalAddr().(*net.TCPAddr),
		srv.LocalAddr().(*net.TCPAddr))

	if err != nil {
		t.Fatalf("TCPClientUID(%q,%q): %s",
			clnt.LocalAddr(), srv.LocalAddr(), err)
	}

	if uid != os.Getuid() {
		t.Fatalf("TCPClientUID(%q,%q): uid mismatch (expected %d, present %d)",
			clnt.LocalAddr(), srv.LocalAddr(), os.Getuid(), uid)
	}
}

// TestTCPClientUIDIp4 performs TCPClientUID test for IPv4
func TestTCPClientUIDIp4(t *testing.T) {
	doTestTCPClientUID(t, true)
}

// TestTCPClientUIDIp6 performs TCPClientUID test for IPv6
func TestTCPClientUIDIp6(t *testing.T) {
	doTestTCPClientUID(t, false)
}
