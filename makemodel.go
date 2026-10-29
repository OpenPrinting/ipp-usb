/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Make and model handling
 */

package main

import "strings"

// CUPSManufacturer transforms the device manufacturer name to be consistent
// with the CUPS USB backend normalization (e.g., "Hewlett-Packard" to "HP").
func CUPSManufacturer(mfg string) string {
	switch {
	case strings.EqualFold(mfg, "Hewlett-Packard"):
		mfg = "HP"
	case strings.EqualFold(mfg, "Lexmark International"):
		mfg = "Lexmark"
	}

	return mfg
}

// MakeAndModel combines device manufacturer name and
// model into the single string.
func MakeAndModel(mfg, mdl string) string {
	makeModel := mdl
	if mfg != "" && !strings.HasPrefix(mdl, mfg) {
		makeModel = mfg + " " + mdl
	}

	return makeModel
}
