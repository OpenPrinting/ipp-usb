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
)

// Attributes represents a slice of attributes
type Attributes []Attribute

// Add Attribute to Attributes
func (attrs *Attributes) Add(attr Attribute) {
	*attrs = append(*attrs, attr)
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

// Attribute represents a single attribute, which consist of
// the Name and one or more Values
type Attribute struct {
	Name   string // Attribute name
	Values Values // Slice of values
}

// MakeAttribute makes Attribute with single value
func MakeAttribute(name string, tag Tag, value Value) Attribute {
	attr := Attribute{Name: name}
	attr.Values.Add(tag, value)
	return attr
}

// Equal checks that Attribute is equal to another Attribute
// (i.e., names are the same and values are equal)
func (a Attribute) Equal(a2 Attribute) bool {
	return a.Name == a2.Name && a.Values.Equal(a2.Values)
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
