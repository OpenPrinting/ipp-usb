/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * URI handling
 */

package main

import (
	"fmt"
	"strings"
)

// URI contains information, encoded into the CUPS-compatible
// USB device URI
type URI struct {
	MFG string // Device Manufacturer
	MDL string // Device Model
	SN  string // Device serial number
}

// String converts information, represented by URIData,
// into the URI string:
//
//	usb://Xerox/B235%20MFP?serial=34004H030206H
func (u URI) String() string {
	buf := strings.Builder{}

	buf.WriteString("usb://")
	uriEncode(&buf, u.MFG, "\"#/:<>?@[\\]^`{|}")
	buf.WriteByte('/')
	uriEncode(&buf, u.MDL, "")
	buf.WriteString("?serial=")
	uriEncode(&buf, u.SN, "")

	return buf.String()
}

// uriEncode escapes part of the URI and appends result
// to the provided strings.Builder.
func uriEncode(buf *strings.Builder, src string, reserved string) {
	for _, c := range []byte(src) {
		escape := false

		switch {
		case c == '%' || c <= ' ' || c >= 128:
			escape = true
		case strings.IndexByte(reserved, c) >= 0:
			escape = true
		}

		switch c {
		case '%', ' ', '/', '?', '#', '[', ']', '@':
			escape = true
		default:
			escape = c >= 128
		}

		if escape {
			buf.WriteString(fmt.Sprintf("%%%2.2X", c))
		} else {
			buf.WriteByte(c)
		}
	}
}
