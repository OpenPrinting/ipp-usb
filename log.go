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
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
)

var (
	logMultilineLock sync.Mutex
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
	logMultilineLock.Lock()
	defer logMultilineLock.Unlock()

	hex := new(bytes.Buffer)
	chr := new(bytes.Buffer)
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

		log_debug("%4.4x: %s %s", off, hex, chr)

		off += sz
		data = data[sz:]
	}
}

// Log HTTP header
func log_http_hdr(prefix, title string, hdr http.Header) {
	logMultilineLock.Lock()
	defer logMultilineLock.Unlock()

	keys := []string{}
	for k := range hdr {
		keys = append(keys, k)
	}

	log_debug("%s%s", prefix, title)
	sort.Strings(keys)
	for _, k := range keys {
		log_debug("%s%s: %s", prefix, k, hdr.Get(k))
	}

	log_debug("")
}

// Log HTTP request
func log_http_rq(session int32, rq *http.Request) {
	prefix := fmt.Sprintf("> HTTP[%d]: ", session)
	title := fmt.Sprintf("%s %s %s", rq.Method, rq.URL, rq.Proto)
	log_http_hdr(prefix, title, rq.Header)
}

// Log HTTP response
func log_http_rsp(session int32, rsp *http.Response) {
	prefix := fmt.Sprintf("< HTTP[%d]: ", session)
	title := fmt.Sprintf("%s %s", rsp.Proto, rsp.Status)
	log_http_hdr(prefix, title, rsp.Header)
}

// Log HTTP error
func log_http_err(session int32, status int, msg string) {
	prefix := fmt.Sprintf("! HTTP[%d]: ", session)
	log_debug("%sHTTP/1.1 %d %s", prefix, status, http.StatusText(status))
	log_debug("%s%s", prefix, msg)
}
