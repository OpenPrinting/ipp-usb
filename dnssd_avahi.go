/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * DNS-SD, Avahi-based system-dependent part
 *
 * +build linux
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
	"sync"
	"unsafe"
)

var (
	avahiInitLock     sync.Mutex
	avahiThreadedPoll *C.AvahiThreadedPoll
)

// dnssdSysdep represents a system-dependent
type dnssdSysdep struct {
	client *C.AvahiClient     // Avahi client
	egroup *C.AvahiEntryGroup // Avahi entry group
}

// newDnssdSysdep creates new dnssdSysdep instance
func newDnssdSysdep(instance string, services []DnsSdInfo) (
	*dnssdSysdep, error) {

	var err error
	var poll *C.AvahiPoll
	var rc C.int
	var client *C.AvahiClient
	var egroup *C.AvahiEntryGroup
	var proto, iface int

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
	client = C.avahi_client_new(
		poll,
		C.AVAHI_CLIENT_NO_FAIL,
		C.AvahiClientCallback(C.avahiClientCallback),
		nil,
		&rc,
	)

	if client == nil {
		goto AVAHI_ERROR
	}

	// Create entry group
	egroup = C.avahi_entry_group_new(
		client,
		C.AvahiEntryGroupCallback(C.avahiEntryGroupCallback),
		nil,
	)

	if egroup == nil {
		rc = C.avahi_client_errno(client)
		goto AVAHI_ERROR
	}

	// Compute iface and proto
	iface = C.AVAHI_IF_UNSPEC
	if Conf.LoopbackOnly {
		iface, err = Loopback()
		if err != nil {
			goto ERROR
		}
	}

	proto = C.AVAHI_PROTO_UNSPEC
	if !Conf.IpV6Enable {
		proto = C.AVAHI_PROTO_INET
	}

	// Populate entry group
	for _, svc := range services {
		c_svc_type := C.CString(svc.Type)

		var c_txt *C.AvahiStringList
		c_txt, err = avahiTxtRecord(svc.Txt)
		if err != nil {
			goto ERROR
		}

		rc = C.avahi_entry_group_add_service_strlst(
			egroup,
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

		C.free(unsafe.Pointer(c_svc_type))
		C.avahi_string_list_free(c_txt)

		if rc != C.AVAHI_OK {
			goto AVAHI_ERROR
		}
	}

	// Commit changes
	rc = C.avahi_entry_group_commit(egroup)
	if rc != C.AVAHI_OK {
		goto AVAHI_ERROR
	}

	// Create and return dnssdSysdep
	return &dnssdSysdep{client: client, egroup: egroup}, nil

AVAHI_ERROR:
	err = errors.New(C.GoString(C.avahi_strerror(rc)))

ERROR:
	if egroup != nil {
		C.avahi_entry_group_free(egroup)
	}

	if client != nil {
		C.avahi_client_free(client)
	}

	return nil, fmt.Errorf("AVAHI: %s", err)
}

// Close dnssdSysdep
func (sd *dnssdSysdep) Close() {
	// Synchronize with Avahi thread
	avahiThreadLock()
	defer avahiThreadUnlock()

	// Free everything
	C.avahi_entry_group_free(sd.egroup)
	C.avahi_client_free(sd.client)
}

// avahiTxtRecord converts DnsDsTxtRecord to AvahiStringList
func avahiTxtRecord(txt DnsDsTxtRecord) (*C.AvahiStringList, error) {
	var buf bytes.Buffer
	var list, prev *C.AvahiStringList

	for _, t := range txt {
		buf.Reset()
		buf.WriteString(t.Key)
		buf.WriteByte('=')
		buf.WriteString(t.Value)

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
	state C.AvahiClientState, data unsafe.Pointer) {

	switch state {
	case C.AVAHI_CLIENT_S_REGISTERING:
		log_debug("  AVAHI CLIENT: AVAHI_CLIENT_S_REGISTERING")
	case C.AVAHI_CLIENT_S_RUNNING:
		log_debug("  AVAHI CLIENT: AVAHI_CLIENT_S_RUNNING")
	case C.AVAHI_CLIENT_S_COLLISION:
		log_debug("  AVAHI CLIENT: AVAHI_CLIENT_S_COLLISION")
	case C.AVAHI_CLIENT_FAILURE:
		log_debug("  AVAHI CLIENT: AVAHI_CLIENT_FAILURE")
	case C.AVAHI_CLIENT_CONNECTING:
		log_debug("  AVAHI CLIENT: AVAHI_CLIENT_CONNECTING")
	}
}

// avahiEntryGroupCallback called by Avahi client to notify us about
// entry group state change
//
//export avahiEntryGroupCallback
func avahiEntryGroupCallback(egroup *C.AvahiEntryGroup,
	state C.AvahiEntryGroupState, data unsafe.Pointer) {
	switch state {
	case C.AVAHI_ENTRY_GROUP_UNCOMMITED:
		log_debug("  AVAHI_ENTRY_GROUP_UNCOMMITED")
	case C.AVAHI_ENTRY_GROUP_REGISTERING:
		log_debug("  AVAHI_ENTRY_GROUP_REGISTERING")
	case C.AVAHI_ENTRY_GROUP_ESTABLISHED:
		log_debug("  AVAHI_ENTRY_GROUP_ESTABLISHED")
	case C.AVAHI_ENTRY_GROUP_COLLISION:
		log_debug("  AVAHI_ENTRY_GROUP_COLLISION")
	case C.AVAHI_ENTRY_GROUP_FAILURE:
		log_debug("  AVAHI_ENTRY_GROUP_FAILURE")
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
