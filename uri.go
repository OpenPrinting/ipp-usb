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

// MakeURI builds device URI string.
//
// The returned string is compatible for the CUPS USB backend
// for the same device.
func MakeURI(mfg, mdl, sn string) string {
	buf := strings.Builder{}

	buf.WriteString("usb://")
	uriEncode(&buf, mfg, "\"#/:<>?@[\\]^`{|}")
	buf.WriteByte('/')
	uriEncode(&buf, mdl, "")
	buf.WriteString("?serial=")
	uriEncode(&buf, sn, "")

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
