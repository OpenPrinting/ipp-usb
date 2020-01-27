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
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
)

var (
	httpSessionId int32
)

// Type HttpProxy represents HTTP protocol proxy backed by
// a specified http.RoundTripper. It implements http.Handler
// interface
type HttpProxy struct {
	log       *Logger           // Logger instance
	server    *http.Server      // HTTP server
	transport http.RoundTripper // Transport for outgoing requests
	closeWait chan struct{}     // Closed at server close
}

// Create new HTTP proxy
func NewHttpProxy(logger *Logger,
	listener net.Listener, transport http.RoundTripper) *HttpProxy {

	proxy := &HttpProxy{
		log:       logger,
		transport: transport,
		closeWait: make(chan struct{}),
	}

	proxy.server = &http.Server{
		Handler:  proxy,
		ErrorLog: log.New(logger.LineWriter(LogError, '!'), "", 0),
	}

	go func() {
		proxy.server.Serve(listener)
		close(proxy.closeWait)
	}()

	return proxy
}

// Close the proxy
func (proxy *HttpProxy) Close() {
	proxy.server.Close()
	<-proxy.closeWait
}

// Handle HTTP request
func (proxy *HttpProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	session := int(atomic.AddInt32(&httpSessionId, 1) - 1)
	defer atomic.AddInt32(&httpSessionId, -1)

	proxy.log.HttpRqParams(LogDebug, '>', session, r)
	proxy.log.HttpHdr(LogTraceHttp, '>', session, r.Header)

	// Perform sanity checking
	if r.Method == "CONNECT" {
		proxy.httpError(session, w, r, http.StatusMethodNotAllowed,
			"CONNECT not allowed")
		return
	}

	if r.Header.Get("Upgrade") != "" {
		proxy.httpError(session, w, r, http.StatusServiceUnavailable,
			"Protocol upgrade is not implemented")
		return
	}

	if r.URL.IsAbs() {
		proxy.httpError(session, w, r, http.StatusServiceUnavailable,
			"Absolute URL not allowed")
		return
	}

	// Adjust request headers
	httpRemoveHopByHopHeaders(r.Header)
	//r.Header.Add("Connection", "close")

	if r.Host == "" {
		// It's a pure black magic how to obtain Host if
		// it is missed in request (i.e., it's HTTP/1.0)
		v := r.Context().Value(http.LocalAddrContextKey)
		if v != nil {
			if addr, ok := v.(net.Addr); ok {
				r.Host = addr.String()
			}
		}
	}

	r.URL.Scheme = "http"
	r.URL.Host = r.Host

	// Serve the request
	resp, err := proxy.transport.RoundTrip(r)
	if err != nil {
		proxy.httpError(session, w, r, http.StatusServiceUnavailable,
			err.Error())
		return
	}

	httpRemoveHopByHopHeaders(resp.Header)
	httpCopyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	resp.Body.Close()

	proxy.log.HttpRspStatus(LogDebug, '<', session, resp)
	proxy.log.HttpHdr(LogTraceHttp, '<', session, resp.Header)
}

// Reject request with a error
func (proxy *HttpProxy) httpError(session int, w http.ResponseWriter, r *http.Request,
	status int, format string, args ...interface{}) {

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	httpNoCache(w)
	w.WriteHeader(status)

	msg := fmt.Sprintf(format, args...)
	msg += "\n"

	w.Write([]byte(msg))
	w.Write([]byte("\n"))

	proxy.log.HttpError('!', session, status, msg)
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
