/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Tests for paper.go
 */

package main

import (
	"testing"
)

var allSizes = []PaperSize{
	PaperLegal,
	PaperA4,
	PaperTabloid,
	PaperA3,
	PaperC,
	PaperA2,
}

// Compute p.Less(p2) and check answer
func testPaperSizeLess(t *testing.T, p, p2 PaperSize, answer bool) {
	rsp := p.Less(p2)
	if rsp != answer {
		t.Errorf("PaperSize{%d,%d}.Less(PaperSize{%d,%d}): %v, must be %v",
			p.Width, p.Height,
			p2.Width, p2.Height,
			rsp, answer,
		)
	}
}

// Compute p.Classify() and check answer
func testPaperSizeClassify(t *testing.T, p PaperSize, answer string) {
	rsp := p.Classify()
	if rsp != answer {
		t.Errorf("PaperSize{%d,%d}.Classify(): %v, must be %v",
			p.Width, p.Height,
			rsp, answer,
		)
	}
}

// Test (PaperSize) Less()
func TestPaperSizeLess(t *testing.T) {
	var p2 PaperSize

	for _, p := range allSizes {
		testPaperSizeLess(t, p, p, false)

		if p.Less(p) {
			t.Fail()
		}

		p2 = PaperSize{p.Width - 1, p.Height}
		testPaperSizeLess(t, p, p2, false)
		testPaperSizeLess(t, p2, p, true)

		p2 = PaperSize{p.Width, p.Height - 1}
		testPaperSizeLess(t, p, p2, false)
		testPaperSizeLess(t, p2, p, true)

		p2 = PaperSize{p.Width - 1, p.Height + 1}
		testPaperSizeLess(t, p, p2, false)
		testPaperSizeLess(t, p2, p, false)

		p2 = PaperSize{p.Width + 1, p.Height - 1}
		testPaperSizeLess(t, p, p2, false)
		testPaperSizeLess(t, p2, p, false)
	}
}

// Test (PaperSize) Classify()
func TestPaperSizeClassify(t *testing.T) {
	testPaperSizeClassify(t, PaperLegal, "legal-A4")
	testPaperSizeClassify(t, PaperA4, "legal-A4")

	testPaperSizeClassify(t, PaperTabloid, "tabloid-A3")
	testPaperSizeClassify(t, PaperA3, "tabloid-A3")

	testPaperSizeClassify(t, PaperC, "isoC-A2")
	testPaperSizeClassify(t, PaperA2, "isoC-A2")

	var sizes []PaperSize
	sizes = []PaperSize{
		{PaperA4.Width - 1, PaperA4.Height},
		{PaperA4.Width, PaperA4.Height - 1},
	}

	for _, p := range sizes {
		testPaperSizeClassify(t, p, "<legal-A4")
	}

	sizes = []PaperSize{
		{PaperC.Width + 1, PaperC.Height},
		{PaperC.Width, PaperC.Height + 1},
		{PaperA2.Width + 1, PaperA2.Height},
		{PaperA2.Width, PaperA2.Height + 1},
	}

	for _, p := range sizes {
		testPaperSizeClassify(t, p, ">isoC-A2")
	}

	// HP LaserJet MFP M28
	testPaperSizeClassify(t, PaperSize{21590, 29692}, "legal-A4")
}
