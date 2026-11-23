/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * URI handling test
 */

package main

import (
	"testing"
)

// TestURIString tests URI.String
func TestURIString(t *testing.T) {
	type testData struct {
		uri URI
		out string
	}

	tests := []testData{
		{
			uri: URI{MFG: "Xerox", MDL: "B235 MFP", SN: "34004H030206H"},
			out: "usb://Xerox/B235%20MFP?serial=34004H030206H",
		},

		// Characters, reserved in the hostname part, must be
		// escaped in MFG but not in MDL and SN
		{
			uri: URI{MFG: "A:B/C<D>E|F\"G", MDL: "A:B<C>", SN: "1:2|3"},
			out: "usb://A%3AB%2FC%3CD%3EE%7CF%22G/A:B<C>?serial=1:2|3",
		},
		{
			uri: URI{MFG: "#?@[\\]^`{}", MDL: "M", SN: "S"},
			out: "usb://%23%3F%40%5B%5C%5D%5E%60%7B%7D/M?serial=S",
		},

		// '/', '?', '#', '[', ']' and '@' are always escaped,
		// even where they are not in the reserved set
		{
			uri: URI{MFG: "M", MDL: "A/B?C#D", SN: "[1]@2"},
			out: "usb://M/A%2FB%3FC%23D?serial=%5B1%5D%402",
		},

		// '%', control characters and non-ASCII bytes are
		// always escaped
		{
			uri: URI{MFG: "100%", MDL: "a\x01b\tc\x7f", SN: "\xd0\x96"},
			out: "usb://100%25/a%01b%09c\x7f?serial=%D0%96",
		},

		// Empty strings
		{
			uri: URI{},
			out: "usb:///?serial=",
		},
	}

	for _, test := range tests {
		out := test.uri.String()
		if out != test.out {
			t.Errorf("%#v:\nexpected: %q\npresent:  %q",
				test.uri, test.out, out)
		}
	}
}
