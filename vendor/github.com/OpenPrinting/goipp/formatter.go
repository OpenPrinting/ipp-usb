/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * IPP formatter (pretty-printer)
 */

package goipp

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Formatter parameters:
const (
	// FormatterIndentShift is the indentation shift, number of space
	// characters per indentation level.
	FormatterIndentShift = 4
)

// Formatter formats IPP messages, attributes, groups etc
// for pretty-printing.
//
// It supersedes [Message.Print] method which is now considered
// deprecated.
type Formatter struct {
	indent     int          // Indentation level
	userIndent int          // User-settable indent
	buf        bytes.Buffer // Output buffer
}

// NewFormatter returns a new Formatter.
func NewFormatter() *Formatter {
	return &Formatter{}
}

// Reset resets the formatter.
func (f *Formatter) Reset() {
	f.buf.Reset()
	f.indent = 0
}

// SetIndent configures indentation. If parameter is greater that
// zero, the specified amount of white space will prepended to each
// non-empty output line.
func (f *Formatter) SetIndent(n int) {
	f.userIndent = 0
	if n > 0 {
		f.userIndent = n
	}
}

// Bytes returns formatted text as a byte slice.
func (f *Formatter) Bytes() []byte {
	return f.buf.Bytes()
}

// String returns formatted text as a string.
func (f *Formatter) String() string {
	return f.buf.String()
}

// WriteTo writes formatted text to w.
// It implements [io.WriterTo] interface.
func (f *Formatter) WriteTo(w io.Writer) (int64, error) {
	return f.buf.WriteTo(w)
}

// Printf writes formatted line into the [Formatter], automatically
// indented and with added newline at the end.
//
// It returns the number of bytes written and nil as an error (for
// consistency with other printf-like functions).
func (f *Formatter) Printf(format string, args ...interface{}) (int, error) {
	s := fmt.Sprintf(format, args...)
	lines := strings.Split(s, "\n")
	cnt := 0

	for _, line := range lines {
		if line != "" {
			cnt += f.doIndent()
		}

		f.buf.WriteString(line)
		f.buf.WriteByte('\n')
		cnt += len(line) + 1
	}

	return cnt, nil
}

// FmtRequest formats a request [Message].
func (f *Formatter) FmtRequest(msg *Message) {
	f.fmtMessage(msg, true)
}

// FmtResponse formats a response [Message].
func (f *Formatter) FmtResponse(msg *Message) {
	f.fmtMessage(msg, false)
}

// fmtMessage formats a request or response Message.
func (f *Formatter) fmtMessage(msg *Message, request bool) {
	f.Printf("{")
	f.indent++

	f.Printf("REQUEST-ID %d", msg.RequestID)
	f.Printf("VERSION %s", msg.Version)

	if request {
		f.Printf("OPERATION %s", Op(msg.Code))
	} else {
		f.Printf("STATUS %s", Status(msg.Code))
	}

	if groups := msg.AttrGroups(); len(groups) != 0 {
		f.Printf("")
		f.FmtGroups(groups)
	}

	f.indent--
	f.Printf("}")
}

// FmtGroups formats a [Groups] slice.
func (f *Formatter) FmtGroups(groups Groups) {
	for i, g := range groups {
		if i != 0 {
			f.Printf("")
		}
		f.FmtGroup(g)
	}
}

// FmtGroup formats a single [Group].
func (f *Formatter) FmtGroup(g Group) {
	f.Printf("GROUP %s", g.Tag)
	f.FmtAttributes(g.Attrs)
}

// FmtAttributes formats a [Attributes] slice.
func (f *Formatter) FmtAttributes(attrs Attributes) {
	for _, attr := range attrs {
		f.FmtAttribute(attr)
	}
}

// FmtAttribute formats a single [Attribute].
func (f *Formatter) FmtAttribute(attr Attribute) {
	f.fmtAttributeOrMember(attr, false)
}

// FmtAttributes formats a single [Attribute] or collection member.
func (f *Formatter) fmtAttributeOrMember(attr Attribute, member bool) {
	buf := &f.buf

	f.doIndent()
	if member {
		fmt.Fprintf(buf, "MEMBER %q", attr.Name)
	} else {
		fmt.Fprintf(buf, "ATTR %q", attr.Name)
	}

	tag := TagZero
	for _, val := range attr.Values {
		if val.T != tag {
			fmt.Fprintf(buf, " %s:", val.T)
			tag = val.T
		}

		if collection, ok := val.V.(Collection); ok {
			if f.onNL() {
				f.Printf("{")
			} else {
				buf.Write([]byte(" {\n"))
			}

			f.indent++

			for _, attr2 := range collection {
				f.fmtAttributeOrMember(attr2, true)
			}

			f.indent--
			f.Printf("}")
		} else {
			fmt.Fprintf(buf, " %s", val.V)
		}
	}

	f.forceNL()
}

// onNL returns true if Formatter is at the beginning of new line.
func (f *Formatter) onNL() bool {
	b := f.buf.Bytes()
	return len(b) == 0 || b[len(b)-1] == '\n'
}

// forceNL inserts newline character if Formatter is not at the.
// beginning of new line
func (f *Formatter) forceNL() {
	if !f.onNL() {
		f.buf.WriteByte('\n')
	}
}

// doIndent outputs indentation space.
// It returns number of characters written.
func (f *Formatter) doIndent() int {
	cnt := FormatterIndentShift * f.indent
	cnt += f.userIndent

	n := cnt
	for n > len(formatterSomeSpace) {
		f.buf.Write([]byte(formatterSomeSpace[:]))
		n -= len(formatterSomeSpace)
	}

	f.buf.Write([]byte(formatterSomeSpace[:n]))

	return cnt
}

// formatterSomeSpace contains some space characters for
// fast output of indentation space.
var formatterSomeSpace [64]byte

func init() {
	for i := range formatterSomeSpace {
		formatterSomeSpace[i] = ' '
	}
}
