/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * DNS-SD publisher: system-independent stuff
 */

package main

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
	Instance string        // Service Instance Name
	Services DnsSdServices // Registered services
	sysdep   *dnssdSysdep  // System-dependent stuff
}

// NewDnsSdPublisher creates new DnsSdPublisher
func NewDnsSdPublisher(services DnsSdServices) *DnsSdPublisher {
	return &DnsSdPublisher{
		Services: services,
	}
}

// Unpublish everything
func (publisher *DnsSdPublisher) Unpublish() {
	publisher.sysdep.Close()
}

// Publish all services
func (publisher *DnsSdPublisher) Publish(instance string) error {
	var err error

	publisher.Instance = instance
	publisher.sysdep, err = newDnssdSysdep(publisher.Instance,
		publisher.Services)

	return err
}
