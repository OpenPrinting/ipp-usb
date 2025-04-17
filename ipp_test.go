/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * IPP tests
 */

package main

import (
	"testing"

	"github.com/OpenPrinting/goipp"
)

// TestNewIppAttrs tests newIppAttrs function
func TestNewIppAttrs(t *testing.T) {
	type testData struct {
		in  goipp.Attributes // Input attributes
		out goipp.Attributes // Resulting attributes
	}

	tests := []testData{
		{
			// Normal data
			in: goipp.Attributes{
				goipp.MakeAttr("mopria-certified",
					goipp.TagText,
					goipp.String("1.3")),
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagText,
					goipp.String("test printer")),
			},

			out: goipp.Attributes{
				goipp.MakeAttr("mopria-certified",
					goipp.TagText,
					goipp.String("1.3")),
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagText,
					goipp.String("test printer")),
			},
		},

		{
			// Duplicated attribute. First occurrence wins
			in: goipp.Attributes{
				goipp.MakeAttr("mopria-certified",
					goipp.TagText,
					goipp.String("1.3")),
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagText,
					goipp.String("test printer")),
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagText,
					goipp.String("duplicate")),
			},

			out: goipp.Attributes{
				goipp.MakeAttr("mopria-certified",
					goipp.TagText,
					goipp.String("1.3")),
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagText,
					goipp.String("test printer")),
			},
		},
	}

	for _, test := range tests {
		attrs := newIppAttrs(test.in)
		out := attrs.export()

		if !out.Similar(test.out) {
			f := goipp.NewFormatter()
			f.Printf("newIppAttrs test failed:")

			f.Printf("input:")
			f.SetIndent(4)
			f.FmtAttributes(test.in)
			f.SetIndent(0)

			f.Printf("expected output:")
			f.SetIndent(4)
			f.FmtAttributes(test.out)
			f.SetIndent(0)

			f.Printf("present output:")
			f.SetIndent(4)
			f.FmtAttributes(out)
			f.SetIndent(0)

			t.Errorf("%s", f.String())

		}
	}
}
