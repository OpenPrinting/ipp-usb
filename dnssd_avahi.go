// +build linux

/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * DNS-SD publisher: Avahi-based system-dependent part
 */

package main

// #cgo pkg-config: avahi-client
//
// #include <stdlib.h>
// #include <avahi-client/publish.h>
// #include <avahi-common/error.h>
// #include <avahi-common/thread-watch.h>
// #include <avahi-common/watch.h>
//
// void avahiClientCallback(AvahiClient*, AvahiClientState, void*);
// void avahiEntryGroupCallback(AvahiEntryGroup*, AvahiEntryGroupState, void*);
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"unsafe"
)

var (
	avahiInitLock     sync.Mutex
	avahiThreadedPoll *C.AvahiThreadedPoll
	avahiClientMap    = make(map[*C.AvahiClient]*dnssdSysdep)
	avahiEgroupMap    = make(map[*C.AvahiEntryGroup]*dnssdSysdep)
)

// dnssdSysdep represents a system-dependent DNS-SD advertiser
type dnssdSysdep struct {
	log        *Logger            // Device's logger
	instance   string             // Service Instance Name
	fqdn       string             // Host's fully-qualified domain name
	client     *C.AvahiClient     // Avahi client
	egroup     *C.AvahiEntryGroup // Avahi entry group
	statusChan chan DNSSdStatus   // Status notifications channel
}

// newDnssdSysdep creates new dnssdSysdep instance
func newDnssdSysdep(log *Logger, instance string, services DNSSdServices) (
	*dnssdSysdep, error) {

	log.Debug(' ', "DNS-SD: %s: trying", instance)

	var err error
	var poll *C.AvahiPoll
	var rc C.int
	var proto, iface int

	sysdep := &dnssdSysdep{
		log:        log,
		instance:   instance,
		statusChan: make(chan DNSSdStatus, 10),
	}

	c_instance := C.CString(instance)
	defer C.free(unsafe.Pointer(c_instance))

	// Obtain AvahiPoll
	poll, err = avahiGetPoll()
	if err != nil {
		goto ERROR
	}

	// Synchronize with Avahi thread
	avahiThreadLock()
	defer avahiThreadUnlock()

	// Create Avahi client
	sysdep.client = C.avahi_client_new(
		poll,
		C.AVAHI_CLIENT_NO_FAIL,
		C.AvahiClientCallback(C.avahiClientCallback),
		nil,
		&rc,
	)

	if sysdep.client == nil {
		goto AVAHI_ERROR
	}

	avahiClientMap[sysdep.client] = sysdep

	sysdep.fqdn = C.GoString(C.avahi_client_get_host_name_fqdn(sysdep.client))
	sysdep.log.Debug(' ', "DNS-SD: FQDN: %q", sysdep.fqdn)

	// Create entry group
	sysdep.egroup = C.avahi_entry_group_new(
		sysdep.client,
		C.AvahiEntryGroupCallback(C.avahiEntryGroupCallback),
		nil,
	)

	if sysdep.egroup == nil {
		rc = C.avahi_client_errno(sysdep.client)
		goto AVAHI_ERROR
	}

	avahiEgroupMap[sysdep.egroup] = sysdep

	// Compute iface and proto, adjust fqdn
	iface = C.AVAHI_IF_UNSPEC
	if Conf.LoopbackOnly {
		iface, err = Loopback()
		if err != nil {
			goto ERROR
		}

		old := sysdep.fqdn
		sysdep.fqdn = "localhost"
		sysdep.log.Debug(' ', "DNS-SD: FQDN: %q->%q", old, sysdep.fqdn)
	}

	proto = C.AVAHI_PROTO_UNSPEC
	if !Conf.IPV6Enable {
		proto = C.AVAHI_PROTO_INET
	}

	// Populate entry group
	for _, svc := range services {
		// Prepare TXT record
		var c_txt *C.AvahiStringList
		c_txt, err = sysdep.avahiTxtRecord(svc.Port, svc.Txt)
		if err != nil {
			goto ERROR
		}

		// Register service type
		c_svc_type := C.CString(svc.Type)

		rc = C.avahi_entry_group_add_service_strlst(
			sysdep.egroup,
			C.AvahiIfIndex(iface),
			C.AvahiProtocol(proto),
			0,
			c_instance,
			c_svc_type,
			nil, // Domain
			nil, // Host
			C.uint16_t(svc.Port),
			c_txt,
		)

		// Register subtypes, if any
		for _, subtype := range svc.SubTypes {
			if rc != C.AVAHI_OK {
				break
			}

			sysdep.log.Debug(' ', "DNS-SD: +subtype: %q", subtype)

			c_subtype := C.CString(subtype)
			rc = C.avahi_entry_group_add_service_subtype(
				sysdep.egroup,
				C.AvahiIfIndex(iface),
				C.AvahiProtocol(proto),
				0,
				c_instance,
				c_svc_type,
				nil,
				c_subtype,
			)
			C.free(unsafe.Pointer(c_subtype))

		}

		// Release C memory
		C.free(unsafe.Pointer(c_svc_type))
		C.avahi_string_list_free(c_txt)

		// Check for Avahi error
		if rc != C.AVAHI_OK {
			goto AVAHI_ERROR
		}
	}

	// Commit changes
	rc = C.avahi_entry_group_commit(sysdep.egroup)
	if rc != C.AVAHI_OK {
		goto AVAHI_ERROR
	}

	// Create and return dnssdSysdep
	return sysdep, nil

AVAHI_ERROR:
	// Report name collision as event rather that error
	if rc == C.AVAHI_ERR_COLLISION {
		sysdep.notify(DNSSdCollision)
		return sysdep, nil
	}

	err = errors.New(C.GoString(C.avahi_strerror(rc)))

ERROR:
	sysdep.destroy()
	return nil, fmt.Errorf("AVAHI: %s", err)
}

// Close dnssdSysdep
//
// It cancel all activity related to the dnssdSysdep instance,
// but sysdep.Chan() remains valid, though no notifications
// will be pushed there anymore
func (sysdep *dnssdSysdep) Close() {
	avahiThreadLock()
	sysdep.destroy()
	avahiThreadUnlock()
}

// Get status change notification channel
func (sysdep *dnssdSysdep) Chan() <-chan DNSSdStatus {
	return sysdep.statusChan
}

// destroy dnssdSysdep
// Must be called under avahiThreadLock
// Can be used with semi-constructed dnssdSysdep
func (sysdep *dnssdSysdep) destroy() {
	// Free all Avahi stuff
	if sysdep.egroup != nil {
		C.avahi_entry_group_free(sysdep.egroup)
		delete(avahiEgroupMap, sysdep.egroup)
		sysdep.egroup = nil
	}

	if sysdep.client != nil {
		C.avahi_client_free(sysdep.client)
		delete(avahiClientMap, sysdep.client)
		sysdep.client = nil
	}

	// Drain status channel
	for len(sysdep.statusChan) > 0 {
		<-sysdep.statusChan
	}
}

// Push status change notification
func (sysdep *dnssdSysdep) notify(status DNSSdStatus) {
	sysdep.statusChan <- status
}

// avahiTxtRecord converts DNSSdTxtRecord to AvahiStringList
func (sysdep *dnssdSysdep) avahiTxtRecord(port int, txt DNSSdTxtRecord) (
	*C.AvahiStringList, error) {
	var buf bytes.Buffer
	var list, prev *C.AvahiStringList

	for _, t := range txt {
		buf.Reset()
		buf.WriteString(t.Key)
		buf.WriteByte('=')

		if !t.URL || sysdep.fqdn == "" {
			buf.WriteString(t.Value)
		} else {
			value := t.Value
			if parsed, err := url.Parse(value); err == nil && parsed.IsAbs() {
				parsed.Host = sysdep.fqdn
				if port != 0 {
					parsed.Host += fmt.Sprintf(":%d", port)
				}

				value = parsed.String()
			}
			buf.WriteString(value)
		}

		b := buf.Bytes()

		prev, list = list, C.avahi_string_list_add_arbitrary(
			list,
			(*C.uint8_t)(unsafe.Pointer(&b[0])),
			C.size_t(len(b)),
		)

		if list == nil {
			C.avahi_string_list_free(prev)
			return nil, ErrNoMemory
		}
	}

	return C.avahi_string_list_reverse(list), nil
}

// avahiClientCallback called by Avahi client to notify us about
// client state change
//
//export avahiClientCallback
func avahiClientCallback(client *C.AvahiClient,
	state C.AvahiClientState, _ unsafe.Pointer) {

	sysdep := avahiClientMap[client]
	if sysdep == nil {
		return
	}

	status := DNSSdNoStatus
	event := ""

	switch state {
	case C.AVAHI_CLIENT_S_REGISTERING:
		event = "AVAHI_CLIENT_S_REGISTERING"
	case C.AVAHI_CLIENT_S_RUNNING:
		event = "AVAHI_CLIENT_S_RUNNING"
	case C.AVAHI_CLIENT_S_COLLISION:
		event = "AVAHI_CLIENT_S_COLLISION"
		status = DNSSdFailure
	case C.AVAHI_CLIENT_FAILURE:
		event = "AVAHI_CLIENT_FAILURE"
		status = DNSSdFailure
	case C.AVAHI_CLIENT_CONNECTING:
		event = "AVAHI_CLIENT_CONNECTING"
	default:
		event = fmt.Sprintf("Unknown event %d", state)
	}

	sysdep.log.Debug(' ', "DNS-SD: %s: %s", sysdep.instance, event)
	if status != DNSSdNoStatus {
		sysdep.notify(status)
	}
}

// avahiEntryGroupCallback called by Avahi client to notify us about
// entry group state change
//
//export avahiEntryGroupCallback
func avahiEntryGroupCallback(egroup *C.AvahiEntryGroup,
	state C.AvahiEntryGroupState, _ unsafe.Pointer) {

	sysdep := avahiEgroupMap[egroup]
	if sysdep == nil {
		return
	}

	status := DNSSdNoStatus
	event := ""

	switch state {
	case C.AVAHI_ENTRY_GROUP_UNCOMMITED:
		event = "AVAHI_ENTRY_GROUP_UNCOMMITED"
	case C.AVAHI_ENTRY_GROUP_REGISTERING:
		event = "AVAHI_ENTRY_GROUP_REGISTERING"
	case C.AVAHI_ENTRY_GROUP_ESTABLISHED:
		event = "AVAHI_ENTRY_GROUP_ESTABLISHED"
		status = DNSSdSuccess
	case C.AVAHI_ENTRY_GROUP_COLLISION:
		event = "AVAHI_ENTRY_GROUP_COLLISION"
		status = DNSSdCollision
	case C.AVAHI_ENTRY_GROUP_FAILURE:
		event = "AVAHI_ENTRY_GROUP_FAILURE"
		status = DNSSdFailure
	}

	sysdep.log.Debug(' ', "DNS-SD: %s: %s", sysdep.instance, event)
	if status != DNSSdNoStatus {
		sysdep.notify(status)
	}
}

// avahiGetPoll returns pointer to AvahiPoll
// Avahi helper thread is created on demand
func avahiGetPoll() (*C.AvahiPoll, error) {
	avahiInitLock.Lock()
	defer avahiInitLock.Unlock()

	if avahiThreadedPoll == nil {
		avahiThreadedPoll = C.avahi_threaded_poll_new()
		if avahiThreadedPoll == nil {
			return nil, errors.New("initialization failed, not enough memory")
		}

		C.avahi_threaded_poll_start(avahiThreadedPoll)
	}

	return C.avahi_threaded_poll_get(avahiThreadedPoll), nil
}

// Lock Avahi thread
func avahiThreadLock() {
	C.avahi_threaded_poll_lock(avahiThreadedPoll)
}

// Unlock Avahi thread
func avahiThreadUnlock() {
	C.avahi_threaded_poll_unlock(avahiThreadedPoll)
}
