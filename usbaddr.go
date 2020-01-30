/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Manipulations with USB addresses
 */

package main

import (
	"fmt"
	"sort"

	"github.com/google/gousb"
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

// MapKey() returns a string suitable as a map key
// It is not even guaranteed that this string is printable
func (addr UsbAddr) MapKey() string {
	return fmt.Sprintf("%d:%d", addr.Bus, addr.Address)
}

// Compare 2 addresses, for sorting
func (addr UsbAddr) Less(addr2 UsbAddr) bool {
	return addr.Bus < addr2.Bus ||
		(addr.Bus == addr2.Bus && addr.Address < addr2.Address)
}

// Open device by address
func (addr UsbAddr) Open() (*gousb.Device, error) {
	found := false
	devs, err := usbCtx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		if found {
			return false
		}

		return addr.Bus == desc.Bus && addr.Address == desc.Address
	})

	if len(devs) != 0 {
		devs[0].SetAutoDetach(true)
		return devs[0], nil
	}

	if err == nil {
		err = gousb.ErrorNotFound
	}

	return nil, fmt.Errorf("%s: %s", addr, err)
}

// UsbAddrList represents a list of USB addresses
//
// For faster lookup and comparable logging, address list
// is always sorted in acceding order. To maintain this
// invariant, never modify list directly, and use the provided
// (*UsbAddrList) Add() function
type UsbAddrList []UsbAddr

// Build UsbAddrList, collection all IPP-over-USB devices
func BuildUsbAddrList() UsbAddrList {
	var list UsbAddrList

	usbCtx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		if len(GetUsbIfAddrs(desc)) >= 2 {
			list.Add(UsbAddr{desc.Bus, desc.Address})
		}
		return false
	})

	return list
}

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
// to/from the list1 to convert it to the list2
func (list1 UsbAddrList) Diff(list2 UsbAddrList) (added, removed UsbAddrList) {
	// Note, there is no needs to sort added and removed
	// lists, they are already created sorted

	for _, a := range list2 {
		if list1.Find(a) < 0 {
			added.Add(a)
		}
	}

	for _, a := range list1 {
		if list2.Find(a) < 0 {
			removed.Add(a)
		}
	}

	return
}

// UsbIfAddr represents a full "address" of the USB interface
type UsbIfAddr struct {
	UsbAddr                     // Device address
	CfgNum  int                 // Config number within device
	Num     int                 // Interface number within Config
	Alt     int                 // Number of alternate setting
	In, Out *gousb.EndpointDesc // Input/output endpoints
}

// String returns a human readable short representation of UsbIfAddr
func (ifaddr UsbIfAddr) String() string {
	return fmt.Sprintf("Bus %.3d Device %.3d Config %d Interface %d Alt %d",
		ifaddr.Bus,
		ifaddr.Address,
		ifaddr.CfgNum,
		ifaddr.Num,
		ifaddr.Alt,
	)
}

// Open the particular interface on device
func (ifaddr *UsbIfAddr) Open(dev *gousb.Device) (*gousb.Interface, error) {
	conf, err := dev.Config(ifaddr.CfgNum)
	if err != nil {
		return nil, err
	}

	iface, err := conf.Interface(ifaddr.Num, ifaddr.Alt)
	if err != nil {
		return nil, err
	}

	return iface, nil
}

// UsbIfAddrList represents a list of USB interface addresses
type UsbIfAddrList []UsbIfAddr

// Add UsbIfAddr to UsbIfAddrList
func (list *UsbIfAddrList) Add(addr UsbIfAddr) {
	*list = append(*list, addr)
}

// GetUsbIfAddrs returns list of IPP over USB interfaces on device
func GetUsbIfAddrs(desc *gousb.DeviceDesc) UsbIfAddrList {
	var list UsbIfAddrList

	for cfgNum, conf := range desc.Configs {
		for ifNum, iface := range conf.Interfaces {
			for altNum, alt := range iface.AltSettings {
				if alt.Class == gousb.ClassPrinter &&
					alt.SubClass == 1 &&
					alt.Protocol == 4 {

					// Build address
					addr := UsbIfAddr{
						UsbAddr: UsbAddr{desc.Bus, desc.Address},
						CfgNum:  cfgNum,
						Num:     ifNum,
						Alt:     altNum,
					}

					// Find in/out endpoins
					for _, ep := range alt.Endpoints {
						switch ep.Direction {
						case gousb.EndpointDirectionIn:
							if addr.In == nil {
								ep2 := ep
								addr.In = &ep2
							}
						case gousb.EndpointDirectionOut:
							if addr.Out == nil {
								ep2 := ep
								addr.Out = &ep2
							}
						}
					}

					// Add address to the list
					if addr.In != nil && addr.Out != nil {
						list.Add(addr)
					}
				}
			}
		}
	}

	return list
}
