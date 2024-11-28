/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Tests for device-specific quirks
 */

package main

import (
	"reflect"
	"testing"
	"time"
)

// TestQuirksLookup tests lookup of various parameters
func TestQuirksLookup(t *testing.T) {
	const path = "testdata/quirks"

	// Load quirks
	qset, err := LoadQuirksSet(path)
	if err != nil {
		t.Fatalf("LoadQuirksSet(%q): %s", path, err)
	}

	// Test loaded values against expected
	type testData struct {
		model  string                   // Model name
		param  string                   // Parameter (quirk) name
		get    func(Quirks) interface{} // Lookup function
		match  string                   // Expected match
		value  interface{}              // Expected value
		origin string                   // Expected origin
	}

	tests := []testData{
		// Default values for unknown device
		{
			model: "Unknown Device",
			param: QuirkNmBlacklist,
			get: func(quirks Quirks) interface{} {
				return quirks.GetBlacklist()
			},
			match:  "*",
			value:  false,
			origin: "testdata/quirks/default.conf:4",
		},

		{
			model: "Unknown Device",
			param: QuirkNmBuggyIppResponses,
			get: func(quirks Quirks) interface{} {
				return quirks.GetBuggyIppRsp()
			},
			match:  "*",
			value:  QuirkBuggyIppRspReject,
			origin: "default",
		},

		{
			model: "Unknown Device",
			param: QuirkNmDisableFax,
			get: func(quirks Quirks) interface{} {
				return quirks.GetDisableFax()
			},
			match:  "*",
			value:  false,
			origin: "default",
		},

		{
			model: "Unknown Device",
			param: QuirkNmIgnoreIppStatus,
			get: func(quirks Quirks) interface{} {
				return quirks.GetIgnoreIppStatus()
			},
			match:  "*",
			value:  false,
			origin: "default",
		},

		{
			model: "Unknown Device",
			param: QuirkNmInitDelay,
			get: func(quirks Quirks) interface{} {
				return quirks.GetInitDelay()
			},
			match:  "*",
			value:  time.Duration(0),
			origin: "default",
		},

		{
			model: "Unknown Device",
			param: QuirkNmInitReset,
			get: func(quirks Quirks) interface{} {
				return quirks.GetInitReset()
			},
			match:  "*",
			value:  QuirkResetNone,
			origin: "default",
		},

		{
			model: "Unknown Device",
			param: QuirkNmRequestDelay,
			get: func(quirks Quirks) interface{} {
				return quirks.GetRequestDelay()
			},
			match:  "*",
			value:  time.Duration(0),
			origin: "default",
		},

		{
			model: "Unknown Device",
			param: QuirkNmUsbMaxInterfaces,
			get: func(quirks Quirks) interface{} {
				return quirks.GetUsbMaxInterfaces()
			},
			match:  "*",
			value:  uint(0),
			origin: "default",
		},

		// Quirks for some known devices
		{
			model: "HP ScanJet Pro 4500 fn1",
			param: QuirkNmUsbMaxInterfaces,
			get: func(quirks Quirks) interface{} {
				return quirks.GetUsbMaxInterfaces()
			},
			match:  "HP ScanJet Pro 4500 fn1",
			value:  uint(1),
			origin: "testdata/quirks/HP.conf:16",
		},

		{
			model: "HP ScanJet Pro 4500 fn1",
			param: QuirkNmRequestDelay,
			get: func(quirks Quirks) interface{} {
				return quirks.GetRequestDelay()
			},
			match:  "*",
			value:  time.Duration(0),
			origin: "default",
		},
	}

	for _, test := range tests {
		quirks := qset.MatchByModelName(test.model)
		q := quirks.Get(test.param)
		v := test.get(quirks)

		if !reflect.DeepEqual(v, test.value) {
			t.Errorf("model: %q, param: %q: value mismatch\n"+
				"expected: %s(%v)\n"+
				"present:  %s(%v)",
				test.model, test.param,
				reflect.TypeOf(test.value), test.value,
				reflect.TypeOf(v), v)
		}

		if q.Match != test.match {
			t.Errorf("model: %q, param: %q: match mismatch\n"+
				"expected: %q\n"+
				"present:  %q",
				test.model, test.param, test.match, q.Match)
		}

		if q.Origin != test.origin {
			t.Errorf("model: %q, param: %q: origin mismatch\n"+
				"expected: %q\n"+
				"present:  %q",
				test.model, test.param, test.origin, q.Origin)
		}
	}
}

// TestQuirksSetLoad tests LoadQuirksSet
func TestQuirksSetLoad(t *testing.T) {
	const path = "testdata/quirks"
	const badPath = path + "-not-exist"

	// Try non-existent directory
	_, err := LoadQuirksSet(badPath)
	if err != nil {
		t.Fatalf("LoadQuirksSet(%q): %s", badPath, err)
	}

	// Try test data
	_, err = LoadQuirksSet(path)
	if err != nil {
		t.Fatalf("LoadQuirksSet(%q): %s", path, err)
	}
}
