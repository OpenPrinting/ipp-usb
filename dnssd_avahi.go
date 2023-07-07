// +build linux freebsd

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

// dnssdSysdepErr implements error interface on a top of
// Avahi error codes
type dnssdSysdepErr C.int

// Error returns error string for the dnssdSysdepErr
func (err dnssdSysdepErr) Error() string {
	return "Avahi error: " + C.GoString(C.avahi_strerror(C.int(err)))
}

// newDnssdSysdep creates new dnssdSysdep instance
func newDnssdSysdep(log *Logger, instance string,
	services DNSSdServices) *dnssdSysdep {

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

	// Obtain index of loopback interface
	loopback, err := Loopback()
	if err != nil {
		goto ERROR // Very unlikely to happen
	}

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
		iface = loopback
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
		var cTxt *C.AvahiStringList
		cTxt, err = sysdep.avahiTxtRecord(svc.Port, svc.Txt)
		if err != nil {
			goto ERROR
		}

		// Prepare C strings for service instance and type
		cSvcType := C.CString(svc.Type)

		var cInstance *C.char
		if svc.Instance != "" {
			cInstance = C.CString(svc.Instance)
		} else {
			cInstance = C.CString(instance)
		}

		// Handle loopback-only mode
		ifaceInUse := iface
		if svc.Loopback {
			ifaceInUse = loopback
		}

		// Register service type
		rc = C.avahi_entry_group_add_service_strlst(
			sysdep.egroup,
			C.AvahiIfIndex(ifaceInUse),
			C.AvahiProtocol(proto),
			0,
			cInstance,
			cSvcType,
			nil, // Domain
			nil, // Host
			C.uint16_t(svc.Port),
			cTxt,
		)

		// Register subtypes, if any
		for _, subtype := range svc.SubTypes {
			if rc != C.AVAHI_OK {
				break
			}

			sysdep.log.Debug(' ', "DNS-SD: +subtype: %q", subtype)

			cSubtype := C.CString(subtype)
			rc = C.avahi_entry_group_add_service_subtype(
				sysdep.egroup,
				C.AvahiIfIndex(ifaceInUse),
				C.AvahiProtocol(proto),
				0,
				cInstance,
				cSvcType,
				nil,
				cSubtype,
			)
			C.free(unsafe.Pointer(cSubtype))

		}

		// Release C memory
		C.free(unsafe.Pointer(cInstance))
		C.free(unsafe.Pointer(cSvcType))
		C.avahi_string_list_free(cTxt)

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
	return sysdep

	// Error: cleanup and exit
AVAHI_ERROR:
	err = dnssdSysdepErr(rc)
ERROR:

	// Raise an error event
	sysdep.log.Error(' ', "DNS-SD: %s: %s", sysdep.instance, err)
	sysdep.haltLocked()

	if err == dnssdSysdepErr(C.AVAHI_ERR_COLLISION) {
		sysdep.notify(DNSSdCollision)
	} else {
		sysdep.notify(DNSSdFailure)
	}

	return sysdep
}

// Halt dnssdSysdep
//
// It cancel all activity related to the dnssdSysdep instance,
// but sysdep.Chan() remains valid, though no notifications
// will be pushed there anymore
func (sysdep *dnssdSysdep) Halt() {
	avahiThreadLock()
	sysdep.haltLocked()
	avahiThreadUnlock()
}

// Get status change notification channel
func (sysdep *dnssdSysdep) Chan() <-chan DNSSdStatus {
	return sysdep.statusChan
}

// Halt dnssdSysdep -- internal version
//
// Must be called under avahiThreadLock
// Can be used with semi-constructed dnssdSysdep
func (sysdep *dnssdSysdep) haltLocked() {
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
		// This is host name collision. We can't recover
		// it here, so lets consider it as DNSSdFailure
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
