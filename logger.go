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

	"github.com/alexpevzner/goipp"
)

const (
	LogMaxFileSize    = 100 * 1025
	LogMaxBackupFiles = 5
)

var (
	logMessagePool = sync.Pool{New: func() interface{} { return &LogMessage{} }}
)

// LogLevel enumerates possible log levels
type LogLevel int

const (
	LogError LogLevel = 1 << iota
	LogInfo
	LogDebug
	LogTraceIpp
	LogTraceEscl
	LogTraceHttp

	LogAll      = LogError | LogInfo | LogDebug | LogTraceAll
	LogTraceAll = LogTraceIpp | LogTraceEscl | LogTraceHttp
)

// Logger implements logging facilities
type Logger struct {
	lock    sync.Mutex // Write lock
	path    string     // Path to log file
	out     io.Writer  // Output stream, may be *os.File
	console bool       // true for console logger
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
		out:     os.Stdout,
		console: true,
	}
}

// Close the logger
func (l *Logger) Close() {
	if !l.console {
		if file, ok := l.out.(*os.File); ok {
			file.Close()
		}
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

// LineWriter creates a LineWriter that writes to the Logger,
// using specified LogLevel and prefix
func (l *Logger) LineWriter(level LogLevel, prefix byte) *LineWriter {
	return &LineWriter{
		Func: func(line []byte) {
			l.Begin().addBytes(level, prefix, line).Commit()
		},
	}
}

// Format a time prefix
func (l *Logger) fmtTime() *logLineBuf {
	buf := logLineBufAlloc(0)

	if !l.console {
		now := time.Now()

		year, month, day := now.Date()
		fmt.Fprintf(buf, "%2.2d-%2.2d-%4.4d ", day, month, year)

		hour, min, sec := now.Clock()
		fmt.Fprintf(buf, "%2.2d:%2.2d:%2.2d", hour, min, sec)

		buf.WriteByte(':')
	}

	return buf
}

// Handle log rotation
func (l *Logger) rotate() {
	// Do we need to rotate?
	file, ok := l.out.(*os.File)
	if !ok {
		return
	}

	stat, err := file.Stat()
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
				file.Truncate(0)
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
	logger *Logger       // Underlying logger
	lines  []*logLineBuf // One buffer per line
}

// Add formats a next line of log message, with level and prefix char
func (msg *LogMessage) Add(level LogLevel, prefix byte,
	format string, args ...interface{}) *LogMessage {

	buf := logLineBufAlloc(level)
	buf.Write([]byte{prefix, ' '})
	fmt.Fprintf(buf, format, args...)
	msg.lines = append(msg.lines, buf)

	return msg
}

// addBytes adds a next line of log message, taking slice of bytes as input
func (msg *LogMessage) addBytes(level LogLevel, prefix byte, line []byte) *LogMessage {
	buf := logLineBufAlloc(level)
	if len(line) > 0 {
		buf.Write([]byte{prefix, ' '})
		buf.Write(line)
	}
	msg.lines = append(msg.lines, buf)

	return msg
}

// Debug appends a LogDebug line to the message
func (msg *LogMessage) Debug(prefix byte, format string, args ...interface{}) *LogMessage {
	return msg.Add(LogDebug, prefix, format, args...)
}

// Info appends a LogInfo line to the message
func (msg *LogMessage) Info(format string, args ...interface{}) *LogMessage {
	return msg.Add(LogInfo, ' ', format, args...)
}

// Error appends a LogError line to the message
func (msg *LogMessage) Error(format string, args ...interface{}) *LogMessage {
	return msg.Add(LogError, '!', format, args...)
}

// Dump appends a HEX dump to the log message
func (msg *LogMessage) HexDump(level LogLevel, data []byte) *LogMessage {
	hex := logLineBufAlloc(level)
	chr := logLineBufAlloc(level)

	defer hex.free()
	defer chr.free()

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

		msg.Add(level, ' ', "%4.4x: %s %s", off, hex, chr)

		off += sz
		data = data[sz:]
	}

	return msg
}

// IppRequest dumps IPP request into the log message
func (msg *LogMessage) IppRequest(level LogLevel, m *goipp.Message) {
	m.Print(msg.LineWriter(level, ' '), true)
}

// IppResponse dumps IPP response into the log message
func (msg *LogMessage) IppResponse(level LogLevel, m *goipp.Message) {
	m.Print(msg.LineWriter(level, ' '), false)
}

// LineWriter creates a LineWriter that writes to the LogMessage,
// using specified LogLevel and prefix
func (msg *LogMessage) LineWriter(level LogLevel, prefix byte) *LineWriter {
	return &LineWriter{
		Func: func(line []byte) { msg.addBytes(level, prefix, line) },
	}
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
	if msg.logger.out == nil && !msg.logger.console {
		os.MkdirAll(PathLogDir, 0755)
		msg.logger.out, _ = os.OpenFile(msg.logger.path,
			os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	}

	if msg.logger.out == nil {
		return
	}

	// Rotate now
	msg.logger.rotate()

	// Send message content to the logger
	buf := msg.logger.fmtTime()
	defer buf.free()

	buflen := buf.Len()
	for _, l := range msg.lines {
		buf.Truncate(buflen)
		if l.Len() > 0 {
			if buflen > 0 {
				buf.WriteByte(' ')
			}
			buf.Write(l.Bytes())
		}
		buf.WriteByte('\n')

		msg.logger.out.Write(buf.Bytes())
	}
}

// Reject the message
func (msg *LogMessage) Reject() {
	msg.free()
}

// Return message to the logMessagePool
func (msg *LogMessage) free() {
	// Free all lines
	for _, l := range msg.lines {
		l.free()
	}

	// Reset the message and put it to the pool
	if len(msg.lines) < 16 {
		msg.lines = msg.lines[:0] // Keep memory, reset content
	} else {
		msg.lines = nil // Drop this large buffer
	}

	msg.logger = nil

	// Put the message
	logMessagePool.Put(msg)
}

// logLineBuf represents a single log line buffer
type logLineBuf struct {
	bytes.Buffer          // Underlying buffer
	level        LogLevel // Log level the line was written on
}

// logLinePool manages a pool of reusable logLines
var logLineBufPool = sync.Pool{New: func() interface{} {
	return &logLineBuf{
		Buffer: bytes.Buffer{},
	}
}}

// logLineAlloc() allocates a logLineBuf
func logLineBufAlloc(level LogLevel) *logLineBuf {
	buf := logLineBufPool.Get().(*logLineBuf)
	buf.level = level
	return buf
}

// free returns the logLineBuf to the pool
func (buf *logLineBuf) free() {
	if buf.Cap() <= 256 {
		buf.Reset()
		logLineBufPool.Put(buf)
	}
}
