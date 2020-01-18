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

// DnsSdTxtItem represents a single TXT record item
type DnsSdTxtItem struct {
	Key, Value string
}

// DnsDsTxtRecord represents a TXT record
type DnsDsTxtRecord []DnsSdTxtItem

// DnsSdService represents a DNS-SD service information
type DnsSdInfo struct {
	Port     int            // TCP port
	Type     string         // Service type, i.e. "_ipp._tcp"
	Instance string         // Service Instance Name
	Txt      DnsDsTxtRecord // TXT record
}

// Type DnsSd represents a DNS-SD service registration
type DnsSd struct {
	server *avahi.Server
}

// Publish self on DNS-SD
func DnsSdPublish(info DnsSdInfo) (*DnsSd, error) {
	var fqdn string
	var txt [][]byte
	var iface, protocol int
	var conn *dbus.Conn
	var server *avahi.Server
	var eg *avahi.EntryGroup
	var err error

	// Compute iface and protocol
	iface = avahi.InterfaceUnspec
	if Conf.LoopbackOnly {
		iface, err = Loopback()
		if err != nil {
			goto ERROR
		}
	}

	protocol = avahi.ProtoUnspec
	if !Conf.IpV6Enable {
		protocol = avahi.ProtoInet
	}

	// Connect to dbus
	conn, err = dbus.SystemBus()
	if err != nil {
		goto ERROR
	}

	// create avahi.Server
	server, err = avahi.ServerNew(conn)
	if err != nil {
		goto ERROR
	}

	// Create new entry group. Have no idea what does it mean, though
	eg, err = server.EntryGroupNew()
	if err != nil {
		goto ERROR
	}

	// Obtain fully qualified host name
	fqdn, err = server.GetHostNameFqdn()
	if err != nil {
		goto ERROR
	}

	// Register a service
	for _, item := range info.Txt {
		one := item.Key
		if item.Value != "" {
			one += "=" + item.Value
		}
		txt = append(txt, []byte(one))
	}

	err = eg.AddService(
		int32(iface),
		int32(protocol),
		0,
		info.Instance,
		info.Type,
		"local",
		fqdn,
		uint16(info.Port),
		txt,
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
	} else if conn != nil {
		conn.Close()
	}

	return nil, err
}

// Remove DNS-SD registration
func (dnssd *DnsSd) Remove() {
	dnssd.server.Close()
}
