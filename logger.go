/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Logging
 */

package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	LogMaxFileSize    = 100 * 1025
	LogMaxBackupFiles = 5
)

var (
	logMessagePool = sync.Pool{New: func() interface{} { return &LogMessage{} }}
	logBufferPool  = sync.Pool{New: func() interface{} { return &bytes.Buffer{} }}
)

// LogLevel enumerates possible log levels
type LogLevel int

const (
	LogError LogLevel = iota
	LogInfo
	LogDebug
	LogTrace
)

// Logger implements logging facilities
type Logger struct {
	lock    sync.Mutex   // Write lock
	path    string       // Path to log file
	time    bytes.Buffer // Time prefix buffer
	file    *os.File     // Output file
	console bool         // true for console logger
}

// Create new device logger
func NewDeviceLogger(info UsbDeviceInfo) *Logger {
	return &Logger{
		path: filepath.Join(PathLogDir, info.Ident()+".log"),
	}
}

// Create new console logger
func NewConsoleLogger() *Logger {
	return &Logger{
		file:    os.Stdout,
		console: true,
	}
}

// Close the logger
func (l *Logger) Close() {
	if !l.console {
		l.file.Close()
	}
}

// Begin new log message
func (l *Logger) Begin() *LogMessage {
	msg := logMessagePool.Get().(*LogMessage)
	msg.logger = l
	return msg
}

// Debug writes a LogDebug message
func (l *Logger) Debug(prefix byte, format string, args ...interface{}) {
	l.Begin().Debug(prefix, format, args...).Commit()
}

// Info writes a LogInfo message
func (l *Logger) Info(format string, args ...interface{}) {
	l.Begin().Info(format, args...).Commit()
}

// Error writes a LogError message
func (l *Logger) Error(format string, args ...interface{}) {
	l.Begin().Error(format, args...).Commit()
}

// Write HEX dump with optional title. If title is not "", it is formatted,
// as fmt.Printf does, and prepended to the dump
func (l *Logger) Dump(data []byte, title string, args ...interface{}) {
	l.Begin().Dump(data, title, args...).Commit()
}

// Format a time prefix
func (l *Logger) fmtTime() {
	if !l.console {
		l.time.Reset()

		now := time.Now()

		year, month, day := now.Date()
		fmt.Fprintf(&l.time, "%2.2d-%2.2d-%4.4d ", day, month, year)

		hour, min, sec := now.Clock()
		fmt.Fprintf(&l.time, "%2.2d:%2.2d:%2.2d", hour, min, sec)

		l.time.WriteString(": ")
	}
}

// Handle log rotation
func (l *Logger) rotate() {
	// Do we need to rotate?
	stat, err := l.file.Stat()
	if err != nil || stat.Size() <= LogMaxFileSize {
		return
	}

	// Perform rotation
	prevpath := ""
	for i := LogMaxBackupFiles; i >= 0; i-- {
		nextpath := l.path
		if i > 0 {
			nextpath += fmt.Sprintf(".%d.gz", i-1)
		}

		switch i {
		case LogMaxBackupFiles:
			os.Remove(nextpath)
		case 0:
			err := l.gzip(nextpath, prevpath)
			if err == nil {
				l.file.Truncate(0)
			}
		default:
			os.Rename(nextpath, prevpath)
		}

		prevpath = nextpath
	}

}

// gzip the log file
func (l *Logger) gzip(ipath, opath string) error {
	// Open input file
	ifile, err := os.Open(ipath)
	if err != nil {
		return err
	}

	defer ifile.Close()

	// Open output file
	ofile, err := os.OpenFile(opath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	// gzip ifile->ofile
	w := gzip.NewWriter(ofile)
	_, err = io.Copy(w, ifile)
	err2 := w.Close()
	err3 := ofile.Close()

	switch {
	case err == nil && err2 != nil:
		err = err2
	case err == nil && err3 != nil:
		err = err3
	}

	// Cleanup and exit
	if err != nil {
		os.Remove(opath)
	}

	return err
}

// LogMessage represents a single (possible multi line) log
// message, which will appear in the output log atomically,
// and will be interrupted in the middle by other log activity
type LogMessage struct {
	logger *Logger         // Underlying logger
	lines  []*bytes.Buffer // One buffer per line
}

// add formats a next line of log message, with level and prefix char
func (msg *LogMessage) add(level LogLevel, prefix byte,
	format string, args ...interface{}) *LogMessage {

	buf := logBufAlloc()
	buf.Write([]byte{prefix, ' '})
	fmt.Fprintf(buf, format, args...)
	buf.WriteByte('\n')
	msg.lines = append(msg.lines, buf)
	return msg
}

// Debug writes a LogDebug message
func (msg *LogMessage) Debug(prefix byte, format string, args ...interface{}) *LogMessage {
	return msg.add(LogDebug, prefix, format, args...)
}

// Info writes a LogInfo message
func (msg *LogMessage) Info(format string, args ...interface{}) *LogMessage {
	return msg.add(LogInfo, ' ', format, args...)
}

// Error writes a LogError message
func (msg *LogMessage) Error(format string, args ...interface{}) *LogMessage {
	return msg.add(LogError, '!', format, args...)
}

// Write implements io.Writer interface. Text is automatically
// split into lines
func (msg *LogMessage) Write(text []byte) (n int, err error) {
	n, err = len(text), nil

	for len(text) > 0 {
		// Fetch next line
		var line []byte

		if l := bytes.IndexByte(text, '\n'); l >= 0 {
			l++
			line = text[:l]
			text = text[l:]
		} else {
			line = text
			text = nil
		}

		// Save the line
		if cnt := len(msg.lines); cnt > 0 && !logBufTerminated(msg.lines[cnt-1]) {
			buf := msg.lines[cnt-1]
			if buf.Len() == 0 {
				buf.Write([]byte("  "))
			}
			buf.Write(line)
		} else {
			buf := logBufAlloc()
			if len(line) != 0 {
				buf.Write([]byte("  "))
				buf.Write(line)
			}
			msg.lines = append(msg.lines, buf)
		}
	}

	return
}

// Write HEX dump with optional title. If title is not "", it is formatted,
// as fmt.Printf does, and prepended to the dump
func (msg *LogMessage) Dump(data []byte, title string, args ...interface{}) *LogMessage {
	if title != "" {
		msg.Debug(' ', title, args...)
	}

	hex := logBufAlloc()
	chr := logBufAlloc()

	defer logBufFree(hex)
	defer logBufFree(chr)

	off := 0

	for len(data) > 0 {
		hex.Reset()
		chr.Reset()

		sz := len(data)
		if sz > 16 {
			sz = 16
		}

		i := 0
		for ; i < sz; i++ {
			c := data[i]
			fmt.Fprintf(hex, "%2.2x", data[i])
			if i%4 == 3 {
				hex.Write([]byte(":"))
			} else {
				hex.Write([]byte(" "))
			}

			if 0x20 <= c && c < 0x80 {
				chr.WriteByte(c)
			} else {
				chr.WriteByte('.')
			}
		}

		for ; i < 16; i++ {
			hex.WriteString("   ")
		}

		msg.Debug(' ', "%4.4x: %s %s", off, hex, chr)

		off += sz
		data = data[sz:]
	}

	return msg
}

// Commit message to the log
func (msg *LogMessage) Commit() {
	// Don't forget to free the message
	defer msg.free()

	// Ignore empty messages
	if len(msg.lines) == 0 {
		return
	}

	// Lock the logger
	msg.logger.lock.Lock()
	defer msg.logger.lock.Unlock()

	// Open log file on demand
	if msg.logger.file == nil && !msg.logger.console {
		os.MkdirAll(PathLogDir, 0755)
		msg.logger.file, _ = os.OpenFile(msg.logger.path,
			os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	}

	if msg.logger.file == nil {
		return
	}

	// Rotate now
	msg.logger.rotate()

	// Send message content to the logger
	msg.logger.fmtTime()
	for _, l := range msg.lines {
		if !logBufTerminated(l) {
			l.WriteByte('\n')
		}
		msg.logger.file.Write(msg.logger.time.Bytes())
		msg.logger.file.Write(l.Bytes())
	}
}

// Reject the message
func (msg *LogMessage) Reject() {
	msg.free()
}

// Return message to the logMessagePool
func (msg *LogMessage) free() {
	for _, l := range msg.lines {
		logBufFree(l)
	}

	// Reset the message and put it to the pool
	if len(msg.lines) < 16 {
		msg.lines = msg.lines[:0] // Keep memory, reset content
	} else {
		msg.lines = nil
	}

	msg.logger = nil

	// Put the message
	logMessagePool.Put(msg)
}

// Check if line buffer is '\n'-terminated
func logBufTerminated(buf *bytes.Buffer) bool {
	if l := buf.Len(); l > 0 {
		return buf.Bytes()[l-1] == '\n'
	}
	return false
}

// Allocate a buffer
func logBufAlloc() *bytes.Buffer {
	return logBufferPool.Get().(*bytes.Buffer)
}

// Free a buffer
func logBufFree(buf *bytes.Buffer) {
	if buf.Cap() <= 256 {
		buf.Reset()
		logBufferPool.Put(buf)
	}
}
