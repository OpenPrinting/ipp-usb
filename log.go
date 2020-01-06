// Logging

package main

import (
	"bytes"
	"fmt"
	"os"
)

// Print debug message
func log_debug(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...) + "\n"
	print(s)
}

// Print log message and exit
func log_exit(format string, args ...interface{}) {
	log_debug(format, args...)
	os.Exit(1)
}

// If error is not nil, print error message and exit
func log_check(err error) {
	if err != nil {
		log_exit(err.Error())
	}
}

// Print usage error and exit
func log_usage(format string, args ...interface{}) {
	if format != "" {
		log_debug(format, args...)
	}

	log_debug("Try %s -h for more information", os.Args[0])
	os.Exit(1)
}

// Print hex dump
func log_dump(data []byte) {
	hex := new(bytes.Buffer)
	chr := new(bytes.Buffer)

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
			fmt.Fprintf(hex, "%2.2x ", data[i])
			if 0x20 <= c && c < 0x80 {
				chr.WriteByte(c)
			} else {
				chr.WriteByte('.')
			}
		}

		for ; i < 16; i++ {
			hex.WriteString("   ")
		}

		log_debug("%s %s", hex, chr)

		data = data[sz:]
	}
}
