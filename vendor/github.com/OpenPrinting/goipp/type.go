/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Enumeration of value types
 */

package goipp

import (
	"fmt"
)

// Type enumerates all possible value types
type Type int

// Type values
const (
	TypeInvalid      Type = -1   // Invalid Value type
	TypeVoid         Type = iota // Value is Void
	TypeInteger                  // Value is Integer
	TypeBoolean                  // Value is Boolean
	TypeString                   // Value is String
	TypeDateTime                 // Value is Time
	TypeResolution               // Value is Resolution
	TypeRange                    // Value is Range
	TypeTextWithLang             // Value is TextWithLang
	TypeBinary                   // Value is Binary
	TypeCollection               // Value is Collection
)

// String converts Type to string, for debugging
func (t Type) String() string {
	if t == TypeInvalid {
		return "Invalid"
	}

	if 0 <= t && int(t) < len(typeNames) {
		if s := typeNames[t]; s != "" {
			return s
		}
	}

	return fmt.Sprintf("0x%4.4x", uint(t))
}

var typeNames = [...]string{
	TypeVoid:         "Void",
	TypeInteger:      "Integer",
	TypeBoolean:      "Boolean",
	TypeString:       "String",
	TypeDateTime:     "DateTime",
	TypeResolution:   "Resolution",
	TypeRange:        "Range",
	TypeTextWithLang: "TextWithLang",
	TypeBinary:       "Binary",
	TypeCollection:   "Collection",
}
