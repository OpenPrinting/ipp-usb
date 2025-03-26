/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * IPP Message decoder
 */

package goipp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// DecoderOptions represents message decoder options
type DecoderOptions struct {
	// EnableWorkarounds, if set to true, enables various workarounds
	// for decoding IPP messages that violate IPP protocol specification
	//
	// Currently it includes the following workarounds:
	// * Pantum M7300FDW violates collection encoding rules.
	//   Instead of using TagMemberName, it uses named attributes
	//   within the collection
	//
	// The list of implemented workarounds may grow in the
	// future
	EnableWorkarounds bool
}

// messageDecoder represents Message decoder
type messageDecoder struct {
	in  io.Reader      // Input stream
	off int            // Offset of last read
	cnt int            // Count of read bytes
	opt DecoderOptions // Options
}

// Decode the message
func (md *messageDecoder) decode(m *Message) error {
	// Wire format:
	//
	//   2 bytes:  Version
	//   2 bytes:  Code (Operation or Status)
	//   4 bytes:  RequestID
	//   variable: attributes
	//   1 byte:   TagEnd

	// Parse message header
	var err error
	m.Version, err = md.decodeVersion()
	if err == nil {
		m.Code, err = md.decodeCode()
	}
	if err == nil {
		m.RequestID, err = md.decodeU32()
	}

	// Now parse attributes
	done := false
	var group *Attributes
	var attr Attribute
	var prev *Attribute

	for err == nil && !done {
		var tag Tag
		tag, err = md.decodeTag()

		if err != nil {
			break
		}

		if tag.IsDelimiter() {
			prev = nil
		}

		if tag.IsGroup() {
			m.Groups.Add(Group{tag, nil})
		}

		switch tag {
		case TagZero:
			err = errors.New("Invalid tag 0")
		case TagEnd:
			done = true

		case TagOperationGroup:
			group = &m.Operation
		case TagJobGroup:
			group = &m.Job
		case TagPrinterGroup:
			group = &m.Printer
		case TagUnsupportedGroup:
			group = &m.Unsupported
		case TagSubscriptionGroup:
			group = &m.Subscription
		case TagEventNotificationGroup:
			group = &m.EventNotification
		case TagResourceGroup:
			group = &m.Resource
		case TagDocumentGroup:
			group = &m.Document
		case TagSystemGroup:
			group = &m.System
		case TagFuture11Group:
			group = &m.Future11
		case TagFuture12Group:
			group = &m.Future12
		case TagFuture13Group:
			group = &m.Future13
		case TagFuture14Group:
			group = &m.Future14
		case TagFuture15Group:
			group = &m.Future15

		default:
			// Decode attribute
			if tag == TagMemberName || tag == TagEndCollection {
				err = fmt.Errorf("Unexpected tag %s", tag)
			} else {
				attr, err = md.decodeAttribute(tag)
			}

			if err == nil && tag == TagBeginCollection {
				attr.Values[0].V, err = md.decodeCollection()
			}

			// If everything is OK, save attribute
			switch {
			case err != nil:
			case attr.Name == "":
				if prev != nil {
					prev.Values.Add(attr.Values[0].T, attr.Values[0].V)

					// Append value to the last Attribute of the
					// last Group in the m.Groups
					//
					// Note, if we are here, this last Attribute definitely exists,
					// because:
					//   * prev != nil
					//   * prev is set when new named attribute is added
					//   * prev is reset when delimiter tag is encountered
					gLast := &m.Groups[len(m.Groups)-1]
					aLast := &gLast.Attrs[len(gLast.Attrs)-1]
					aLast.Values.Add(attr.Values[0].T, attr.Values[0].V)
				} else {
					err = errors.New("Additional value without preceding attribute")
				}
			case group != nil:
				group.Add(attr)
				prev = &(*group)[len(*group)-1]
				m.Groups[len(m.Groups)-1].Add(attr)
			default:
				err = errors.New("Attribute without a group")
			}
		}
	}

	if err != nil {
		err = fmt.Errorf("%s at 0x%x", err, md.off)
	}

	return err
}

// Decode a Collection
//
// Collection is like a nested object - an attribute which value is a sequence
// of named attributes. Collections can be nested.
//
// Wire format:
//   ATTR: Tag = TagBeginCollection,            - the outer attribute that
//         Name = "name", value - ignored         contains the collection
//
//   ATTR: Tag = TagMemberName, name = "",      - member name  \
//         value - string, name of the next                     |
//         member                                               | repeated for
//                                                              | each member
//   ATTR: Tag = any attribute tag, name = "",  - repeated for  |
//         value = member value                   multi-value  /
//                                                members
//
//   ATTR: Tag = TagEndCollection, name = "",
//         value - ignored
//
// The format looks a bit baroque, but please note that it was added
// in the IPP 2.0. For IPP 1.x collection looks like a single multi-value
// TagBeginCollection attribute (attributes without names considered
// next value for the previously defined named attributes) and so
// 1.x parser silently ignores collections and doesn't get confused
// with them.
func (md *messageDecoder) decodeCollection() (Collection, error) {
	collection := make(Collection, 0)

	memberName := ""

	for {
		tag, err := md.decodeTag()
		if err != nil {
			return nil, err
		}

		// Delimiter cannot be inside a collection
		if tag.IsDelimiter() {
			err = fmt.Errorf("Collection: unexpected tag %s", tag)
			return nil, err
		}

		// Check for TagMemberName without the subsequent value attribute
		if (tag == TagMemberName || tag == TagEndCollection) && memberName != "" {
			err = fmt.Errorf("Collection: unexpected %s, expected value tag", tag)
			return nil, err
		}

		// Fetch next attribute
		attr, err := md.decodeAttribute(tag)
		if err != nil {
			return nil, err
		}

		// Process next attribute
		switch tag {
		case TagEndCollection:
			return collection, nil

		case TagMemberName:
			memberName = string(attr.Values[0].V.(String))
			if memberName == "" {
				err = fmt.Errorf("Collection: %s value is empty", tag)
				return nil, err
			}

		case TagBeginCollection:
			// Decode nested collection
			attr.Values[0].V, err = md.decodeCollection()
			if err != nil {
				return nil, err
			}
			fallthrough

		default:
			if md.opt.EnableWorkarounds &&
				memberName == "" && attr.Name != "" {
				// Workaround for: Pantum M7300FDW
				//
				// This device violates collection encoding rules.
				// Instead of using TagMemberName, it uses named
				// attributes within the collection
				memberName = attr.Name
			}

			if memberName != "" {
				attr.Name = memberName
				collection = append(collection, attr)
				memberName = ""
			} else if len(collection) > 0 {
				l := len(collection)
				collection[l-1].Values.Add(tag, attr.Values[0].V)
			} else {
				// We've got a value without preceding TagMemberName
				err = fmt.Errorf("Collection: unexpected %s, expected %s", tag, TagMemberName)
				return nil, err
			}
		}
	}
}

// Decode a tag
func (md *messageDecoder) decodeTag() (Tag, error) {
	t, err := md.decodeU8()

	return Tag(t), err
}

// Decode a Version
func (md *messageDecoder) decodeVersion() (Version, error) {
	code, err := md.decodeU16()
	return Version(code), err
}

// Decode a Code
func (md *messageDecoder) decodeCode() (Code, error) {
	code, err := md.decodeU16()
	return Code(code), err
}

// Decode a single attribute
//
// Wire format:
//   1   byte:   Tag
//   2+N bytes:  Name length (2 bytes) + name string
//   2+N bytes:  Value length (2 bytes) + value bytes
//
// For the extended tag format, Tag is encoded as TagExtension and
// 4 bytes of the actual tag value prepended to the value bytes
func (md *messageDecoder) decodeAttribute(tag Tag) (Attribute, error) {
	var attr Attribute
	var value []byte
	var err error

	// Obtain attribute name and raw value
	attr.Name, err = md.decodeString()
	if err != nil {
		goto ERROR
	}

	value, err = md.decodeBytes()
	if err != nil {
		goto ERROR
	}

	// Handle TagExtension
	if tag == TagExtension {
		if len(value) < 4 {
			err = errors.New("Extension tag truncated")
			goto ERROR
		}

		t := binary.BigEndian.Uint32(value[:4])

		if t > 0x7fffffff {
			err = fmt.Errorf(
				"Extension tag 0x%8.8x out of range", t)
			goto ERROR
		}
	}

	// Unpack value
	err = attr.unpack(tag, value)
	if err != nil {
		goto ERROR
	}

	return attr, nil

	// Return a error
ERROR:
	return Attribute{}, err
}

// Decode a 8-bit integer
func (md *messageDecoder) decodeU8() (uint8, error) {
	buf := make([]byte, 1)
	err := md.read(buf)
	return buf[0], err
}

// Decode a 16-bit integer
func (md *messageDecoder) decodeU16() (uint16, error) {
	buf := make([]byte, 2)
	err := md.read(buf)
	return binary.BigEndian.Uint16(buf[:]), err
}

// Decode a 32-bit integer
func (md *messageDecoder) decodeU32() (uint32, error) {
	buf := make([]byte, 4)
	err := md.read(buf)
	return binary.BigEndian.Uint32(buf[:]), err
}

// Decode sequence of bytes
func (md *messageDecoder) decodeBytes() ([]byte, error) {
	length, err := md.decodeU16()
	if err != nil {
		return nil, err
	}

	data := make([]byte, length)
	err = md.read(data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Decode string
func (md *messageDecoder) decodeString() (string, error) {
	data, err := md.decodeBytes()
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// Read a piece of raw data from input stream
func (md *messageDecoder) read(data []byte) error {
	md.off = md.cnt

	for len(data) > 0 {
		n, err := md.in.Read(data)
		if n > 0 {
			md.cnt += n
			data = data[n:]
		} else {
			md.off = md.cnt
			if err == nil || err == io.EOF {
				err = errors.New("Message truncated")
			}
			return err
		}

	}

	return nil
}
