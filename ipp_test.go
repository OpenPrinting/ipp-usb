/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * IPP tests
 */

package main

import (
	"reflect"
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

// TestIppAttrsGetStrings tests ippAttrs.getStrings function
func TestIppAttrsGetStrings(t *testing.T) {
	type testData struct {
		attrs goipp.Attributes // Attributes in the set
		name  string           // Requested name
		out   []string         // Expected ippAttrs.getStrings output
	}

	tests := []testData{
		{
			// Empty set returns nil
			attrs: nil,
			name:  "printer-make-and-model",
			out:   nil,
		},

		{
			// Invalid attribute type, nil is expected
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagInteger,
					goipp.Integer(5)),
			},
			name: "printer-make-and-model",
			out:  nil,
		},

		{
			// Single value of type goipp.String
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagText,
					goipp.String("test printer")),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer"},
		},

		{
			// Single value of type goipp.TextWithLang
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagTextLang,
					goipp.TextWithLang{
						Lang: "en",
						Text: "test printer",
					},
				),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer"},
		},

		{
			// Mix of goipp.String/goipp.TextWithLang
			// goipp.String wins
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagTextLang,
					goipp.TextWithLang{
						Lang: "en",
						Text: "test printer - en",
					},
					goipp.String("test printer"),
				),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer"},
		},

		{
			// Mix of goipp.String/goipp.TextWithLang, reordered
			// goipp.String wins
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagTextLang,
					goipp.String("test printer"),
					goipp.TextWithLang{
						Lang: "en",
						Text: "test printer - en",
					},
				),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer"},
		},

		{
			// "en" vs "ru". "en" wins
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagTextLang,
					goipp.TextWithLang{
						Lang: "en",
						Text: "test printer",
					},
					goipp.TextWithLang{
						Lang: "ru",
						Text: "тестовый принтер",
					},
				),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer"},
		},

		{
			// "en" vs "ru", in the different order. "en" wins
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagTextLang,
					goipp.TextWithLang{
						Lang: "ru",
						Text: "тестовый принтер",
					},
					goipp.TextWithLang{
						Lang: "en",
						Text: "test printer",
					},
				),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer"},
		},

		{
			// "EN" vs "RU" (in uppercase). "EN" wins
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagTextLang,
					goipp.TextWithLang{
						Lang: "EN",
						Text: "test printer",
					},
					goipp.TextWithLang{
						Lang: "RU",
						Text: "тестовый принтер",
					},
				),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer"},
		},

		{
			// "EN" vs "RU", in the different order. "EN" wins
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagTextLang,
					goipp.TextWithLang{
						Lang: "RU",
						Text: "тестовый принтер",
					},
					goipp.TextWithLang{
						Lang: "EN",
						Text: "test printer",
					},
				),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer"},
		},

		{
			// "en" vs "en-US". "en" wins
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagTextLang,
					goipp.TextWithLang{
						Lang: "en",
						Text: "test printer - en",
					},
					goipp.TextWithLang{
						Lang: "en-US",
						Text: "test printer - en-US",
					},
				),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer - en"},
		},

		{
			// "en-US" vs "en-XXX". "en-US" wins
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagTextLang,
					goipp.TextWithLang{
						Lang: "en-US",
						Text: "test printer - US",
					},
					goipp.TextWithLang{
						Lang: "en-XXX",
						Text: "test printer - XXX",
					},
				),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer - US"},
		},

		{
			// "en-XXX" vs "ru". "en-XXX" wins
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagTextLang,
					goipp.TextWithLang{
						Lang: "ru",
						Text: "тестовый принтер",
					},
					goipp.TextWithLang{
						Lang: "en-XXX",
						Text: "test printer",
					},
				),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer"},
		},

		{
			// "de" vs "it". First occurrence wins
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagTextLang,
					goipp.TextWithLang{
						Lang: "de",
						Text: "test printer - de",
					},
					goipp.TextWithLang{
						Lang: "it",
						Text: "test printer - it",
					},
				),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer - de"},
		},

		{
			// "de" vs "it" in reverse order.
			// First occurrence wins
			attrs: goipp.Attributes{
				goipp.MakeAttr("printer-make-and-model",
					goipp.TagTextLang,
					goipp.TextWithLang{
						Lang: "it",
						Text: "test printer - it",
					},
					goipp.TextWithLang{
						Lang: "de",
						Text: "test printer - de",
					},
				),
			},
			name: "printer-make-and-model",
			out:  []string{"test printer - it"},
		},

		{
			// Multiple values
			attrs: goipp.Attributes{
				goipp.MakeAttr("kind",
					goipp.TagKeyword,
					goipp.String("document"),
					goipp.String("envelope"),
				),
			},
			name: "kind",
			out:  []string{"document", "envelope"},
		},
	}

	for _, test := range tests {
		attrs := newIppAttrs(test.attrs)
		out := attrs.getStrings(test.name)

		if !reflect.DeepEqual(test.out, out) {
			f := goipp.NewFormatter()
			f.Printf("ippAttrs.getStrings test failed:")

			f.Printf("input:")
			f.SetIndent(4)
			f.FmtAttributes(test.attrs)
			f.SetIndent(0)

			f.Printf("query:    %s", test.name)
			f.Printf("expected: %v", test.out)
			f.Printf("presemt:  %v", out)

			t.Errorf("%s", f.String())
		}
	}
}
