/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * DNS-SD publisher: system-independent stuff
 */

package main

import (
	"fmt"
	"sync"
	"time"
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

// DnsSdSvcInfo represents a DNS-SD service information
type DnsSdSvcInfo struct {
	Type string         // Service type, i.e. "_ipp._tcp"
	Port int            // TCP port
	Txt  DnsDsTxtRecord // TXT record
}

// DnsSdServices represents a collection of DNS-SD services
type DnsSdServices []DnsSdSvcInfo

// Add DnsSdSvcInfo to DnsSdServices
func (services *DnsSdServices) Add(srv DnsSdSvcInfo) {
	*services = append(*services, srv)
}

// DnsSdPublisher represents a DNS-SD service publisher
// One publisher may publish multiple services unser the
// same Service Instance Name
type DnsSdPublisher struct {
	DevState *DevState      // Device persistent state
	Services DnsSdServices  // Registered services
	fin      chan struct{}  // Closed to terminate publisher goroutine
	finDone  sync.WaitGroup // To wait for goroutine termination
	sysdep   *dnssdSysdep   // System-dependent stuff
}

// DnsSdStatus represents DNS-SD publisher status
type DnsSdStatus int

const (
	DnsSdNoStatus  DnsSdStatus = iota // Invalid status
	DnsSdCollision                    // Service instance name collision
	DnsSdFailure                      // Publisher failed
	DnsSdSuccess                      // Services successfully published
)

// String returns human-readable representation of DnsSdStatus
func (status DnsSdStatus) String() string {
	switch status {
	case DnsSdNoStatus:
		return "DnsSdNoStatus"
	case DnsSdCollision:
		return "DnsSdCollision"
	case DnsSdFailure:
		return "DnsSdFailure"
	case DnsSdSuccess:
		return "DnsSdSuccess"
	}

	return fmt.Sprintf("Unknown DnsSdStatus %d", status)
}

// NewDnsSdPublisher creates new DnsSdPublisher
//
// Service instanse name comes from the DevState, and if
// name changes as result of name collision resolution,
// DevState will be updated
func NewDnsSdPublisher(devstate *DevState, services DnsSdServices) *DnsSdPublisher {
	return &DnsSdPublisher{
		DevState: devstate,
		Services: services,
		fin:      make(chan struct{}),
	}
}

// Publish all services
func (publisher *DnsSdPublisher) Publish() error {
	var err error

	instance := publisher.instance(0)
	publisher.sysdep, err = newDnssdSysdep(instance,
		publisher.Services)

	if err != nil {
		return err
	}

	log_debug("+ DNS-SD: %s published", instance)

	publisher.finDone.Add(1)
	go publisher.goroutine()

	return nil
}

// Unpublish everything
func (publisher *DnsSdPublisher) Unpublish() {
	close(publisher.fin)
	publisher.finDone.Wait()

	publisher.sysdep.Close()

	log_debug("- DNS-SD: %s removed", publisher.instance(0))
}

// Build service instance name with optional collision-resolution suffix
func (publisher *DnsSdPublisher) instance(suffix int) string {
	if suffix == 0 {
		if publisher.DevState.DnsSdName == publisher.DevState.DnsSdOverride {
			return publisher.DevState.DnsSdName + " (USB)"
		} else {
			return publisher.DevState.DnsSdOverride
		}
	} else {
		return publisher.DevState.DnsSdName + fmt.Sprintf(" (USB %d)", suffix)
	}
}

// Event handling goroutine
func (publisher *DnsSdPublisher) goroutine() {
	defer publisher.finDone.Done()

	timer := time.NewTimer(time.Hour)
	timer.Stop()       // Not ticking now
	defer timer.Stop() // And cleanup at return

	var err error
	var suffix int

	instance := publisher.instance(0)
	for {
		fail := false

		select {
		case <-publisher.fin:
			return

		case status := <-publisher.sysdep.Chan():
			log_debug("  DNS-SD: %s", status)

			switch status {
			case DnsSdSuccess:
				if instance != publisher.DevState.DnsSdOverride {
					publisher.DevState.DnsSdOverride = instance
					publisher.DevState.Save()
				}

			case DnsSdCollision:
				suffix++
				fallthrough

			default:
				fail = true
				publisher.sysdep.Close()
			}

		case <-timer.C:
			instance = publisher.instance(suffix)
			publisher.sysdep, err = newDnssdSysdep(instance,
				publisher.Services)

			if err != nil {
				log_debug("+ DNS-SD: %s", err)
				fail = true
			}
		}

		if fail {
			timer.Reset(1 * time.Second)
		}
	}
}
