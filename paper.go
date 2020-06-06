/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Paper Size Classifier
 */

package main

// PaperSize represents paper size, in IPP units (1/100 mm)
type PaperSize struct {
	Width, Height int // Paper width and height
}

// Standard paper sizes
//                  US name      US inches   US mm           ISO mm
//   "legal-A4"     A, Legal     8.5 x 14    215.9 x 355.6   A4: 210 x 297
//   "tabloid-A3"   B, Tabloid   11 x 17     279.4 x 431.8   A3: 297 x 420
//   "isoC-A2"      C            17 × 22     431.8 × 558.8   A2: 420 x 594
//
// Please note, Apple in the "Bonjour Printing Specification"
// incorrectly states paper sizes as 9x14, 13x19 and 18x24 inches
var (
	PaperLegal   = PaperSize{21590, 35560}
	PaperA4      = PaperSize{21000, 29700}
	PaperTabloid = PaperSize{27940, 43180}
	PaperA3      = PaperSize{29700, 42000}
	PaperC       = PaperSize{43180, 55880}
	PaperA2      = PaperSize{42000, 59400}
)

// Less checks that p is less that p2, which means:
//   * Either p.Width or p.Height is less that p2.Width or p2.Heigh
//   * Neither of p.Width or p.Height is greater that p2.Width or p2.Heigh
func (p PaperSize) Less(p2 PaperSize) bool {
	return (p.Width < p2.Width && p.Height <= p2.Height) ||
		(p.Height < p2.Height && p.Width <= p2.Width)
}

// Classify paper size according to Apple Bonjour rules
// Returns:
//     ">isoC-A2" for paper larger that C or A2
//     "isoC-A2" for C or A2 paper
//     "tabloid-A3" for Tabloid or A3 paper
//     "legal-A4" for Legal or A4 paper
//     "<legal-A4" for paper smaller that Legal or A4
func (p PaperSize) Classify() string {
	switch {
	case PaperC.Less(p) || PaperA2.Less(p):
		return ">isoC-A2"

	case !p.Less(PaperC) || !p.Less(PaperA2):
		return "isoC-A2"

	case !p.Less(PaperTabloid) || !p.Less(PaperA3):
		return "tabloid-A3"

	case !p.Less(PaperLegal) || !p.Less(PaperA4):
		return "legal-A4"

	default:
		return "<legal-A4"
	}
}
