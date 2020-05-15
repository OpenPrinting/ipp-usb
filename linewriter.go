/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * LineWriter is a helper object, implementing io.Writer interface
 * on a top of write-line callback. It is used by logger.
 */

package main

import (
	"bytes"
)

// LineWriter implements io.Write and io.Close interfaces
// It splits stream into text lines and calls a provided
// callback for each complete line.
//
// Line passed to callback is not terminated by '\n'
// character. Close flushes last incomplete line, if any
type LineWriter struct {
	Func func([]byte) // write-line callback
	buf  bytes.Buffer // buffer for incomplete lines
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
			line = text[:l-1]
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
			lw.Func(line)
			lw.buf.Reset()
		}
	}

	return
}

// Close implements io.Closer interface
//
// Close flushes the last incomplete line from the
// internal buffer. Close is not needed, if it is
// known that there is no such a line, or if its
// presence doesn't matter (without Close its content
// will be lost)
func (lw *LineWriter) Close() error {
	if lw.buf.Len() > 0 {
		lw.Func(lw.buf.Bytes())
	}
	return nil
}

// WriteClose writes text to LineWriter and then closes it
func (lw *LineWriter) WriteClose(text []byte) {
	lw.Write(text)
	lw.Close()
}
