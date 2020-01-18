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

// Add item to DnsDsTxtRecord
func (txt *DnsDsTxtRecord) Add(Key, Value string) {
	*txt = append(*txt, DnsSdTxtItem{Key, Value})
}

// export DnsDsTxtRecord into Avahi format
func (txt DnsDsTxtRecord) export() [][]byte {
	var exported [][]byte

	for _, item := range txt {
		one := item.Key
		if item.Value != "" {
			one += "=" + item.Value
		}
		exported = append(exported, []byte(one))
	}

	return exported
}

// DnsSdService represents a DNS-SD service information
type DnsSdInfo struct {
	Port int            // TCP port
	Type string         // Service type, i.e. "_ipp._tcp"
	Txt  DnsDsTxtRecord // TXT record
}

// DnsSdPublisher represents a DNS-SD service publisher
// One publisher may publish multiple services unser the
// same Service Instance Name
type DnsSdPublisher struct {
	Instance     string            // Service Instance Name
	iface, proto int               // interface and protocol IDs
	server       *avahi.Server     // Avahi Server connection
	egroup       *avahi.EntryGroup // Avahi Entry Group
}

// NewDnsSdPublisher creates new DnsSdPublisher
func NewDnsSdPublisher(instanse string) (*DnsSdPublisher, error) {
	var iface, proto int
	var conn *dbus.Conn
	var server *avahi.Server
	var egroup *avahi.EntryGroup
	var publisher *DnsSdPublisher
	var err error

	// Compute iface and proto
	iface = avahi.InterfaceUnspec
	if Conf.LoopbackOnly {
		iface, err = Loopback()
		if err != nil {
			goto ERROR
		}
	}

	proto = avahi.ProtoUnspec
	if !Conf.IpV6Enable {
		proto = avahi.ProtoInet
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

	conn = nil // Now owned by server

	// Create new entry group
	egroup, err = server.EntryGroupNew()
	if err != nil {
		goto ERROR
	}

	// Build DnsSdPublisher
	publisher = &DnsSdPublisher{
		Instance: instanse,
		iface:    iface,
		proto:    proto,
		server:   server,
		egroup:   egroup,
	}

	return publisher, nil

	// Error: cleanup and exit
ERROR:
	if egroup != nil {
		server.EntryGroupFree(egroup)
	}

	if server != nil {
		server.Close()
	}

	if conn != nil {
		conn.Close()
	}

	return nil, err
}

// Close DNS-SD publisher
func (publisher *DnsSdPublisher) Close() {
	publisher.server.Close()
}

// Add service to the publisher
func (publisher *DnsSdPublisher) Add(info DnsSdInfo) error {
	// Obtain fully qualified host name
	fqdn, err := publisher.server.GetHostNameFqdn()
	if err != nil {
		return err
	}

	// Register a service
	err = publisher.egroup.AddService(
		int32(publisher.iface),
		int32(publisher.proto),
		0,
		publisher.Instance,
		info.Type,
		"local",
		fqdn,
		uint16(info.Port),
		info.Txt.export(),
	)

	return err
}

// Publish all previously added services
func (publisher *DnsSdPublisher) Publish() error {
	return publisher.egroup.Commit()
}
