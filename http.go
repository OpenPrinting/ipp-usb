/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * HTTP proxy
 */

package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
)

var (
	httpSessionID int32
)

// HTTPProxy represents HTTP protocol proxy backed by the
// specified http.RoundTripper. It implements http.Handler
// interface
type HTTPProxy struct {
	log       *Logger       // Logger instance
	server    *http.Server  // HTTP server
	transport *UsbTransport // Transport for outgoing requests
	closeWait chan struct{} // Closed at server close
}

// NewHTTPProxy creates new HTTP proxy
func NewHTTPProxy(logger *Logger,
	listener net.Listener, transport *UsbTransport) *HTTPProxy {

	proxy := &HTTPProxy{
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
func (proxy *HTTPProxy) Close() {
	proxy.server.Close()
	<-proxy.closeWait
}

// Handle HTTP request
func (proxy *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Catch panics to log
	defer func() {
		v := recover()
		if v != nil {
			Log.Panic(v)
		}
	}()

	session := int(atomic.AddInt32(&httpSessionID, 1)-1) % 1000

	proxy.log.Begin().
		HTTPRqParams(LogDebug, '>', session, r).
		HTTPHdr(LogTraceHTTP, '>', session, r.Header).
		Commit()

	// Perform sanity checking
	if r.Method == "CONNECT" {
		proxy.httpError(session, w, r, http.StatusMethodNotAllowed,
			errors.New("CONNECT not allowed"))
		return
	}

	if r.Header.Get("Upgrade") != "" {
		proxy.httpError(session, w, r, http.StatusServiceUnavailable,
			errors.New("Protocol upgrade is not implemented"))
		return
	}

	if r.URL.IsAbs() {
		proxy.httpError(session, w, r, http.StatusServiceUnavailable,
			errors.New("Absolute URL not allowed"))
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

	// Send request and obtain response status and header
	resp, err := proxy.transport.RoundTripSession(session, r)
	if err != nil {
		proxy.httpError(session, w, r, http.StatusServiceUnavailable, err)
		return
	}

	httpRemoveHopByHopHeaders(resp.Header)
	httpCopyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	proxy.log.Begin().
		HTTPRspStatus(LogDebug, '<', session, resp).
		HTTPHdr(LogTraceHTTP, '<', session, resp.Header).
		Commit()

	// Obtain response body, if any
	_, err = io.Copy(w, resp.Body)

	if err != nil {
		proxy.log.HTTPError('!', session, "%s", err)
	}

	resp.Body.Close()

}

// Reject request with a error
func (proxy *HTTPProxy) httpError(session int, w http.ResponseWriter, r *http.Request,
	status int, err error) {

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	httpNoCache(w)
	w.WriteHeader(status)

	w.Write([]byte(err.Error()))
	w.Write([]byte("\n"))

	if err != context.Canceled {
		proxy.log.HTTPError('!', session, "%s", err.Error())
	} else {
		proxy.log.HTTPDebug(' ', session, "request canceled by impatient client")
	}
}

// HTTPLoggingRoundTripper wraps UsbTransport, adding logging
// for each request. It implements http.RoundTripper interface
type HTTPLoggingRoundTripper struct {
	Log       *Logger       // Logger to write logs to
	transport *UsbTransport // Underlying UsbTransport
}

// RoundTrip executes a single HTTP transaction, returning
// a Response for the provided Request.
func (rtp *HTTPLoggingRoundTripper) RoundTrip(r *http.Request) (
	*http.Response, error) {
	session := int(atomic.AddInt32(&httpSessionID, 1)-1) % 1000

	rtp.Log.HTTPRqParams(LogDebug, '>', session, r)
	rtp.Log.HTTPHdr(LogTraceHTTP, '>', session, r.Header)

	resp, err := rtp.transport.RoundTripSession(session, r)
	if err == nil {
		rtp.Log.HTTPRspStatus(LogDebug, '<', session, resp)
		rtp.Log.HTTPHdr(LogTraceHTTP, '<', session, resp.Header)
	} else {
		rtp.Log.HTTPError('!', session, "%s", err)
	}

	return resp, err
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
