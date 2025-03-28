/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Common types for USB
 */

package main

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// UsbAddr represents an USB device address
type UsbAddr struct {
	Bus     int // The bus on which the device was detected
	Address int // The address of the device on the bus
}

// String returns a human-readable representation of UsbAddr
func (addr UsbAddr) String() string {
	return fmt.Sprintf("Bus %.3d Device %.3d", addr.Bus, addr.Address)
}

// Less returns true, if addr is "less" that addr2, for sorting
func (addr UsbAddr) Less(addr2 UsbAddr) bool {
	return addr.Bus < addr2.Bus ||
		(addr.Bus == addr2.Bus && addr.Address < addr2.Address)
}

// UsbAddrList represents a list of USB addresses
//
// For faster lookup and comparable logging, address list
// is always sorted in acceding order. To maintain this
// invariant, never modify list directly, and use the provided
// (*UsbAddrList) Add() function
type UsbAddrList []UsbAddr

// Add UsbAddr to UsbAddrList
func (list *UsbAddrList) Add(addr UsbAddr) {
	// Find the smallest index of address list
	// item which is greater or equal to the
	// newly inserted address
	//
	// Note, of "not found" case sort.Search()
	// returns len(*list)
	i := sort.Search(len(*list), func(n int) bool {
		return !(*list)[n].Less(addr)
	})

	// Check for duplicate
	if i < len(*list) && (*list)[i] == addr {
		return
	}

	// The simple case: all items are less
	// that newly added, so just append new
	// address to the end
	if i == len(*list) {
		*list = append(*list, addr)
		return
	}

	// Insert item in the middle
	*list = append(*list, (*list)[i])
	(*list)[i] = addr
}

// Find address in a list. Returns address index,
// if address is found, -1 otherwise
func (list UsbAddrList) Find(addr UsbAddr) int {
	i := sort.Search(len(list), func(n int) bool {
		return !list[n].Less(addr)
	})

	if i < len(list) && list[i] == addr {
		return i
	}

	return -1
}

// Diff computes a difference between two address lists,
// returning lists of elements to be added and to be removed
// to/from the list to convert it to the list2
func (list UsbAddrList) Diff(list2 UsbAddrList) (added, removed UsbAddrList) {
	// Note, there is no needs to sort added and removed
	// lists, they are already created sorted

	for _, a := range list2 {
		if list.Find(a) < 0 {
			added.Add(a)
		}
	}

	for _, a := range list {
		if list2.Find(a) < 0 {
			removed.Add(a)
		}
	}

	return
}

// UsbIfAddr represents a full "address" of the USB interface
type UsbIfAddr struct {
	UsbAddr     // Device address
	Num     int // Interface number within Config
	Alt     int // Number of alternate setting
	In, Out int // Input/output endpoint numbers
}

// String returns a human readable short representation of UsbIfAddr
func (ifaddr UsbIfAddr) String() string {
	return fmt.Sprintf("Bus %.3d Device %.3d Interface %d Alt %d",
		ifaddr.Bus,
		ifaddr.Address,
		ifaddr.Num,
		ifaddr.Alt,
	)
}

// UsbIfAddrList represents a list of USB interface addresses
type UsbIfAddrList []UsbIfAddr

// Add UsbIfAddr to UsbIfAddrList
func (list *UsbIfAddrList) Add(addr UsbIfAddr) {
	*list = append(*list, addr)
}

// UsbDeviceDesc represents an IPP-over-USB device descriptor
type UsbDeviceDesc struct {
	UsbAddr               // Device address
	Vendor  uint16        // USB Vendor ID
	Product uint16        // USB Device ID
	Config  int           // IPP-over-USB configuration
	IfAddrs UsbIfAddrList // IPP-over-USB interfaces
	IfDescs []UsbIfDesc   // Descriptors of all interfaces
}

// GetUsbDeviceInfo obtains UsbDeviceInfo by UsbDeviceDesc
// It may fail, if device cannot be opened
func (desc UsbDeviceDesc) GetUsbDeviceInfo() (UsbDeviceInfo, error) {
	dev, err := UsbOpenDevice(desc)
	if err == nil {
		defer dev.Close()
		return dev.UsbDeviceInfo()
	}
	return UsbDeviceInfo{}, err
}

// UsbIfDesc represents an USB interface descriptor
type UsbIfDesc struct {
	Vendor   uint16 // USB Vendor ID
	Product  uint16 // USB Device ID
	Config   int    // Configuration
	IfNum    int    // Interface number
	Alt      int    // Alternate setting
	Class    int    // Class
	SubClass int    // Subclass
	Proto    int    // Protocol
}

// IsIppOverUsb check if interface is IPP over USB
//
// FIXME. The matching rules must be configurable
func (ifdesc UsbIfDesc) IsIppOverUsb() bool {
	switch {
	// The classical combination, 7/1/4
	case ifdesc.Class == 7 && ifdesc.SubClass == 1 && ifdesc.Proto == 4:
		return true

	// Some HP devices use non-standard combination, 255/9/1
	//
	// This is valid at least with the following devices:
	//   HP LaserJet MFP M426fdn
	//   HP ColorLaserJet MFP M278-M281
	case ifdesc.Vendor == 0x03f0 &&
		ifdesc.Class == 255 && ifdesc.SubClass == 9 && ifdesc.Proto == 1:
		return true
	}

	return false
}

// UsbDeviceInfo represents USB device information
type UsbDeviceInfo struct {
	// Fields, directly decoded from USB
	Vendor       uint16          // Vendor ID
	Product      uint16          // Device ID
	SerialNumber string          // Device serial number
	Manufacturer string          // Manufacturer name
	ProductName  string          // Product name
	PortNum      int             // USB port number
	BasicCaps    UsbIppBasicCaps // Device basic capabilities
}

// UsbIppBasicCaps represents device basic capabilities bits,
// according to the IPP-USB specification, section 4.3
type UsbIppBasicCaps int

// Basic capabilities bits, see IPP-USB specification, section 4.3
const (
	UsbIppBasicCapsPrint UsbIppBasicCaps = 1 << iota
	UsbIppBasicCapsScan
	UsbIppBasicCapsFax
	UsbIppBasicCapsOther
	UsbIppBasicCapsAnyHTTP
)

// String returns a human-readable representation of UsbAddr
func (caps UsbIppBasicCaps) String() string {
	s := []string{}

	if caps&UsbIppBasicCapsPrint != 0 {
		s = append(s, "print")
	}

	if caps&UsbIppBasicCapsScan != 0 {
		s = append(s, "scan")
	}

	if caps&UsbIppBasicCapsFax != 0 {
		s = append(s, "fax")
	}

	if caps&UsbIppBasicCapsAnyHTTP != 0 {
		s = append(s, "http")
	}

	return strings.Join(s, ",")
}

// CheckMissed return a error, if UsbDeviceInfo misses some
// essential parameters.
//
// It check for the following parameters:
//
//   - Manufacturer
//   - ProductName
//   - SerialNumber
//
// If some of them missed, the appropriate error is returned.
func (info UsbDeviceInfo) CheckMissed() error {
	switch {
	case info.Manufacturer == "":
		return errors.New("missed Manufacturer string")
	case info.ProductName == "":
		return errors.New("missed ProductName string")
	case info.SerialNumber == "":
		return errors.New("missed SerialNumber string")
	}

	return nil
}

// MakeAndModel returns device Make and Model as a single
// string
func (info UsbDeviceInfo) MakeAndModel() string {
	mfg := strings.TrimSpace(info.Manufacturer)
	prod := strings.TrimSpace(info.ProductName)

	makeModel := prod
	if mfg != "" && !strings.HasPrefix(prod, mfg) {
		makeModel = mfg + " " + prod
	}

	return makeModel
}

// Ident returns device identification string, suitable as
// persistent state identifier
func (info UsbDeviceInfo) Ident() string {
	id := fmt.Sprintf("%4.4x-%4.4x", info.Vendor, info.Product)

	if info.SerialNumber != "" {
		id += "-" + info.SerialNumber
	}

	if model := info.MakeAndModel(); model != "" {
		id += "-" + model
	}

	id = strings.Map(func(c rune) rune {
		switch {
		case '0' <= c && c <= '9':
		case 'a' <= c && c <= 'z':
		case 'A' <= c && c <= 'Z':
		case c == '-' || c == '_':
		default:
			c = '-'
		}
		return c
	}, id)
	return id
}

// UUID generates device UUID in a case it is not available
// from IPP or eSCL
func (info UsbDeviceInfo) UUID() string {
	hash := sha1.New()

	// Arbitrary namespace UUID
	const namespace = "fe678de6-f422-467e-9f83-2354e26c3b41"

	hash.Write([]byte(namespace))
	hash.Write([]byte(info.Ident()))
	uuid := hash.Sum(nil)

	// UUID.Version = 5: Name-based with SHA1; see RFC4122, 4.1.3.
	uuid[6] &= 0x0f
	uuid[6] |= 0x5f

	// UUID.Variant = 0b10: see RFC4122, 4.1.1.
	uuid[8] &= 0x3F
	uuid[8] |= 0x80

	return fmt.Sprintf(
		"%.2x%.2x%.2x%.2x-%.2x%.2x-%.2x%.2x-%.2x%.2x-%.2x%.2x%.2x%.2x%.2x%.2x",
		uuid[0], uuid[1], uuid[2], uuid[3],
		uuid[4], uuid[5], uuid[6], uuid[7],
		uuid[8], uuid[9], uuid[10], uuid[11],
		uuid[12], uuid[13], uuid[14], uuid[15])
}

// Comment returns a short comment, describing a device
func (info UsbDeviceInfo) Comment() string {
	return info.MakeAndModel() + " serial=" + info.SerialNumber
}
