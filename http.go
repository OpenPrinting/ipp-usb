/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * HTTP proxy
 */

package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

var (
	httpSessionId int32
)

// Type httpProxy represents HTTP protocol proxy backed by
// a specified http.RoundTripper. It implements http.Handler
// interface
type httpProxy struct {
	transport http.RoundTripper // Transport for outgoing requests
	host      string            // Host: header in outgoing requests
}

// Handle HTTP request
func (proxy *httpProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	session := atomic.AddInt32(&httpSessionId, 1) - 1
	defer atomic.AddInt32(&httpSessionId, -1)

	log_http_rq(session, r)

	// Perform sanity checking
	if r.Method == "CONNECT" {
		httpError(session, w, r, http.StatusMethodNotAllowed,
			"CONNECT not allowed")
		return
	}

	if r.Header.Get("Upgrade") != "" {
		httpError(session, w, r, http.StatusServiceUnavailable,
			"Protocol upgrade is not implemented")
		return
	}

	if r.URL.IsAbs() {
		httpError(session, w, r, http.StatusServiceUnavailable,
			"Absolute URL not allowed")
		return
	}

	// Adjust request headers
	httpRemoveHopByHopHeaders(r.Header)
	//r.Header.Add("Connection", "close")

	r.URL.Scheme = "http"
	r.URL.Host = proxy.host

	// Serve the request
	resp, err := proxy.transport.RoundTrip(r)
	if err != nil {
		httpError(session, w, r, http.StatusServiceUnavailable,
			err.Error())
		return
	}

	httpRemoveHopByHopHeaders(resp.Header)
	httpCopyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	resp.Body.Close()

	log_http_rsp(session, resp)
}

// Reject request with a error
func httpError(session int32, w http.ResponseWriter, r *http.Request,
	status int, format string, args ...interface{}) {

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	httpNoCache(w)
	w.WriteHeader(status)

	msg := fmt.Sprintf(format, args...)
	msg += "\n"

	w.Write([]byte(msg))
	w.Write([]byte("\n"))

	log_http_err(session, status, msg)
}

// Set response headers to disable cacheing
func httpNoCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

// Remove HTTP hop-by-hop headers, RFC 7230, section 6.1
func httpRemoveHopByHopHeaders(hdr http.Header) {
	if c := hdr.Get("Connection"); c != "" {
		for _, f := range strings.Split(c, ",") {
			if f = strings.TrimSpace(f); f != "" {
				hdr.Del(f)
			}
		}
	}

	for _, c := range []string{"Connection", "Keep-Alive",
		"Proxy-Authenticate", "Proxy-Connection",
		"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding"} {
		hdr.Del(c)
	}
}

// Copy HTTP headers
func httpCopyHeaders(dst, src http.Header) {
	for k, v := range src {
		dst[k] = v
	}
}

// Close connection when timer expires
func httpTimeGoroutine(tmr *time.Timer, c <-chan struct{}, src io.ReadCloser) {
	for {
		select {
		case <-c:
			return
		case <-tmr.C:
			src.Close()
			log_debug("! killing idle connection")
			return
		}
	}
}

// Run HTTP server at given address
func HttpListenAndServe(addr string, transport http.RoundTripper) error {
	proxy := &httpProxy{
		transport: transport,
		host:      addr,
	}
	log_debug("Starting HTTP server at http://%s", addr)
	return http.ListenAndServe(addr, proxy)
}
