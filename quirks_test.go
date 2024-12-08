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
			param: QuirkNmInitTimeout,
			get: func(quirks Quirks) interface{} {
				return quirks.GetInitTimeout()
			},
			match:  "*",
			value:  DevInitTimeout,
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

		{
			model: "Unknown Device",
			param: QuirkNmZlpRecvHack,
			get: func(quirks Quirks) interface{} {
				return quirks.GetZlpRecvHack()
			},
			match:  "*",
			value:  false,
			origin: "default",
		},

		{
			model: "Unknown Device",
			param: QuirkNmZlpSend,
			get: func(quirks Quirks) interface{} {
				return quirks.GetZlpSend()
			},
			match:  "*",
			value:  false,
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

// TestQuirksParsers tests parsers for quirks
func TestQuirksParsers(t *testing.T) {
	type testData struct {
		parser func(*Quirk) error // Parser to test
		input  string             // Input string
		value  interface{}        // Expected output value
		err    string             // Or expected error
	}

	tests := []testData{
		// parseBool
		{
			parser: (*Quirk).parseBool,
			input:  "true",
			value:  true,
		},

		{
			parser: (*Quirk).parseBool,
			input:  "false",
			value:  false,
		},

		{
			parser: (*Quirk).parseBool,
			input:  "invalid",
			err:    `"invalid": must be true or false`,
		},

		// parseQuirkBuggyIppRsp
		{
			parser: (*Quirk).parseQuirkBuggyIppRsp,
			input:  "allow",
			value:  QuirkBuggyIppRspAllow,
		},

		{
			parser: (*Quirk).parseQuirkBuggyIppRsp,
			input:  "reject",
			value:  QuirkBuggyIppRspReject,
		},

		{
			parser: (*Quirk).parseQuirkBuggyIppRsp,
			input:  "sanitize",
			value:  QuirkBuggyIppRspSanitize,
		},

		{
			parser: (*Quirk).parseQuirkBuggyIppRsp,
			input:  "invalid",
			err:    `"invalid": must be allow, reject or sanitize`,
		},

		// parseDuration
		{
			parser: (*Quirk).parseDuration,
			input:  "0",
			value:  time.Duration(0),
		},

		{
			parser: (*Quirk).parseDuration,
			input:  "0s",
			value:  time.Duration(0),
		},

		{
			parser: (*Quirk).parseDuration,
			input:  "12345",
			value:  12345 * time.Millisecond,
		},

		{
			parser: (*Quirk).parseDuration,
			input:  "1h2m3s",
			value: time.Hour +
				2*time.Minute +
				3*time.Second,
		},

		{
			parser: (*Quirk).parseDuration,
			input:  "0.5s",
			value:  time.Second / 2,
		},

		{
			parser: (*Quirk).parseDuration,
			input:  "+0s",
			err:    `"+0s": invalid duration`,
		},

		{
			parser: (*Quirk).parseDuration,
			input:  "-0s",
			err:    `"-0s": invalid duration`,
		},

		{
			parser: (*Quirk).parseDuration,
			input:  "hello",
			err:    `"hello": invalid duration`,
		},

		// parseQuirkResetMethod
		{
			parser: (*Quirk).parseQuirkResetMethod,
			input:  "none",
			value:  QuirkResetNone,
		},

		{
			parser: (*Quirk).parseQuirkResetMethod,
			input:  "soft",
			value:  QuirkResetSoft,
		},

		{
			parser: (*Quirk).parseQuirkResetMethod,
			input:  "hard",
			value:  QuirkResetHard,
		},

		{
			parser: (*Quirk).parseQuirkResetMethod,
			input:  "invalid",
			err:    `"invalid": must be none, soft or hard`,
		},

		// parseUint
		{
			parser: (*Quirk).parseUint,
			input:  "0",
			value:  uint(0),
		},

		{
			parser: (*Quirk).parseUint,
			input:  "12345",
			value:  uint(12345),
		},

		{
			parser: (*Quirk).parseUint,
			input:  "hello",
			err:    `"hello": invalid unsigned integer`,
		},
	}

	for _, test := range tests {
		q := Quirk{
			RawValue: test.input,
		}

		err := test.parser(&q)
		errstr := ""
		if err != nil {
			errstr = err.Error()
		}

		if errstr != test.err {
			t.Errorf("error mismatch:\n"+
				"expected: %s\n"+
				"present:  %s",
				test.err, errstr)

			continue
		}

		if q.Parsed != test.value {
			t.Errorf("value mismatch:\n"+
				"expected: %s(%v)\n"+
				"present:  %s(%v)",
				reflect.TypeOf(test.value), test.value,
				reflect.TypeOf(q.Parsed), q.Parsed)
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
