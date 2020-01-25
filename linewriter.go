/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * LineWriter is a helper object, implementing io.Writer interface
 * on a top of write-line callback
 */

package main

import (
	"bytes"
)

// LineWriter implements io.Write and io.Close interfaces
// It splits stream into text lines and calls a proviced
// callback for each complete line.
//
// Line passed to callback is always terminated by '\n'
// character. Close flushes last incomplete line, if any
type LineWriter struct {
	Callback func([]byte)
	buf      bytes.Buffer
}

// Write implements io.Writer interface
func (lw *LineWriter) Write(text []byte) (n int, err error) {
	n = len(text)

	for len(text) > 0 {
		// Fetch next line
		var line []byte
		var unfinished bool

		if l := bytes.IndexByte(text, '\n'); l >= 0 {
			l++
			line = text[:l]
			text = text[l:]
		} else {
			line = text
			text = nil
			unfinished = true
		}

		// Dispatch next line
		if unfinished || lw.buf.Len() > 0 {
			lw.buf.Write(line)
			line = lw.buf.Bytes()
		}

		if !unfinished {
			lw.Callback(line)
			lw.buf.Reset()
		}
	}

	return
}

// Write implements io.Closer interface
func (lw *LineWriter) Close() error {
	if lw.buf.Len() > 0 {
		lw.buf.WriteByte('\n')
		lw.Callback(lw.buf.Bytes())
	}
	return nil
}
