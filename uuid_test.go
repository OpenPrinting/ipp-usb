/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * UUID normalizer test
 */

package main

import (
	"testing"
)

// Don't forget to update testData when ipp-ini.conf changes
var testDataUUID = []struct{ in, out string }{
	{"01234567-89ab-cdef-0123-456789abcdef", "01234567-89ab-cdef-0123-456789abcdef"},
	{"01234567-89ab-cdef-0123-456789abcde", ""},
	{"01234567-89ab-cdef-0123-456789abcdef0", ""},
	{"urn:01234567-89ab-cdef-0123-456789abcdef", "01234567-89ab-cdef-0123-456789abcdef"},
	{"urn:uuid:01234567-89ab-cdef-0123-456789abcdef", "01234567-89ab-cdef-0123-456789abcdef"},
	{"0123456789abcdef0123456789abcdef", "01234567-89ab-cdef-0123-456789abcdef"},
	{"{0123456789abcdef0123456789abcdef}", "01234567-89ab-cdef-0123-456789abcdef"},
}

// Test .INI reader
func TestUUIDNormalize(t *testing.T) {
	for _, data := range testDataUUID {
		uuid := UUIDNormalize(data.in)
		if uuid != data.out {
			t.Errorf("UUIDNormalize(%q): expected %q, got %q", data.in, data.out, uuid)
		}
	}
}
