/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Tests for usbcommon.go
 */

package main

import (
	"testing"
)

// Check if two UsbAddrList are equal
func equalUsbAddrList(l1, l2 UsbAddrList) bool {
	if len(l1) != len(l2) {
		return false
	}

	for i := range l1 {
		if l1[i] != l2[i] {
			return false
		}
	}

	return true
}

// Make UsbAddrList from individual addresses
func makeUsbAddrList(addrs ...UsbAddr) UsbAddrList {
	l := UsbAddrList{}

	for _, a := range addrs {
		l.Add(a)
	}

	return l
}

// Test (*UsbAddrList)Add() against (*UsbAddrList)Find()
func TestUsbAddrListAddFind(t *testing.T) {
	a1 := UsbAddr{0, 1}
	a2 := UsbAddr{0, 2}
	a3 := UsbAddr{0, 3}

	l1 := makeUsbAddrList(a1, a2)

	if l1.Find(a1) < 0 {
		t.Fail()
	}

	if l1.Find(a2) < 0 {
		t.Fail()
	}

	if l1.Find(a3) >= 0 {
		t.Fail()
	}
}

// Test that (*UsbAddrList)Add() is commutative operation
func TestUsbAddrListAddCommutative(t *testing.T) {
	a1 := UsbAddr{0, 1}
	a2 := UsbAddr{0, 2}

	l1 := UsbAddrList{}
	l2 := UsbAddrList{}

	l1.Add(a1)
	l1.Add(a2)

	l2.Add(a2)
	l2.Add(a1)

	if !equalUsbAddrList(l1, l2) {
		t.Fail()
	}
}

// Test (*UsbAddrList) Diff()
func TestUsbAddrListDiff(t *testing.T) {
	a1 := UsbAddr{0, 1}
	a2 := UsbAddr{0, 2}
	a3 := UsbAddr{0, 3}

	l1 := makeUsbAddrList(a2, a3)
	l2 := makeUsbAddrList(a1, a3)

	added, removed := l1.Diff(l2)

	if !equalUsbAddrList(added, makeUsbAddrList(a1)) {
		t.Fail()
	}

	if !equalUsbAddrList(removed, makeUsbAddrList(a2)) {
		t.Fail()
	}

	added, removed = l2.Diff(l1)

	if !equalUsbAddrList(removed, makeUsbAddrList(a1)) {
		t.Fail()
	}

	if !equalUsbAddrList(added, makeUsbAddrList(a2)) {
		t.Fail()
	}
}
