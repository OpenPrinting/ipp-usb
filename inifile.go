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
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// IniFile represents opened .INI file
type IniFile struct {
	file        *os.File      // Underlying file
	line        int           // Line in that file
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

// Record types:
//   [section]       <- IniRecordSection
//     key - value   <- IniRecordKeyVal
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
		line:   1,
		reader: bufio.NewReader(f),
		rec: IniRecord{
			File: path,
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
		ini.rec.Line = ini.line
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
		ini.line++
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
		ini.line--
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

// LoadIPPort loads IP port value
// The destination remains untouched in a case of an error
func (rec *IniRecord) LoadIPPort(out *int) error {
	port, err := strconv.Atoi(rec.Value)
	if err == nil && (port < 1 || port > 65535) {
		err = rec.errBadValue("must be in range 1...65535")
	}
	if err != nil {
		return err
	}

	*out = port
	return nil
}

// LoadBool loads boolean value
// The destination remains untouched in a case of an error
func (rec *IniRecord) LoadBool(out *bool) error {
	return rec.LoadNamedBool(out, "false", "true")
}

// LoadNamedBool loads boolean value
// Names for "true" and "false" values are specified explicitly
// The destination remains untouched in a case of an error
func (rec *IniRecord) LoadNamedBool(out *bool, vFalse, vTrue string) error {
	switch rec.Value {
	case vFalse:
		*out = false
		return nil
	case vTrue:
		*out = true
		return nil
	default:
		return rec.errBadValue("must be %s or %s", vFalse, vTrue)
	}
}

// LoadLogLevel loads LogLevel value
// The destination remains untouched in a case of an error
func (rec *IniRecord) LoadLogLevel(out *LogLevel) error {
	var mask LogLevel

	for _, s := range strings.Split(rec.Value, ",") {
		s = strings.TrimSpace(s)
		switch s {
		case "":
		case "error":
			mask |= LogError
		case "info":
			mask |= LogInfo | LogError
		case "debug":
			mask |= LogDebug | LogInfo | LogError
		case "trace-ipp":
			mask |= LogTraceIPP | LogDebug | LogInfo | LogError
		case "trace-escl":
			mask |= LogTraceESCL | LogDebug | LogInfo | LogError
		case "trace-http":
			mask |= LogTraceHTTP | LogDebug | LogInfo | LogError
		case "trace-usb":
			mask |= LogTraceUSB | LogDebug | LogInfo | LogError
		case "all", "trace-all":
			mask |= LogAll & ^LogTraceUSB
		default:
			return rec.errBadValue("invalid log level %q", s)
		}
	}

	*out = mask
	return nil
}

// LoadDuration loads time.Duration value
// The destination remains untouched in a case of an error
func (rec *IniRecord) LoadDuration(out *time.Duration) error {
	var ms uint
	err := rec.LoadUint(&ms)
	if err == nil {
		*out = time.Millisecond * time.Duration(ms)
	}
	return err
}

// LoadSize loads size value (returned as int64)
// The syntax is following:
//   123  - size in bytes
//   123K - size in kilobytes, 1K == 1024
//   123M - size in megabytes, 1M == 1024K
// The destination remains untouched in a case of an error
func (rec *IniRecord) LoadSize(out *int64) error {
	var units uint64 = 1

	if l := len(rec.Value); l > 0 {
		switch rec.Value[l-1] {
		case 'k', 'K':
			units = 1024
		case 'm', 'M':
			units = 1024 * 1024
		}

		if units != 1 {
			rec.Value = rec.Value[:l-1]
		}
	}

	sz, err := strconv.ParseUint(rec.Value, 10, 64)
	if err != nil {
		return rec.errBadValue("%q: invalid size", rec.Value)
	}

	if sz > uint64(math.MaxInt64/units) {
		return rec.errBadValue("size too large")
	}

	*out = int64(sz * units)
	return nil
}

// LoadUint loads unsigned integer value
// The destination remains untouched in a case of an error
func (rec *IniRecord) LoadUint(out *uint) error {
	num, err := strconv.ParseUint(rec.Value, 10, 0)
	if err != nil {
		return rec.errBadValue("%q: invalid number", rec.Value)
	}

	*out = uint(num)
	return nil
}

// LoadUintRange loads unsigned integer value within the range
// The destination remains untouched in a case of an error
func (rec *IniRecord) LoadUintRange(out *uint, min, max uint) error {
	var val uint

	err := rec.LoadUint(&val)
	if err == nil && (val < min || val > max) {
		err = rec.errBadValue("must be in range %d...%d", min, max)
	}

	if err != nil {
		return err
	}

	*out = val
	return nil
}

// LoadQuirksResetMethod loads QuirksResetMethod value
// The destination remains untouched in a case of an error
func (rec *IniRecord) LoadQuirksResetMethod(out *QuirksResetMethod) error {
	switch rec.Value {
	case "none":
		*out = QuirksResetNone
		return nil
	case "soft":
		*out = QuirksResetSoft
		return nil
	case "hard":
		*out = QuirksResetHard
		return nil
	default:
		return rec.errBadValue("must be none, soft or hard")
	}
}

// errBadValue creates a "bad value" error related to the INI record
func (rec *IniRecord) errBadValue(format string, args ...interface{}) error {
	return fmt.Errorf(rec.Key+": "+format, args...)
}

// Error implements error interface for the IniError
func (err *IniError) Error() string {
	return fmt.Sprintf("%s:%d: %s", err.File, err.Line, err.Message)
}
