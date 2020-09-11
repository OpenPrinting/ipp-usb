/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * UUID normalizer
 */

package main

import (
	"bytes"
)

// UUIDNormalize parses an UUID and then reformats it into
// the standard form (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx)
//
// If input is not a valid UUID, it returns an empty string
// Many standard formats of UUIDs are recognized
func UUIDNormalize(uuid string) string {
	var buf [32]byte
	var cnt int

	in := bytes.ToLower([]byte(uuid))

	if bytes.HasPrefix(in, []byte("urn:")) {
		in = in[4:]
	}

	if bytes.HasPrefix(in, []byte("uuid:")) {
		in = in[5:]
	}

	for len(in) != 0 {
		c := in[0]
		in = in[1:]

		if '0' <= c && c <= '9' || 'a' <= c && c <= 'f' {
			if cnt == 32 {
				return ""
			}

			buf[cnt] = c
			cnt++
		}
	}

	if cnt != 32 {
		return ""
	}

	return string(buf[0:8]) + "-" +
		string(buf[8:12]) + "-" +
		string(buf[12:16]) + "-" +
		string(buf[16:20]) + "-" +
		string(buf[20:32])
}
