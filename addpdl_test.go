/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * (*DNSSdTxtRecord) AddPDL() test
 */

package main

import (
	"testing"
)

var testDataAddPDL = []struct{ in, out string }{
	{
		"application/pdf",
		"application/pdf",
	},

	{
		"application/octet-stream," +
			"application/pdf,image/tiff,image/jpeg,image/urf," +
			"application/postscript,application/vnd.hp-PCL," +
			"application/vnd.hp-PCLXL,application/vnd.xpsdocument," +
			"image/pwg-raster",

		"application/octet-stream," +
			"application/pdf,image/tiff,image/jpeg,image/urf," +
			"application/postscript,application/vnd.hp-PCL," +
			"application/vnd.hp-PCLXL,application/vnd.xpsdocument," +
			"image/pwg-raster",
	},

	{
		"application/vnd.hp-PCL,application/vnd.hp-PCLXL," +
			"application/postscript,application/msword," +
			"application/pdf,image/jpeg,image/urf," +
			"image/pwg-raster," +
			"application/PCLm," +
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document," +
			"application/vnd.ms-excel," +
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet," +
			"application/vnd.ms-powerpoint," +
			"application/vnd.openxmlformats-officedocument.presentationml.presentation," +
			"application/octet-stream",

		"application/vnd.hp-PCL,application/vnd.hp-PCLXL," +
			"application/postscript,application/msword," +
			"application/pdf,image/jpeg,image/urf," +
			"image/pwg-raster,application/PCLm," +
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	},
}

// Test .INI reader
func TestAddPDL(t *testing.T) {
	for i, data := range testDataAddPDL {
		var txt DNSSdTxtRecord
		txt.AddPDL("pdl", data.in)

		if len(txt) != 1 {
			t.Errorf("test %d: unexpected (%d) number of TXT elements added",
				i+1, len(txt))
			return
		}

		if txt[0].Value != data.out {
			t.Errorf("test %d: extected %q, got %q",
				i+1, data.out, txt[0].Value)
		}
	}
}
