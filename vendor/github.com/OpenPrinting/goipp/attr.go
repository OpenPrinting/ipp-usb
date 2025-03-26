/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Message attributes
 */

package goipp

import (
	"fmt"
	"sort"
)

// Attributes represents a slice of attributes
type Attributes []Attribute

// Add Attribute to Attributes
func (attrs *Attributes) Add(attr Attribute) {
	*attrs = append(*attrs, attr)
}

// Clone creates a shallow copy of Attributes
func (attrs Attributes) Clone() Attributes {
	attrs2 := make(Attributes, len(attrs))
	copy(attrs2, attrs)
	return attrs2
}

// DeepCopy creates a deep copy of Attributes
func (attrs Attributes) DeepCopy() Attributes {
	attrs2 := make(Attributes, len(attrs))
	for i := range attrs {
		attrs2[i] = attrs[i].DeepCopy()
	}
	return attrs2
}

// Equal checks that attrs and attrs2 are equal
func (attrs Attributes) Equal(attrs2 Attributes) bool {
	if len(attrs) != len(attrs2) {
		return false
	}

	for i, attr := range attrs {
		attr2 := attrs2[i]
		if !attr.Equal(attr2) {
			return false
		}
	}

	return true
}

// Similar checks that attrs and attrs2 are **logically** equal,
// which means the following:
//   - attrs and addrs2 contain the same set of attributes,
//     but may be differently ordered
//   - Values of attributes of the same name within attrs and
//     attrs2 are similar
func (attrs Attributes) Similar(attrs2 Attributes) bool {
	// Fast check: if lengths are not the same, attributes
	// are definitely not equal
	if len(attrs) != len(attrs2) {
		return false
	}

	// Sort attrs and attrs2 by name
	sorted1 := attrs.Clone()
	sort.SliceStable(sorted1, func(i, j int) bool {
		return sorted1[i].Name < sorted1[j].Name
	})

	sorted2 := attrs2.Clone()
	sort.SliceStable(sorted2, func(i, j int) bool {
		return sorted2[i].Name < sorted2[j].Name
	})

	// And now compare sorted slices
	for i, attr1 := range sorted1 {
		attr2 := sorted2[i]
		if !attr1.Similar(attr2) {
			return false
		}
	}

	return true
}

// Attribute represents a single attribute, which consist of
// the Name and one or more Values
type Attribute struct {
	Name   string // Attribute name
	Values Values // Slice of values
}

// MakeAttribute makes Attribute with single value.
//
// Deprecated. Use [MakeAttr] instead.
func MakeAttribute(name string, tag Tag, value Value) Attribute {
	attr := Attribute{Name: name}
	attr.Values.Add(tag, value)
	return attr
}

// MakeAttr makes Attribute with one or more values.
func MakeAttr(name string, tag Tag, val1 Value, values ...Value) Attribute {
	attr := Attribute{Name: name}
	attr.Values.Add(tag, val1)
	for _, val := range values {
		attr.Values.Add(tag, val)
	}
	return attr
}

// MakeAttrCollection makes [Attribute] with [Collection] value.
func MakeAttrCollection(name string,
	member1 Attribute, members ...Attribute) Attribute {

	col := make(Collection, len(members)+1)
	col[0] = member1
	copy(col[1:], members)

	return MakeAttribute(name, TagBeginCollection, col)
}

// Equal checks that Attribute is equal to another Attribute
// (i.e., names are the same and values are equal)
func (a Attribute) Equal(a2 Attribute) bool {
	return a.Name == a2.Name && a.Values.Equal(a2.Values)
}

// Similar checks that Attribute is **logically** equal to another
// Attribute (i.e., names are the same and values are similar)
func (a Attribute) Similar(a2 Attribute) bool {
	return a.Name == a2.Name && a.Values.Similar(a2.Values)
}

// DeepCopy creates a deep copy of the Attribute
func (a Attribute) DeepCopy() Attribute {
	a2 := a
	a2.Values = a2.Values.DeepCopy()
	return a2
}

// Unpack attribute value from its wire representation
func (a *Attribute) unpack(tag Tag, value []byte) error {
	var err error
	var val Value

	switch tag.Type() {
	case TypeVoid, TypeCollection:
		val = Void{}

	case TypeInteger:
		val = Integer(0)

	case TypeBoolean:
		val = Boolean(false)

	case TypeString:
		val = String("")

	case TypeDateTime:
		val = Time{}

	case TypeResolution:
		val = Resolution{}

	case TypeRange:
		val = Range{}

	case TypeTextWithLang:
		val = TextWithLang{}

	case TypeBinary:
		val = Binary(nil)

	default:
		panic(fmt.Sprintf("(Attribute) uppack(): tag=%s type=%s", tag, tag.Type()))
	}

	val, err = val.decode(value)

	if err == nil {
		a.Values.Add(tag, val)
	} else {
		err = fmt.Errorf("%s: %s", tag, err)
	}

	return err
}
