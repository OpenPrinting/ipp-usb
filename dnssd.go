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

// DNSSdTxtItem represents a single TXT record item
type DNSSdTxtItem struct {
	Key, Value string // TXT entry: Key=Value
	URL        bool   // It's an URL, hostname must be adjusted
}

// DNSSdTxtRecord represents a TXT record
type DNSSdTxtRecord []DNSSdTxtItem

// Add adds regular (non-URL) item to DNSSdTxtRecord
func (txt *DNSSdTxtRecord) Add(key, value string) {
	*txt = append(*txt, DNSSdTxtItem{key, value, false})
}

// AddURL adds URL item to DNSSdTxtRecord
func (txt *DNSSdTxtRecord) AddURL(key, value string) {
	*txt = append(*txt, DNSSdTxtItem{key, value, true})
}

// IfNotEmpty adds item to DNSSdTxtRecord if its value is not empty
//
// It returns true if item was actually added, false otherwise
func (txt *DNSSdTxtRecord) IfNotEmpty(key, value string) bool {
	if value != "" {
		txt.Add(key, value)
		return true
	}
	return false
}

// URLIfNotEmpty works as IfNotEmpty, but for URLs
func (txt *DNSSdTxtRecord) URLIfNotEmpty(key, value string) bool {
	if value != "" {
		txt.AddURL(key, value)
		return true
	}
	return false
}

// export DNSSdTxtRecord into Avahi format
func (txt DNSSdTxtRecord) export() [][]byte {
	var exported [][]byte

	// Note, for a some strange reason, Avahi published
	// TXT record in reverse order, so compensate it here
	for i := len(txt) - 1; i >= 0; i-- {
		item := txt[i]
		exported = append(exported, []byte(item.Key+"="+item.Value))
	}

	return exported
}

// DNSSdSvcInfo represents a DNS-SD service information
type DNSSdSvcInfo struct {
	Type string         // Service type, i.e. "_ipp._tcp"
	Port int            // TCP port
	Txt  DNSSdTxtRecord // TXT record
}

// DNSSdServices represents a collection of DNS-SD services
type DNSSdServices []DNSSdSvcInfo

// Add DNSSdSvcInfo to DNSSdServices
func (services *DNSSdServices) Add(srv DNSSdSvcInfo) {
	*services = append(*services, srv)
}

// DNSSdPublisher represents a DNS-SD service publisher
// One publisher may publish multiple services unser the
// same Service Instance Name
type DNSSdPublisher struct {
	Log      *Logger        // Device's logger
	DevState *DevState      // Device persistent state
	Services DNSSdServices  // Registered services
	fin      chan struct{}  // Closed to terminate publisher goroutine
	finDone  sync.WaitGroup // To wait for goroutine termination
	sysdep   *dnssdSysdep   // System-dependent stuff
}

// DNSSdStatus represents DNS-SD publisher status
type DNSSdStatus int

const (
	DNSSdNoStatus  DNSSdStatus = iota // Invalid status
	DNSSdCollision                    // Service instance name collision
	DNSSdFailure                      // Publisher failed
	DNSSdSuccess                      // Services successfully published
)

// String returns human-readable representation of DNSSdStatus
func (status DNSSdStatus) String() string {
	switch status {
	case DNSSdNoStatus:
		return "DNSSdNoStatus"
	case DNSSdCollision:
		return "DNSSdCollision"
	case DNSSdFailure:
		return "DNSSdFailure"
	case DNSSdSuccess:
		return "DNSSdSuccess"
	}

	return fmt.Sprintf("Unknown DNSSdStatus %d", status)
}

// NewDNSSdPublisher creates new DNSSdPublisher
//
// Service instance name comes from the DevState, and if
// name changes as result of name collision resolution,
// DevState will be updated
func NewDNSSdPublisher(log *Logger,
	devstate *DevState, services DNSSdServices) *DNSSdPublisher {

	return &DNSSdPublisher{
		Log:      log,
		DevState: devstate,
		Services: services,
		fin:      make(chan struct{}),
	}
}

// Publish all services
func (publisher *DNSSdPublisher) Publish() error {
	var err error

	instance := publisher.instance(0)
	publisher.sysdep, err = newDnssdSysdep(publisher.Log,
		instance, publisher.Services)

	if err != nil {
		return err
	}

	publisher.Log.Info('+', "DNS-SD: %s: publishing requested", instance)

	publisher.finDone.Add(1)
	go publisher.goroutine()

	return nil
}

// Unpublish everything
func (publisher *DNSSdPublisher) Unpublish() {
	close(publisher.fin)
	publisher.finDone.Wait()

	publisher.sysdep.Close()

	publisher.Log.Info('-', "DNS-SD: %s: removed", publisher.instance(0))
}

// Build service instance name with optional collision-resolution suffix
func (publisher *DNSSdPublisher) instance(suffix int) string {
	switch {
	// This happens when we try to resolve name conflict
	case suffix != 0:
		return publisher.DevState.DNSSdName + fmt.Sprintf(" (USB %d)", suffix)

	// This happens when we've just initialized or reset DNSSdOverride,
	// so append "(USB)" suffix
	case publisher.DevState.DNSSdName == publisher.DevState.DNSSdOverride:
		return publisher.DevState.DNSSdName + " (USB)"

	// Otherwise, DNSSdOverride contains saved conflict-resolved device name
	default:
		return publisher.DevState.DNSSdOverride
	}
}

// Event handling goroutine
func (publisher *DNSSdPublisher) goroutine() {
	// Catch panics to log
	defer func() {
		v := recover()
		if v != nil {
			Log.Panic(v)
		}
	}()

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
			switch status {
			case DNSSdSuccess:
				publisher.Log.Info(' ', "DNS-SD: %s: published", instance)
				if instance != publisher.DevState.DNSSdOverride {
					publisher.DevState.DNSSdOverride = instance
					publisher.DevState.Save()
				}

			case DNSSdCollision:
				publisher.Log.Error(' ', "DNS-SD: %s: name collision",
					instance)
				suffix++
				fallthrough

			case DNSSdFailure:
				publisher.Log.Error(' ', "DNS-SD: %s: publishing failed",
					instance)

				fail = true
				publisher.sysdep.Close()

			default:
				publisher.Log.Error(' ', "DNS-SD: %s: unknown event %s",
					instance, status)
			}

		case <-timer.C:
			instance = publisher.instance(suffix)
			publisher.sysdep, err = newDnssdSysdep(publisher.Log,
				instance, publisher.Services)

			if err != nil {
				publisher.Log.Error('!', "DNS-SD: %s: %s", instance, err)
				fail = true
			}
		}

		if fail {
			timer.Reset(1 * time.Second)
		}
	}
}
