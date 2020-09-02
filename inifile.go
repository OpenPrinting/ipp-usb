/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * .INI file loader
 */

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
)

// IniFile represents opened .INI file
type IniFile struct {
	file        *os.File      // Underlying file
	reader      *bufio.Reader // Reader on a top of file
	buf         bytes.Buffer  // Temporary buffer to speed up things
	rec         IniRecord     // Next record
	withRecType bool          // Return records of any type
}

// IniRecord represents a single .INI file record
type IniRecord struct {
	Section    string        // Section name
	Key, Value string        // Key and value
	File       string        // Origin file
	Line       int           // Line in that file
	Type       IniRecordType // Record type
}

// IniRecordType represents IniRecord type
type IniRecordType int

const (
	IniRecordSection IniRecordType = iota
	IniRecordKeyVal
)

// IniError represents an .INI file read error
type IniError struct {
	File    string // Origin file
	Line    int    // Line in that file
	Message string // Error message
}

// OpenIniFile opens the .INI file for reading
//
// If file is opened this way, (*IniFile) Next() returns
// records of IniRecordKeyVal type only
func OpenIniFile(path string) (ini *IniFile, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	ini = &IniFile{
		file:   f,
		reader: bufio.NewReader(f),
		rec: IniRecord{
			File: path,
			Line: 1,
		},
	}

	return ini, nil
}

// OpenIniFileWithRecType opens the .INI file for reading
//
// If file is opened this way, (*IniFile) Next() returns
// records of any type
func OpenIniFileWithRecType(path string) (ini *IniFile, err error) {
	ini, err = OpenIniFile(path)
	if ini != nil {
		ini.withRecType = true
	}
	return
}

// Close the .INI file
func (ini *IniFile) Close() error {
	return ini.file.Close()
}

// Next returns next IniRecord or an error
func (ini *IniFile) Next() (*IniRecord, error) {
	for {
		// Read until next non-space character, skipping all comments
		c, err := ini.getcNonSpace()
		for err == nil && ini.iscomment(c) {
			ini.getcNl()
			c, err = ini.getcNonSpace()
		}

		if err != nil {
			return nil, err
		}

		// Parse next record
		var token string
		switch c {
		case '[':
			c, token, err = ini.token(']', false)

			if err == nil && c == ']' {
				ini.rec.Section = token
			}

			ini.getcNl()
			ini.rec.Type = IniRecordSection

			if ini.withRecType {
				return &ini.rec, nil
			}

		case '=':
			ini.getcNl()
			return nil, ini.errorf("unexpected '=' character")

		default:
			ini.ungetc(c)

			c, token, err = ini.token('=', false)
			if err == nil && c == '=' {
				ini.rec.Key = token
				c, token, err = ini.token(-1, true)
				if err == nil {
					ini.rec.Value = token
					ini.rec.Type = IniRecordKeyVal
					return &ini.rec, nil
				}
			} else if err == nil {
				return nil, ini.errorf("expected '=' character")
			}
		}
	}
}

// Read next token
func (ini *IniFile) token(delimiter rune, linecont bool) (byte, string, error) {
	var accumulator, count, trailingSpace int
	var c byte
	var err error
	type prsState int
	const (
		prsSkipSpace prsState = iota
		prsBody
		prsString
		prsStringBslash
		prsStringHex
		prsStringOctal
		prsComment
	)

	// Parse the string
	state := prsSkipSpace
	ini.buf.Reset()

	for {
		c, err = ini.getc()
		if err != nil || c == '\n' {
			break
		}

		if (state == prsBody || state == prsSkipSpace) && rune(c) == delimiter {
			break
		}

		switch state {
		case prsSkipSpace:
			if ini.isspace(c) {
				break
			}

			state = prsBody
			fallthrough

		case prsBody:
			if c == '"' {
				state = prsString
			} else if ini.iscomment(c) {
				state = prsComment
			} else if c == '\\' && linecont {
				c2, _ := ini.getc()
				if c2 == '\n' {
					ini.buf.Truncate(ini.buf.Len() - trailingSpace)
					trailingSpace = 0
					state = prsSkipSpace
				} else {
					ini.ungetc(c2)
				}
			} else {
				ini.buf.WriteByte(c)
			}

			if state == prsBody {
				if ini.isspace(c) {
					trailingSpace++
				} else {
					trailingSpace = 0
				}
			} else {
				ini.buf.Truncate(ini.buf.Len() - trailingSpace)
				trailingSpace = 0
			}
			break

		case prsString:
			if c == '\\' {
				state = prsStringBslash
			} else if c == '"' {
				state = prsBody
			} else {
				ini.buf.WriteByte(c)
			}
			break

		case prsStringBslash:
			if c == 'x' || c == 'X' {
				state = prsStringHex
				accumulator, count = 0, 0
			} else if ini.isoctal(c) {
				state = prsStringOctal
				accumulator = ini.hex2int(c)
				count = 1
			} else {
				switch c {
				case 'a':
					c = '\a'
					break
				case 'b':
					c = '\b'
					break
				case 'e':
					c = '\x1b'
					break
				case 'f':
					c = '\f'
					break
				case 'n':
					c = '\n'
					break
				case 'r':
					c = '\r'
					break
				case 't':
					c = '\t'
					break
				case 'v':
					c = '\v'
					break
				}

				ini.buf.WriteByte(c)
				state = prsString
			}
			break

		case prsStringHex:
			if ini.isxdigit(c) {
				if count != 2 {
					accumulator = accumulator*16 + ini.hex2int(c)
					count++
				}
			} else {
				state = prsString
				ini.ungetc(c)
			}

			if state != prsStringHex {
				ini.buf.WriteByte(c)
			}
			break

		case prsStringOctal:
			if ini.isoctal(c) {
				accumulator = accumulator*8 + ini.hex2int(c)
				count++
				if count == 3 {
					state = prsString
				}
			} else {
				state = prsString
				ini.ungetc(c)
			}

			if state != prsStringOctal {
				ini.buf.WriteByte(c)
			}
			break

		case prsComment:
			break
		}
	}

	// Remove trailing space, if any
	ini.buf.Truncate(ini.buf.Len() - trailingSpace)

	// Check for syntax error
	if state != prsSkipSpace && state != prsBody && state != prsComment {
		return 0, "", ini.errorf("unterminated string")
	}

	return c, ini.buf.String(), nil
}

// getc returns a next character from the input file
func (ini *IniFile) getc() (byte, error) {
	c, err := ini.reader.ReadByte()
	if c == '\n' {
		ini.rec.Line++
	}
	return c, err
}

// getcNonSpace returns a next non-space character from the input file
func (ini *IniFile) getcNonSpace() (byte, error) {
	for {
		c, err := ini.getc()
		if err != nil || !ini.isspace(c) {
			return c, err
		}
	}
}

// getcNl returns a next newline character, or reads until EOF or error
func (ini *IniFile) getcNl() (byte, error) {
	for {
		c, err := ini.getc()
		if err != nil || c == '\n' {
			return c, err
		}
	}
}

// ungetc pushes a character back to the input stream
// only one character can be unread this way
func (ini *IniFile) ungetc(c byte) {
	if c == '\n' {
		ini.rec.Line--
	}
	ini.reader.UnreadByte()
}

// isspace returns true, if character is whitespace
func (ini *IniFile) isspace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

// iscomment returns true, if character is commentary
func (ini *IniFile) iscomment(c byte) bool {
	return c == ';' || c == '#'
}

// isoctal returns true for octal digit
func (ini *IniFile) isoctal(c byte) bool {
	return '0' <= c && c <= '7'
}

// isoctal returns true for hexadecimal digit
func (ini *IniFile) isxdigit(c byte) bool {
	return ('0' <= c && c <= '7') ||
		('a' <= c && c <= 'f') ||
		('A' <= c && c <= 'F')
}

// hex2int return integer value of hexadecimal character
func (ini *IniFile) hex2int(c byte) int {
	switch {
	case '0' <= c && c <= '9':
		return int(c - '0')
	case 'a' <= c && c <= 'f':
		return int(c-'a') + 10
	case 'A' <= c && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}

// errorf creates a new IniError
func (ini *IniFile) errorf(format string, args ...interface{}) *IniError {
	return &IniError{
		File:    ini.rec.File,
		Line:    ini.rec.Line,
		Message: fmt.Sprintf(format, args...),
	}
}

// Error implements error interface for the IniError
func (err *IniError) Error() string {
	return fmt.Sprintf("%s:%d: %s", err.File, err.Line, err.Message)
}
