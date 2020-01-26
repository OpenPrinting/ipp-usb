/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Logging
 */

package main

import (
	"fmt"
	"net/http"
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
