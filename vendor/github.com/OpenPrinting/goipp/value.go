/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Values for message attributes
 */

package goipp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
)

// Values represents a sequence of values with tags.
// Usually Values used as a "payload" of Attribute
type Values []struct {
	T Tag   // The tag
	V Value // The value
}

// Add Value to Values
func (values *Values) Add(t Tag, v Value) {
	*values = append(*values, struct {
		T Tag
		V Value
	}{t, v})
}

// String converts Values to string
func (values Values) String() string {
	if len(values) == 1 {
		return values[0].V.String()
	}

	var buf bytes.Buffer
	buf.Write([]byte("["))
	for i, v := range values {
		if i != 0 {
			buf.Write([]byte(","))
		}
		buf.Write([]byte(v.V.String()))
	}
	buf.Write([]byte("]"))

	return buf.String()
}

// Clone creates a shallow copy of Values.
// For nil input it returns nil output.
func (values Values) Clone() Values {
	var values2 Values
	if values != nil {
		values2 = make(Values, len(values))
		copy(values2, values)
	}
	return values2
}

// DeepCopy creates a deep copy of Values
// For nil input it returns nil output.
func (values Values) DeepCopy() Values {
	var values2 Values
	if values != nil {
		values2 = make(Values, len(values))
		for i := range values {
			values2[i].T = values[i].T
			values2[i].V = values[i].V.DeepCopy()
		}
	}
	return values2
}

// Equal performs deep check of equality of two Values.
//
// Note, Values(nil) and Values{} are not Equal but Similar.
func (values Values) Equal(values2 Values) bool {
	if len(values) != len(values2) {
		return false
	}

	if (values == nil) != (values2 == nil) {
		return false
	}

	for i, v := range values {
		v2 := values2[i]
		if v.T != v2.T || !ValueEqual(v.V, v2.V) {
			return false
		}
	}

	return true
}

// Similar performs deep check of **logical** equality of two Values
//
// Note, Values(nil) and Values{} are not Equal but Similar.
func (values Values) Similar(values2 Values) bool {
	if len(values) != len(values2) {
		return false
	}

	for i, v := range values {
		v2 := values2[i]
		if v.T != v2.T || !ValueSimilar(v.V, v2.V) {
			return false
		}
	}

	return true
}

// Value represents an attribute value
//
// IPP uses typed values, and type of each value is unambiguously
// defined by the attribute tag
type Value interface {
	String() string
	Type() Type
	DeepCopy() Value
	encode() ([]byte, error)
	decode([]byte) (Value, error)
}

var (
	_ = Value(Binary(nil))
	_ = Value(Boolean(false))
	_ = Value(Collection(nil))
	_ = Value(Integer(0))
	_ = Value(Range{})
	_ = Value(Resolution{})
	_ = Value(String(""))
	_ = Value(TextWithLang{})
	_ = Value(Time{time.Time{}})
	_ = Value(Void{})
)

// IntegerOrRange is a Value of type Integer or Range
type IntegerOrRange interface {
	Value

	// Within checks that x fits within the range:
	//
	//   for Integer: x == Integer's value
	//   for Range:   Lower <= x && x <= Upper
	Within(x int) bool
}

var (
	_ = IntegerOrRange(Integer(0))
	_ = IntegerOrRange(Range{})
)

// ValueEqual checks if two values are equal
//
// Equality means that types and values are equal. For structured
// values, like Collection, deep comparison is performed
func ValueEqual(v1, v2 Value) bool {
	if v1.Type() != v2.Type() {
		return false
	}

	switch v1.Type() {
	case TypeDateTime:
		return v1.(Time).Equal(v2.(Time).Time)
	case TypeBinary:
		return bytes.Equal(v1.(Binary), v2.(Binary))
	case TypeCollection:
		c1 := Attributes(v1.(Collection))
		c2 := Attributes(v2.(Collection))
		return c1.Equal(c2)
	}

	return v1 == v2
}

// ValueSimilar checks if two values are **logically** equal,
// which means the following:
//   - If values are equal (i.e., ValueEqual() returns true),
//     they are similar.
//   - Binary and String values are similar, if they represent
//     the same sequence of bytes.
//   - Two collections are similar, if they contain the same
//     set of attributes (but may be differently ordered) and
//     values of these attributes are similar.
func ValueSimilar(v1, v2 Value) bool {
	if ValueEqual(v1, v2) {
		return true
	}

	t1 := v1.Type()
	t2 := v2.Type()

	switch {
	case t1 == TypeBinary && t2 == TypeString:
		return bytes.Equal(v1.(Binary), []byte(v2.(String)))

	case t1 == TypeString && t2 == TypeBinary:
		return bytes.Equal([]byte(v1.(String)), v2.(Binary))

	case t1 == TypeCollection && t2 == TypeCollection:
		return Attributes(v1.(Collection)).Similar(
			Attributes(v2.(Collection)))
	}

	return false
}

// Void is the Value that represents "no value"
//
// Use with: TagUnsupportedValue, TagDefault, TagUnknown,
// TagNotSettable, TagDeleteAttr, TagAdminDefine
type Void struct{}

// String converts Void Value to string
func (Void) String() string { return "" }

// Type returns type of Value (TypeVoid for Void)
func (Void) Type() Type { return TypeVoid }

// DeepCopy returns a deep copy of the Void Value
func (v Void) DeepCopy() Value {
	return v
}

// Encode Void Value into wire format
func (v Void) encode() ([]byte, error) {
	return []byte{}, nil
}

// Decode Void Value from wire format
func (Void) decode([]byte) (Value, error) {
	return Void{}, nil
}

// Integer is the Value that represents 32-bit signed int
//
// Use with: TagInteger, TagEnum
type Integer int32

// String converts Integer value to string
func (v Integer) String() string { return fmt.Sprintf("%d", int32(v)) }

// Type returns type of Value (TypeInteger for Integer)
func (Integer) Type() Type { return TypeInteger }

// DeepCopy returns a deep copy of the Integer Value
func (v Integer) DeepCopy() Value {
	return v
}

// Within checks that x fits within the range
//
// It implements IntegerOrRange interface
func (v Integer) Within(x int) bool {
	return x == int(v)
}

// Encode Integer Value into wire format
func (v Integer) encode() ([]byte, error) {
	return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}, nil
}

// Decode Integer Value from wire format
func (Integer) decode(data []byte) (Value, error) {
	if len(data) != 4 {
		return nil, errors.New("value must be 4 bytes")
	}

	return Integer(int32(binary.BigEndian.Uint32(data))), nil
}

// Boolean is the Value that contains true of false
//
// Use with: TagBoolean
type Boolean bool

// String converts Boolean value to string
func (v Boolean) String() string { return fmt.Sprintf("%t", bool(v)) }

// Type returns type of Value (TypeBoolean for Boolean)
func (Boolean) Type() Type { return TypeBoolean }

// DeepCopy returns a deep copy of the Boolean Value
func (v Boolean) DeepCopy() Value {
	return v
}

// Encode Boolean Value into wire format
func (v Boolean) encode() ([]byte, error) {
	if v {
		return []byte{1}, nil
	}
	return []byte{0}, nil
}

// Decode Boolean Value from wire format
func (Boolean) decode(data []byte) (Value, error) {
	if len(data) != 1 {
		return nil, errors.New("value must be 1 byte")
	}

	return Boolean(data[0] != 0), nil
}

// String is the Value that represents string of text
//
// Use with: TagText, TagName, TagReservedString, TagKeyword, TagURI,
// TagURIScheme, TagCharset, TagLanguage, TagMimeType, TagMemberName
type String string

// String converts String value to string
func (v String) String() string { return string(v) }

// Type returns type of Value (TypeString for String)
func (String) Type() Type { return TypeString }

// DeepCopy returns a deep copy of the String Value
func (v String) DeepCopy() Value {
	return v
}

// Encode String Value into wire format
func (v String) encode() ([]byte, error) {
	return []byte(v), nil
}

// Decode String Value from wire format
func (String) decode(data []byte) (Value, error) {
	return String(data), nil
}

// Time is the Value that represents DataTime
//
// Use with: TagTime
type Time struct{ time.Time }

// String converts Time value to string
func (v Time) String() string { return v.Time.Format(time.RFC3339) }

// Type returns type of Value (TypeDateTime for Time)
func (Time) Type() Type { return TypeDateTime }

// DeepCopy returns a deep copy of the Time Value
func (v Time) DeepCopy() Value {
	return v
}

// Encode Time Value into wire format
func (v Time) encode() ([]byte, error) {
	// From RFC2579:
	//
	//     field  octets  contents                  range
	//     -----  ------  --------                  -----
	//       1      1-2   year*                     0..65536
	//       2       3    month                     1..12
	//       3       4    day                       1..31
	//       4       5    hour                      0..23
	//       5       6    minutes                   0..59
	//       6       7    seconds                   0..60
	//                    (use 60 for leap-second)
	//       7       8    deci-seconds              0..9
	//       8       9    direction from UTC        '+' / '-'
	//       9      10    hours from UTC*           0..13
	//      10      11    minutes from UTC          0..59
	//
	//     * Notes:
	//     - the value of year is in network-byte order
	//     - daylight saving time in New Zealand is +13

	year := v.Year()
	_, zone := v.Zone()
	dir := byte('+')
	if zone < 0 {
		zone = -zone
		dir = '-'
	}

	return []byte{
		byte(year >> 8), byte(year),
		byte(v.Month()),
		byte(v.Day()),
		byte(v.Hour()),
		byte(v.Minute()),
		byte(v.Second()),
		byte(v.Nanosecond() / 100000000),
		dir,
		byte(zone / 3600),
		byte((zone / 60) % 60),
	}, nil
}

// Decode Time Value from wire format
func (Time) decode(data []byte) (Value, error) {
	// Check size
	if len(data) != 11 {
		return nil, errors.New("value must be 11 bytes")
	}

	// Validate ranges
	var err error
	switch {
	case data[2] < 1 || data[2] > 12:
		err = fmt.Errorf("bad month %d", data[2])
	case data[3] < 1 || data[3] > 31:
		err = fmt.Errorf("bad day %d", data[3])
	case data[4] > 23:
		err = fmt.Errorf("bad hours %d", data[4])
	case data[5] > 59:
		err = fmt.Errorf("bad minutes %d", data[5])
	case data[6] > 60:
		err = fmt.Errorf("bad seconds %d", data[6])
	case data[7] > 9:
		err = fmt.Errorf("bad deciseconds %d", data[7])
	case data[8] != '+' && data[8] != '-':
		return nil, errors.New("bad UTC sign")
	case data[9] > 11:
		err = fmt.Errorf("bad UTC hours %d", data[9])
	case data[10] > 59:
		err = fmt.Errorf("bad UTC minutes %d", data[10])
	}

	if err != nil {
		return Time{}, err
	}

	// Decode time zone
	tzName := fmt.Sprintf("UTC%c%d", data[8], data[9])
	if data[10] != 0 {
		tzName += fmt.Sprintf(":%d", data[10])
	}

	tzOff := 3600*int(data[9]) + 60*int(data[10])
	if data[8] == '-' {
		tzOff = -tzOff
	}

	tz := time.FixedZone(tzName, tzOff)

	// Decode time
	t := time.Date(
		int(binary.BigEndian.Uint16(data[0:2])), // year
		time.Month(data[2]),                     // month
		int(data[3]),                            // day
		int(data[4]),                            // hour
		int(data[5]),                            // min
		int(data[6]),                            // sec
		int(data[7])*100000000,                  // nsec
		tz,                                      // time zone
	)

	return Time{t}, nil
}

// Resolution is the Value that represents image resolution.
//
// Use with: TagResolution
type Resolution struct {
	Xres, Yres int   // X/Y resolutions
	Units      Units // Resolution units
}

// String converts Resolution value to string
func (v Resolution) String() string {
	return fmt.Sprintf("%dx%d%s", v.Xres, v.Yres, v.Units)
}

// Type returns type of Value (TypeResolution for Resolution)
func (Resolution) Type() Type { return TypeResolution }

// DeepCopy returns a deep copy of the Resolution Value
func (v Resolution) DeepCopy() Value {
	return v
}

// Encode Resolution Value into wire format
func (v Resolution) encode() ([]byte, error) {
	// Wire format
	//    4 bytes: Xres
	//    4 bytes: Yres
	//    1 byte:  Units

	x, y := v.Xres, v.Yres

	return []byte{
		byte(x >> 24), byte(x >> 16), byte(x >> 8), byte(x),
		byte(y >> 24), byte(y >> 16), byte(y >> 8), byte(y),
		byte(v.Units),
	}, nil
}

// Decode Resolution Value from wire format
func (Resolution) decode(data []byte) (Value, error) {
	if len(data) != 9 {
		return nil, errors.New("value must be 9 bytes")
	}

	return Resolution{
		Xres:  int(int32(binary.BigEndian.Uint32(data[0:4]))),
		Yres:  int(int32(binary.BigEndian.Uint32(data[4:8]))),
		Units: Units(data[8]),
	}, nil

}

// Units represents resolution units
type Units uint8

// Resolution units codes
const (
	UnitsDpi  Units = 3 // Dots per inch
	UnitsDpcm Units = 4 // Dots per cm
)

// String converts Units to string
func (u Units) String() string {
	switch u {
	case UnitsDpi:
		return "dpi"
	case UnitsDpcm:
		return "dpcm"
	default:
		return fmt.Sprintf("unknown(0x%2.2x)", uint8(u))
	}
}

// Range is the Value that represents a range of 32-bit signed integers
//
// Use with: TagRange
type Range struct {
	Lower, Upper int // Lower/upper bounds
}

// String converts Range value to string
func (v Range) String() string {
	return fmt.Sprintf("%d-%d", v.Lower, v.Upper)
}

// Type returns type of Value (TypeRange for Range)
func (Range) Type() Type { return TypeRange }

// DeepCopy returns a deep copy of the Range Value
func (v Range) DeepCopy() Value {
	return v
}

// Encode Range Value into wire format
func (v Range) encode() ([]byte, error) {
	// Wire format
	//    4 bytes: Lower
	//    4 bytes: Upper

	l, u := v.Lower, v.Upper

	return []byte{
		byte(l >> 24), byte(l >> 16), byte(l >> 8), byte(l),
		byte(u >> 24), byte(u >> 16), byte(u >> 8), byte(u),
	}, nil
}

// Within checks that x fits within the range
//
// It implements IntegerOrRange interface
func (v Range) Within(x int) bool {
	return v.Lower <= x && x <= v.Upper
}

// Decode Range Value from wire format
func (Range) decode(data []byte) (Value, error) {
	if len(data) != 8 {
		return nil, errors.New("value must be 8 bytes")
	}

	return Range{
		Lower: int(int32(binary.BigEndian.Uint32(data[0:4]))),
		Upper: int(int32(binary.BigEndian.Uint32(data[4:8]))),
	}, nil
}

// TextWithLang is the Value that represents a combination
// of two strings:
//   - text on some natural language (i.e., "hello")
//   - name of that language (i.e., "en")
//
// Use with: TagTextLang, TagNameLang
type TextWithLang struct {
	Lang, Text string // Language and text
}

// String converts TextWithLang value to string
func (v TextWithLang) String() string { return v.Text + " [" + v.Lang + "]" }

// Type returns type of Value (TypeTextWithLang for TextWithLang)
func (TextWithLang) Type() Type { return TypeTextWithLang }

// DeepCopy returns a deep copy of the TextWithLang Value
func (v TextWithLang) DeepCopy() Value {
	return v
}

// Encode TextWithLang Value into wire format
func (v TextWithLang) encode() ([]byte, error) {
	// Wire format
	//    2 bytes:  len(Lang)
	//    variable: Lang
	//    2 bytes:  len(Text)
	//    variable: Text

	lang := []byte(v.Lang)
	text := []byte(v.Text)

	if len(lang) > math.MaxUint16 {
		return nil, fmt.Errorf("Lang exceeds %d bytes", math.MaxUint16)
	}

	if len(text) > math.MaxUint16 {
		return nil, fmt.Errorf("Text exceeds %d bytes", math.MaxUint16)
	}

	data := make([]byte, 2+2+len(lang)+len(text))
	binary.BigEndian.PutUint16(data, uint16(len(lang)))
	copy(data[2:], []byte(lang))

	data2 := data[2+len(lang):]
	binary.BigEndian.PutUint16(data2, uint16(len(text)))
	copy(data2[2:], []byte(text))

	return data, nil
}

// Decode TextWithLang Value from wire format
func (TextWithLang) decode(data []byte) (Value, error) {
	var langLen, textLen int
	var lang, text string

	// Unpack language length
	if len(data) < 2 {
		return nil, errors.New("truncated language length")
	}

	langLen = int(binary.BigEndian.Uint16(data[0:2]))
	data = data[2:]

	// Unpack language value
	if len(data) < langLen {
		return nil, errors.New("truncated language name")
	}

	lang = string(data[:langLen])
	data = data[langLen:]

	// Unpack text length
	if len(data) < 2 {
		return nil, errors.New("truncated text length")
	}

	textLen = int(binary.BigEndian.Uint16(data[0:2]))
	data = data[2:]

	// Unpack text value
	if len(data) < textLen {
		return nil, errors.New("truncated text string")
	}

	text = string(data[:textLen])
	data = data[textLen:]

	// We must have consumed all bytes at this point
	if len(data) != 0 {
		return nil, fmt.Errorf("extra %d bytes at the end of value",
			len(data))
	}

	// Return a value
	return TextWithLang{Lang: lang, Text: text}, nil
}

// Binary is the Value that represents a raw binary data
type Binary []byte

// String converts Binary value to string
func (v Binary) String() string {
	return fmt.Sprintf("%x", []byte(v))
}

// Type returns type of Value (TypeBinary for Binary)
func (Binary) Type() Type { return TypeBinary }

// DeepCopy returns a deep copy of the Binary Value
func (v Binary) DeepCopy() Value {
	v2 := make(Binary, len(v))
	copy(v2, v)
	return v2
}

// Encode TextWithLang Value into wire format
func (v Binary) encode() ([]byte, error) {
	return []byte(v), nil
}

// Decode Binary Value from wire format
func (Binary) decode(data []byte) (Value, error) {
	return Binary(data), nil
}

// Collection is the Value that represents collection of attributes
//
// Use with: TagBeginCollection
type Collection Attributes

// Add Attribute to Attributes
func (v *Collection) Add(attr Attribute) {
	*v = append(*v, attr)
}

// String converts Collection to string
func (v Collection) String() string {
	var buf bytes.Buffer
	buf.Write([]byte("{"))
	for i, attr := range v {
		if i > 0 {
			buf.Write([]byte(" "))
		}
		fmt.Fprintf(&buf, "%s=%s", attr.Name, attr.Values)
	}
	buf.Write([]byte("}"))

	return buf.String()
}

// Type returns type of Value (TypeCollection for Collection)
func (Collection) Type() Type { return TypeCollection }

// DeepCopy returns a deep copy of the Collection Value
func (v Collection) DeepCopy() Value {
	return Collection(Attributes(v).DeepCopy())
}

// Encode Collection Value into wire format
func (Collection) encode() ([]byte, error) {
	// Note, TagBeginCollection attribute contains
	// no data, collection itself handled the different way
	return []byte{}, nil
}

// Decode Collection Value from wire format
func (Collection) decode(data []byte) (Value, error) {
	panic("internal error")
}
