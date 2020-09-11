/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Tests for device-specific quirks
 */

package main

import (
	"testing"
)

// Test (QuirksSet) matchModelName()
func TestQuirksSetMatchModelName(t *testing.T) {
	testData := []struct {
		model, pattern string
		count          int
	}{
		{"test", "test", 4},
		{"test", "tes?", 3},
		{"test", "te?t", 3},
		{"test", "te??", 2},
		{"test", "te??x", -1},
		{"test", "te*", 2},
		{"test", "te**", 2},
		{"test", "*te**", 2},
		{"", "*", 0},
		{"test", "t\\est", 4},
		{"t?st", "t\\?st", 4},
	}

	var qset QuirksSet
	for _, data := range testData {
		n := qset.matchModelName(data.model, data.pattern, 0)
		if n != data.count {
			t.Errorf("matchModelName(%q,%q): expected %d got %d",
				data.model, data.pattern, data.count, n)
		}
	}
}

// Test quirls loading and lookup
func TestQuirksSetLoadAndLookup(t *testing.T) {
	const path = "testdata/quirks"
	const bad_path = path + "-not-exist"

	// Try non-existent directory
	_, err := LoadQuirksSet(bad_path)
	if err != nil {
		t.Fatalf("LoadQuirksSet(%q): %s", bad_path, err)
	}

	// Try test data
	qset, err := LoadQuirksSet(path)
	if err != nil {
		t.Fatalf("LoadQuirksSet(%q): %s", path, err)
	}

	// Test default quirks
	quirks := qset.Get("unknown device")
	if quirks == nil {
		t.Fatalf("default quirls: missed")
	}

	if len(quirks) != 1 {
		t.Fatalf("default quirls: expected 1, got %d", len(quirks))
	}

	// Test quirks for some known device
	device := "HP LaserJet MFP M28-M31"
	quirks = qset.Get(device)
	if quirks == nil {
		t.Fatalf("%q quirls: missed", device)
	}

	if len(quirks) != 1 { // default overridden by this device
		t.Fatalf("%q quirls: expected 1, got %d", device, len(quirks))
	}

	if quirks[0].Model != device {
		t.Fatalf("%q quirls: wrong ordering of returned quirks", device)
	}
}
