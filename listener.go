/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * HTTP listener
 */

package main

import (
	"net"
	"strconv"
	"time"
)

// Listener wraps net.Listener
//
// Note, if IP address is not specified, go stdlib
// creates a beautiful listener, able to listen to
// IPv4 and IPv6 simultaneously. But it cannot do it,
// if IP address is given
//
// So it is much simpler to always create a broadcast listener
// and to filter incoming connection in Accept() wrapper rather
// that create separate IPv4 and IPv6 listeners and dial with
// them both
type Listener struct {
	net.Listener // Underlying net.Listener
}

// Create new listener
func NewListener(port int) (net.Listener, error) {
	// Setup network and address
	network := "tcp4"
	if Conf.IpV6Enable {
		network = "tcp"
	}

	addr := ":" + strconv.Itoa(port)

	// Create net.Listener
	nl, err := net.Listen(network, addr)
	if err != nil {
		return nil, err
	}

	// Wrap into Listener
	return Listener{nl}, nil
}

// Accept new connection
func (l Listener) Accept() (net.Conn, error) {
	for {
		// Accept new connection
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}

		// Obtain underlying net.TCPConn
		tcpconn, ok := conn.(*net.TCPConn)
		if !ok {
			// Should never happen, actually
			conn.Close()
			continue
		}

		// Reject non-loopback connections, if required
		if Conf.LoopbackOnly &&
			!tcpconn.LocalAddr().(*net.TCPAddr).IP.IsLoopback() {
			tcpconn.SetLinger(0)
			tcpconn.Close()
			continue
		}

		// Setup TCP parameters
		tcpconn.SetKeepAlive(true)
		tcpconn.SetKeepAlivePeriod(20 * time.Second)

		return tcpconn, nil
	}
}
