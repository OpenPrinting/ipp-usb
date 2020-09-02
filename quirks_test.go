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

	qset := make(QuirksSet)
	for _, data := range testData {
		n := qset.matchModelName(data.model, data.pattern, 0)
		if n != data.count {
			t.Errorf("matchModelName(%q,%q): expected %d got %d",
				data.model, data.pattern, data.count, n)
		}
	}

}
