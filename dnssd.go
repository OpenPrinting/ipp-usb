/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Publishing self via DNS-SD
 */

package main

import (
	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-avahi"
)

// Type DnsSd represents a DNS-SD service registration
type DnsSd struct {
	server *avahi.Server
}

// Publish self on DNS-SD
func DnsSdPublish() (*DnsSd, error) {
	var host, fqdn string

	// Connect to dbus
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, err
	}

	// create avahi.Server
	server, err := avahi.ServerNew(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Create new entry group. Have no idea what does it mean, though
	eg, err := server.EntryGroupNew()
	if err != nil {
		goto ERROR
	}

	host, err = server.GetHostName()
	if err != nil {
		goto ERROR
	}

	fqdn, err = server.GetHostNameFqdn()
	if err != nil {
		goto ERROR
	}

	err = eg.AddService(
		avahi.InterfaceUnspec,
		avahi.ProtoUnspec,
		0,
		host,
		"_test._tcp",
		"local",
		fqdn,
		1234,
		nil,
	)

	if err != nil {
		goto ERROR
	}

	err = eg.Commit()
	if err != nil {
		goto ERROR
	}

	return &DnsSd{server}, nil

ERROR:
	if server != nil {
		server.Close()
	}

	return nil, err
}

// Remove DNS-SD registration
func (dnssd *DnsSd) Remove() {
	dnssd.server.Close()
}
