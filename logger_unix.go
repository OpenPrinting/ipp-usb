//go:build darwin || dragonfly || freebsd || linux || nacl || netbsd || openbsd || solaris
// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris

/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Logging, system-dependent part for UNIX
 */

package main

import (
	"io"
	"os"
)

// #include <unistd.h>
import "C"

// logIsAtty returns true, if os.File refers to a terminal
func logIsAtty(file *os.File) bool {
	fd := file.Fd()
	return C.isatty(C.int(fd)) == 1
}

// logColorConsoleWrite writes a colorized line to console
func logColorConsoleWrite(out io.Writer, level LogLevel, line []byte) {
	var beg, end string

	switch {
	case (level & LogError) != 0:
		beg, end = "\033[31;1m", "\033[0m" // Red
	case (level & LogInfo) != 0:
		beg, end = "\033[32;1m", "\033[0m" // Green
	case (level & LogDebug) != 0:
		beg, end = "\033[37;1m", "\033[0m" // White
	case (level & LogTraceAll) != 0:
		beg, end = "\033[37m", "\033[0m" // Gray
	}

	out.Write([]byte(beg))
	out.Write(line)
	out.Write([]byte(end))
}
