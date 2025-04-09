/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * IPP protocol messages
 */

package goipp

import (
	"bytes"
	"fmt"
	"io"
)

// Code represents Op(operation) or Status codes
type Code uint16

// Version represents a protocol version. It consist
// of Major and Minor version codes, packed into a single
// 16-bit word
type Version uint16

// DefaultVersion is the default IPP version (2.0 for now)
const DefaultVersion Version = 0x0200

// MakeVersion makes version from major and minor parts
func MakeVersion(major, minor uint8) Version {
	return Version(major)<<8 | Version(minor)
}

// Major returns a major part of version
func (v Version) Major() uint8 {
	return uint8(v >> 8)
}

// Minor returns a minor part of version
func (v Version) Minor() uint8 {
	return uint8(v)
}

// String() converts version to string (i.e., "2.0")
func (v Version) String() string {
	return fmt.Sprintf("%d.%d", v.Major(), v.Minor())
}

// Message represents a single IPP message, which may be either
// client request or server response
type Message struct {
	// Common header
	Version   Version // Protocol version
	Code      Code    // Operation for request, status for response
	RequestID uint32  // Set in request, returned in response

	// Groups of Attributes
	//
	// This field allows to represent messages with repeated
	// groups of attributes with the same group tag. The most
	// noticeable use case is the Get-Jobs response which uses
	// multiple Job groups, one per returned job. See RFC 8011,
	// 4.2.6.2. for more details
	//
	// See also the following discussions which explain the demand
	// to implement this interface:
	//   https://github.com/OpenPrinting/goipp/issues/2
	//   https://github.com/OpenPrinting/goipp/pull/3
	//
	// With respect to backward compatibility, the following
	// behavior is implemented here:
	//   1. (*Message).Decode() fills both Groups and named per-group
	//      fields (i.e., Operation, Job etc)
	//   2. (*Message).Encode() and (*Message) Print, if Groups != nil,
	//      uses Groups and ignores  named per-group fields. Otherwise,
	//      named fields are used as in 1.0.0
	//   3. (*Message) Equal(), for each message uses Groups if
	//      it is not nil or named per-group fields otherwise.
	//      In another words, Equal() compares messages as if
	//      they were encoded
	//
	// Since 1.1.0
	Groups Groups

	// Attributes, by group
	Operation         Attributes // Operation attributes
	Job               Attributes // Job attributes
	Printer           Attributes // Printer attributes
	Unsupported       Attributes // Unsupported attributes
	Subscription      Attributes // Subscription attributes
	EventNotification Attributes // Event Notification attributes
	Resource          Attributes // Resource attributes
	Document          Attributes // Document attributes
	System            Attributes // System attributes
	Future11          Attributes // \
	Future12          Attributes //  \
	Future13          Attributes //   | Reserved for future extensions
	Future14          Attributes //  /
	Future15          Attributes // /
}

// NewRequest creates a new request message
//
// Use DefaultVersion as a first argument, if you don't
// have any specific needs
func NewRequest(v Version, op Op, id uint32) *Message {
	return &Message{
		Version:   v,
		Code:      Code(op),
		RequestID: id,
	}
}

// NewResponse creates a new response message
//
// Use DefaultVersion as a first argument, if you don't
func NewResponse(v Version, status Status, id uint32) *Message {
	return &Message{
		Version:   v,
		Code:      Code(status),
		RequestID: id,
	}
}

// NewMessageWithGroups creates a new message with Groups of
// attributes.
//
// Fields like m.Operation, m.Job. m.Printer... and so on will
// be properly filled automatically.
func NewMessageWithGroups(v Version, code Code,
	id uint32, groups Groups) *Message {

	m := &Message{
		Version:   v,
		Code:      code,
		RequestID: id,
		Groups:    groups,
	}

	for _, grp := range m.Groups {
		switch grp.Tag {
		case TagOperationGroup:
			m.Operation = append(m.Operation, grp.Attrs...)
		case TagJobGroup:
			m.Job = append(m.Job, grp.Attrs...)
		case TagPrinterGroup:
			m.Printer = append(m.Printer, grp.Attrs...)
		case TagUnsupportedGroup:
			m.Unsupported = append(m.Unsupported, grp.Attrs...)
		case TagSubscriptionGroup:
			m.Subscription = append(m.Subscription, grp.Attrs...)
		case TagEventNotificationGroup:
			m.EventNotification = append(m.EventNotification,
				grp.Attrs...)
		case TagResourceGroup:
			m.Resource = append(m.Resource, grp.Attrs...)
		case TagDocumentGroup:
			m.Document = append(m.Document, grp.Attrs...)
		case TagSystemGroup:
			m.System = append(m.System, grp.Attrs...)
		case TagFuture11Group:
			m.Future11 = append(m.Future11, grp.Attrs...)
		case TagFuture12Group:
			m.Future12 = append(m.Future12, grp.Attrs...)
		case TagFuture13Group:
			m.Future13 = append(m.Future13, grp.Attrs...)
		case TagFuture14Group:
			m.Future14 = append(m.Future14, grp.Attrs...)
		case TagFuture15Group:
			m.Future15 = append(m.Future15, grp.Attrs...)
		}
	}

	return m
}

// Equal checks that two messages are equal
func (m Message) Equal(m2 Message) bool {
	if m.Version != m2.Version ||
		m.Code != m2.Code ||
		m.RequestID != m2.RequestID {
		return false
	}

	groups := m.AttrGroups()
	groups2 := m2.AttrGroups()

	return groups.Equal(groups2)
}

// Similar checks that two messages are **logically** equal,
// which means the following:
//   - Version, Code and RequestID are equal
//   - Groups of attributes are Similar
func (m Message) Similar(m2 Message) bool {
	if m.Version != m2.Version ||
		m.Code != m2.Code ||
		m.RequestID != m2.RequestID {
		return false
	}

	groups := m.AttrGroups()
	groups2 := m2.AttrGroups()

	return groups.Similar(groups2)
}

// Reset the message into initial state
func (m *Message) Reset() {
	*m = Message{}
}

// Encode message
func (m *Message) Encode(out io.Writer) error {
	me := messageEncoder{
		out: out,
	}

	return me.encode(m)
}

// EncodeBytes encodes message to byte slice
func (m *Message) EncodeBytes() ([]byte, error) {
	var buf bytes.Buffer

	err := m.Encode(&buf)
	return buf.Bytes(), err
}

// Decode reads message from io.Reader
func (m *Message) Decode(in io.Reader) error {
	return m.DecodeEx(in, DecoderOptions{})
}

// DecodeEx reads message from io.Reader
//
// It is extended version of the Decode method, with additional
// DecoderOptions parameter
func (m *Message) DecodeEx(in io.Reader, opt DecoderOptions) error {
	md := messageDecoder{
		in:  in,
		opt: opt,
	}

	m.Reset()
	return md.decode(m)
}

// DecodeBytes decodes message from byte slice
func (m *Message) DecodeBytes(data []byte) error {
	return m.Decode(bytes.NewBuffer(data))
}

// DecodeBytesEx decodes message from byte slice
//
// It is extended version of the DecodeBytes method, with additional
// DecoderOptions parameter
func (m *Message) DecodeBytesEx(data []byte, opt DecoderOptions) error {
	return m.DecodeEx(bytes.NewBuffer(data), opt)
}

// Print pretty-prints the message. The 'request' parameter affects
// interpretation of Message.Code: it is interpreted either
// as [Op] or as [Status].
//
// Deprecated. Use [Formatter] instead.
func (m *Message) Print(out io.Writer, request bool) {
	f := Formatter{}

	if request {
		f.FmtRequest(m)
	} else {
		f.FmtResponse(m)
	}

	f.WriteTo(out)
}

// AttrGroups returns [Message] attributes as a sequence of
// attribute groups.
//
// If [Message.Groups] is set, it will be returned.
//
// Otherwise, [Groups] will be reconstructed from [Message.Operation],
// [Message.Job], [Message.Printer] and so on.
//
// Groups with nil [Group.Attrs] will be skipped, but groups with non-nil
// will be not, even if len(Attrs) == 0
func (m *Message) AttrGroups() Groups {
	// If m.Groups is set, use it
	if m.Groups != nil {
		return m.Groups
	}

	// Initialize slice of groups
	groups := Groups{
		{TagOperationGroup, m.Operation},
		{TagJobGroup, m.Job},
		{TagPrinterGroup, m.Printer},
		{TagUnsupportedGroup, m.Unsupported},
		{TagSubscriptionGroup, m.Subscription},
		{TagEventNotificationGroup, m.EventNotification},
		{TagResourceGroup, m.Resource},
		{TagDocumentGroup, m.Document},
		{TagSystemGroup, m.System},
		{TagFuture11Group, m.Future11},
		{TagFuture12Group, m.Future12},
		{TagFuture13Group, m.Future13},
		{TagFuture14Group, m.Future14},
		{TagFuture15Group, m.Future15},
	}

	// Skip all empty groups
	out := 0
	for in := 0; in < len(groups); in++ {
		if groups[in].Attrs != nil {
			groups[out] = groups[in]
			out++
		}
	}

	return groups[:out]
}
