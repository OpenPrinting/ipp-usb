/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * USB devices matching by HWID
 */

package main

import "strconv"

// HWIDPattern defines matching rule for matching USB devices by
// the hardware ID
type HWIDPattern struct {
	vid, pid uint16 // Vendor/Product IDs
	anypid   bool   // Pattern matches any PID
}

// ParseHWIDPattern parses supplied string as the HWID-style
// pattern.
//
// HWID pattern may take one of the following forms:
//
//	VVVV:DDDD - matches devices by vendor and device IDs
//	VVVV:*    - matches devices by vendor ID with any device ID
//
// VVVV and DDDD are device/vendor IDs, represented as sequence of
// the four hexadecimal digits.
//
// It returns *HWIDPattern or nil, if string doesn't match HWIDPattern
// syntax.
func ParseHWIDPattern(pattern string) *HWIDPattern {
	// Split pattern into VID and PID
	if len(pattern) != 6 && len(pattern) != 9 {
		return nil
	}

	if pattern[4] != ':' {
		return nil
	}

	strVID := pattern[:4]
	strPID := pattern[5:]

	// Parse parts
	var vid, pid uint64
	var anypid bool
	var err error

	vid, err = strconv.ParseUint(strVID, 16, 16)
	if err != nil {
		return nil
	}

	if strPID == "*" {
		anypid = true
	} else {
		pid, err = strconv.ParseUint(strPID, 16, 16)
		if err != nil {
			return nil
		}
	}

	return &HWIDPattern{vid: uint16(vid), pid: uint16(pid), anypid: anypid}
}

// Match reports if the USB device VID/PID matches the pattern.
//
// It returns the "matching weight" which allows to prioritize
// quirks, if there are multiple matches, as more or less specific
// (the more the weight, the more specific the quirk is).
//
// The matching weight is the math.MaxInt32 for the exact match (VID+PID)
// and 1 for the wildcard match (VID only). It makes the exact match to
// be considered as very specific, while wildcard match to be considered
// only slightly more specific, that the all-wildcard (i.e., the default)
// match by the model name.
//
// If there is no match, it returns -1.
//
// See also [GlobMatch] documentation for comparison with the
// similar function, used for match-by-model-name purpose.
func (p *HWIDPattern) Match(vid, pid uint16) int {
	ok := vid == p.vid && (p.anypid || pid == p.pid)

	switch {
	case !ok:
		return -1 // No match
	case p.anypid:
		return 1 // Match by VID only
	default:
		return 1000 // Match by VID+PID
	}
}
