/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Tests for glob-style pattern matching
 */

package main

import (
	"testing"
)

// Test GlobMatch
func TestGlobMatch(t *testing.T) {
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

	for _, data := range testData {
		n := GlobMatch(data.model, data.pattern)
		if n != data.count {
			t.Errorf("matchModelName(%q,%q): expected %d got %d",
				data.model, data.pattern, data.count, n)
		}
	}
}
