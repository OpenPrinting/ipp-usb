/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Tests for .INI reader
 */

package main

import (
	"io"
	"testing"
)

// Don't forget to update testData when ipp-ini.conf changes
var testData = []struct{ section, key, value string }{
	{"network", "http-min-port", "60000"},
	{"network", "http-max-port", "65535"},
	{"network", "dns-sd", "enable"},
	{"network", "interface", "loopback"},
	{"network", "ipv6", "enable"},
	{"logging", "device-log", "all"},
	{"logging", "main-log", "debug"},
	{"logging", "console-log", "debug"},
	{"logging", "max-file-size", "256K"},
	{"logging", "max-backup-files", "5"},
	{"logging", "console-color", "enable"},
}

// Test .INI reader
func TestIniReader(t *testing.T) {
	// Open ipp-usb.conf
	ini, err := OpenIniFile("testdata/ipp-usb.conf")
	if err != nil {
		t.Fatalf("%s", err)
	}

	defer ini.Close()

	// Read record by record
	var rec *IniRecord
	current := 0
	for err == nil {
		rec, err = ini.Next()
		if err != nil {
			break
		}

		if current >= len(testData) {
			t.Errorf("unexpected record: [%s] %s = %s", rec.Section, rec.Key, rec.Value)
		} else if rec.Section != testData[current].section ||
			rec.Key != testData[current].key ||
			rec.Value != testData[current].value {
			t.Errorf("data mismatch:")
			t.Errorf("  expected: [%s] %s = %s", testData[current].section, testData[current].key, testData[current].value)
			t.Errorf("  present:  [%s] %s = %s", rec.Section, rec.Key, rec.Value)
		} else {
			current++
		}
	}

	if err != io.EOF {
		t.Fatalf("%s", err)
	}
}
