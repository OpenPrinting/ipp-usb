/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Publishing self via DNS-SD
 */

package main

import (
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-avahi"
)

// DnsSdTxtItem represents a single TXT record item
type DnsSdTxtItem struct {
	Key, Value string
}

// DnsDsTxtRecord represents a TXT record
type DnsDsTxtRecord []DnsSdTxtItem

// Add adds item to DnsDsTxtRecord
func (txt *DnsDsTxtRecord) Add(key, value string) {
	*txt = append(*txt, DnsSdTxtItem{key, value})
}

// IfNotEmpty adds item to DnsDsTxtRecord if its value is not empty
//
// It returns true if item was actually added, false otherwise
func (txt *DnsDsTxtRecord) IfNotEmpty(key, value string) bool {
	if value != "" {
		txt.Add(key, value)
		return true
	}
	return false
}

// export DnsDsTxtRecord into Avahi format
func (txt DnsDsTxtRecord) export() [][]byte {
	var exported [][]byte

	// Note, for a some strange reason, Avahi published
	// TXT record in reverse order, so compensate it here
	for i := len(txt) - 1; i >= 0; i-- {
		item := txt[i]
		exported = append(exported, []byte(item.Key+"="+item.Value))
	}

	return exported
}

// DnsSdService represents a DNS-SD service information
type DnsSdInfo struct {
	Type string         // Service type, i.e. "_ipp._tcp"
	Port int            // TCP port
	Txt  DnsDsTxtRecord // TXT record
}

// DnsSdPublisher represents a DNS-SD service publisher
// One publisher may publish multiple services unser the
// same Service Instance Name
type DnsSdPublisher struct {
	Instance     string            // Service Instance Name
	Services     []DnsSdInfo       // Registered services
	iface, proto int               // interface and protocol IDs
	server       *avahi.Server     // Avahi Server connection
	egroup       *avahi.EntryGroup // Avahi Entry Group
}

// NewDnsSdPublisher creates new DnsSdPublisher
func NewDnsSdPublisher(services []DnsSdInfo) *DnsSdPublisher {
	return &DnsSdPublisher{
		Services: services,
	}

}

// Unpublish everything
func (publisher *DnsSdPublisher) Unpublish() {
	publisher.server.Close()

	if publisher.egroup != nil {
		publisher.server.EntryGroupFree(publisher.egroup)
		publisher.egroup = nil
	}

	if publisher.server != nil {
		publisher.server.Close()
		publisher.server = nil
	}
}

// Publish all services
func (publisher *DnsSdPublisher) Publish(instance string) error {
	var err error
	var conn *dbus.Conn

	// Save instance
	publisher.Instance = instance

	// Compute iface and proto
	publisher.iface = avahi.InterfaceUnspec
	if Conf.LoopbackOnly {
		publisher.iface, err = Loopback()
		if err != nil {
			goto ERROR
		}
	}

	publisher.proto = avahi.ProtoUnspec
	if !Conf.IpV6Enable {
		publisher.proto = avahi.ProtoInet
	}

	// Connect to dbus
	conn, err = dbus.SystemBus()
	if err != nil {
		goto ERROR
	}

	// create avahi.Server
	publisher.server, err = avahi.ServerNew(conn)
	if err != nil {
		goto ERROR
	}

	conn = nil // Now owned by publisher.server

	// Create new entry group
	publisher.egroup, err = publisher.server.EntryGroupNew()
	if err != nil {
		goto ERROR
	}

	// Add all services
	for _, svc := range publisher.Services {
		err = publisher.egroup.AddService(
			int32(publisher.iface),
			int32(publisher.proto),
			0,
			publisher.Instance,
			svc.Type,
			"", // Domain, let Avahi choose
			"", // Host, let Avahi choose
			uint16(svc.Port),
			svc.Txt.export(),
		)

		if err != nil {
			goto ERROR
		}
	}

	// Commit everything
	err = publisher.egroup.Commit()
	if err != nil {
		goto ERROR
	}

	return nil

	// Error: cleanup and exit
ERROR:
	if conn != nil {
		conn.Close()
	}

	publisher.Unpublish()

	return fmt.Errorf("DNS-SD: %s", err)
}
