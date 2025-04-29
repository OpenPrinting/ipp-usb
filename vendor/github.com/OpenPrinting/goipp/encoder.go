/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * IPP Message encoder
 */

package goipp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Type messageEncoder represents Message encoder
type messageEncoder struct {
	out io.Writer // Output stream
}

// Encode the message
func (me *messageEncoder) encode(m *Message) error {
	// Wire format:
	//
	//   2 bytes:  Version
	//   2 bytes:  Code (Operation or Status)
	//   4 bytes:  RequestID
	//   variable: attributes
	//   1 byte:   TagEnd

	// Encode message header
	var err error
	err = me.encodeU16(uint16(m.Version))
	if err == nil {
		err = me.encodeU16(uint16(m.Code))
	}
	if err == nil {
		err = me.encodeU32(uint32(m.RequestID))
	}

	// Encode attributes
	for _, grp := range m.AttrGroups() {
		err = me.encodeTag(grp.Tag)
		if err == nil {
			for _, attr := range grp.Attrs {
				if attr.Name == "" {
					err = errors.New("Attribute without name")
				} else {
					err = me.encodeAttr(attr, true)
				}
			}
		}

		if err != nil {
			break
		}
	}

	if err == nil {
		err = me.encodeTag(TagEnd)
	}

	return err
}

// Encode attribute
func (me *messageEncoder) encodeAttr(attr Attribute, checkTag bool) error {
	// Wire format
	//     1 byte:   Tag
	//     2 bytes:  len(Name)
	//     variable: name
	//     2 bytes:  len(Value)
	//     variable  Value
	//
	// And each additional value comes as attribute
	// without name
	if len(attr.Values) == 0 {
		return errors.New("Attribute without value")
	}

	name := attr.Name
	for _, val := range attr.Values {
		tag := val.T

		if checkTag {
			if tag.IsDelimiter() || tag == TagMemberName || tag == TagEndCollection {
				return fmt.Errorf("Tag %s cannot be used with value", tag)
			}

			if uint(tag) >= 0x100 {
				return fmt.Errorf("Tag %s out of range", tag)
			}
		}

		err := me.encodeTag(tag)
		if err != nil {
			return err
		}

		err = me.encodeName(name)
		if err != nil {
			return err
		}

		err = me.encodeValue(val.T, val.V)
		if err != nil {
			return err
		}

		name = "" // Each additional value comes without name
	}

	return nil
}

// Encode 8-bit integer
func (me *messageEncoder) encodeU8(v uint8) error {
	return me.write([]byte{v})
}

// Encode 16-bit integer
func (me *messageEncoder) encodeU16(v uint16) error {
	return me.write([]byte{byte(v >> 8), byte(v)})
}

// Encode 32-bit integer
func (me *messageEncoder) encodeU32(v uint32) error {
	return me.write([]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

// Encode Tag
func (me *messageEncoder) encodeTag(tag Tag) error {
	return me.encodeU8(byte(tag))
}

// Encode Attribute name
func (me *messageEncoder) encodeName(name string) error {
	if len(name) > math.MaxInt16 {
		return fmt.Errorf("Attribute name exceeds %d bytes",
			math.MaxInt16)
	}

	err := me.encodeU16(uint16(len(name)))
	if err == nil {
		err = me.write([]byte(name))
	}

	return err
}

// Encode Attribute value
func (me *messageEncoder) encodeValue(tag Tag, v Value) error {
	// Check Value type vs the Tag
	tagType := tag.Type()
	if tagType == TypeVoid {
		v = Void{} // Ignore supplied value
	} else if tagType != v.Type() {
		return fmt.Errorf("Tag %s: %s value required, %s present",
			tag, tagType, v.Type())
	}

	// Convert Value to bytes in wire representation.
	data, err := v.encode()
	if err != nil {
		return err
	}

	if len(data) > math.MaxInt16 {
		return fmt.Errorf("Attribute value exceeds %d bytes",
			math.MaxInt16)
	}

	// TagExtension encoding rules enforcement.
	if tag == TagExtension {
		if len(data) < 4 {
			return fmt.Errorf(
				"Extension tag truncated (%d bytes)", len(data))
		}

		t := binary.BigEndian.Uint32(data)
		if t > 0x7fffffff {
			return fmt.Errorf(
				"Extension tag 0x%8.8x out of range", t)
		}
	}

	// Encode the value
	err = me.encodeU16(uint16(len(data)))
	if err == nil {
		err = me.write(data)
	}

	// Handle collection
	if collection, ok := v.(Collection); ok {
		return me.encodeCollection(tag, collection)
	}

	return err
}

// Encode collection
func (me *messageEncoder) encodeCollection(tag Tag, collection Collection) error {
	for _, attr := range collection {
		if attr.Name == "" {
			return errors.New("Collection member without name")
		}

		attrName := MakeAttribute("", TagMemberName, String(attr.Name))

		err := me.encodeAttr(attrName, false)
		if err == nil {
			err = me.encodeAttr(
				Attribute{Name: "", Values: attr.Values}, true)
		}

		if err != nil {
			return err
		}
	}

	return me.encodeAttr(MakeAttribute("", TagEndCollection, Void{}), false)
}

// Write a piece of raw data to output stream
func (me *messageEncoder) write(data []byte) error {
	for len(data) > 0 {
		n, err := me.out.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}

	return nil
}
