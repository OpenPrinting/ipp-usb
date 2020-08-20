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
	file   *os.File // Underlying file
	reader *bufio.Reader
	buf    bytes.Buffer
	rec    IniRecord // Next record
}

// IniRecord represents a single .INI file record
type IniRecord struct {
	Section    string // Section name
	Key, Value string // Key and value
	File       string // Origin file
	Line       int    // Line in that file
}

// IniError represents an .INI file read error
type IniError struct {
	File    string // Origin file
	Line    int    // Line in that file
	Message string // Error message
}

// Open the .INI file
func OpenIniFile(path string) (ini *IniFile, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	ini = &IniFile{
		file: f,
		rec: IniRecord{
			File: path,
			Line: 1,
		},
	}

	if f != nil {
		ini.reader = bufio.NewReader(f)
	} else {
		ini.reader = bufio.NewReader(bytes.NewBuffer(nil))

	}

	return ini, nil
}

// Close the .INI file
func (ini *IniFile) Close() error {
	return ini.file.Close()
}

// Read next IniRecord
func (ini *IniFile) Next() (*IniRecord, error) {
	for {
		// Read until next non-space character, skipping all comments
		c, err := ini.getc_nonspace()
		for err == nil && ini.iscomment(c) {
			ini.getc_nl()
			c, err = ini.getc_nonspace()
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

			ini.getc_nl()

		case '=':
			ini.getc_nl()
			return nil, ini.errorf("unexpected '=' character")

		default:
			ini.ungetc(c)

			c, token, err = ini.token('=', false)
			if err == nil && c == '=' {
				ini.rec.Key = token
				c, token, err = ini.token(-1, true)
				if err == nil {
					ini.rec.Value = token
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
	var accumulator, count, trailing_space int
	var c byte
	var err error
	type prsState int
	const (
		PRS_SKIP_SPACE prsState = iota
		PRS_BODY
		PRS_STRING
		PRS_STRING_BSLASH
		PRS_STRING_HEX
		PRS_STRING_OCTAL
		PRS_COMMENT
	)

	// Parse the string
	state := PRS_SKIP_SPACE
	ini.buf.Reset()

	for {
		c, err = ini.getc()
		if err != nil || c == '\n' {
			break
		}

		if (state == PRS_BODY || state == PRS_SKIP_SPACE) && rune(c) == delimiter {
			break
		}

		switch state {
		case PRS_SKIP_SPACE:
			if ini.isspace(c) {
				break
			}

			state = PRS_BODY
			fallthrough

		case PRS_BODY:
			if c == '"' {
				state = PRS_STRING
			} else if ini.iscomment(c) {
				state = PRS_COMMENT
			} else if c == '\\' && linecont {
				c2, _ := ini.getc()
				if c2 == '\n' {
					ini.buf.Truncate(ini.buf.Len() - trailing_space)
					trailing_space = 0
					state = PRS_SKIP_SPACE
				} else {
					ini.ungetc(c2)
				}
			} else {
				ini.buf.WriteByte(c)
			}

			if state == PRS_BODY {
				if ini.isspace(c) {
					trailing_space++
				} else {
					trailing_space = 0
				}
			} else {
				ini.buf.Truncate(ini.buf.Len() - trailing_space)
				trailing_space = 0
			}
			break

		case PRS_STRING:
			if c == '\\' {
				state = PRS_STRING_BSLASH
			} else if c == '"' {
				state = PRS_BODY
			} else {
				ini.buf.WriteByte(c)
			}
			break

		case PRS_STRING_BSLASH:
			if c == 'x' || c == 'X' {
				state = PRS_STRING_HEX
				accumulator, count = 0, 0
			} else if ini.isoctal(c) {
				state = PRS_STRING_OCTAL
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
				state = PRS_STRING
			}
			break

		case PRS_STRING_HEX:
			if ini.isxdigit(c) {
				if count != 2 {
					accumulator = accumulator*16 + ini.hex2int(c)
					count++
				}
			} else {
				state = PRS_STRING
				ini.ungetc(c)
			}

			if state != PRS_STRING_HEX {
				ini.buf.WriteByte(c)
			}
			break

		case PRS_STRING_OCTAL:
			if ini.isoctal(c) {
				accumulator = accumulator*8 + ini.hex2int(c)
				count++
				if count == 3 {
					state = PRS_STRING
				}
			} else {
				state = PRS_STRING
				ini.ungetc(c)
			}

			if state != PRS_STRING_OCTAL {
				ini.buf.WriteByte(c)
			}
			break

		case PRS_COMMENT:
			break
		}
	}

	// Remove trailing space, if any
	ini.buf.Truncate(ini.buf.Len() - trailing_space)

	// Check for syntax error
	if state != PRS_SKIP_SPACE && state != PRS_BODY && state != PRS_COMMENT {
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

// getc_nonspace returns a next non-space character from the input file
func (ini *IniFile) getc_nonspace() (byte, error) {
	for {
		c, err := ini.getc()
		if err != nil || !ini.isspace(c) {
			return c, err
		}
	}
}

// getc_nl returns a next newline character, or reads until EOF or error
func (ini *IniFile) getc_nl() (byte, error) {
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
