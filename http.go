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
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
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

	// Obtain our local address the request was ordered to
	var localAddr *net.TCPAddr

	if v := r.Context().Value(http.LocalAddrContextKey); v != nil {
		if v != nil {
			localAddr, _ = v.(*net.TCPAddr)
		}
	}

	if localAddr == nil {
		proxy.httpError(session, w, r, http.StatusInternalServerError,
			errors.New("Unable to get local address for request"))
		return
	}

	// Adjust request headers
	httpRemoveHopByHopHeaders(r.Header)

	if r.Host == "" {
		if localAddr.IP.IsLoopback() {
			r.Host = fmt.Sprintf("localhost:%d", localAddr.Port)
		} else {
			r.Host = localAddr.String()
		}
	}

	r.URL.Scheme = "http"
	r.URL.Host = r.Host

	// If request is ordered to the loopback address, and r.Host is not
	// "localhost" or "localhost:port", redirect request to the localhost
	//
	// Note, IPP over USB specification requires Host: to be always
	// "localhost" or "localhost:port". Although most of the printers
	// accept any syntactically correct Host: header, some of the OKI
	// printers doesn't, and reject requests that violate this rule
	//
	// This redirection fixes compatibility with these printers for
	// clients that follow redirects (i.e., web browser and sane-airscan;
	// CUPS unfortunately doesn't follow redirects)
	if localAddr.IP.IsLoopback() &&
		!strings.HasPrefix(strings.ToLower(r.Host), "localhost:") &&
		r.Method == "GET" || r.Method == "HEAD" {

		url := *r.URL
		url.Host = fmt.Sprintf("localhost:%d", localAddr.Port)

		proxy.httpRedirect(session, w, r, http.StatusFound, &url)
		return
	}

	// Send request and obtain response status and header
	resp, err := proxy.transport.RoundTripWithSession(session, r)
	if err != nil {
		proxy.httpError(session, w, r, http.StatusServiceUnavailable, err)
		return
	}

	httpRemoveHopByHopHeaders(resp.Header)
	httpCopyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

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

	proxy.log.Begin().
		HTTPRqParams(LogDebug, '>', session, r).
		HTTPRequest(LogTraceHTTP, '>', session, r).
		Commit()

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

// Respond to request with the HTTP redirect
func (proxy *HTTPProxy) httpRedirect(session int, w http.ResponseWriter, r *http.Request,
	status int, location *url.URL) {

	proxy.log.Begin().
		HTTPRqParams(LogDebug, '>', session, r).
		HTTPRequest(LogTraceHTTP, '>', session, r).
		Commit()

	w.Header().Set("Location", location.String())
	w.WriteHeader(status)

	proxy.log.HTTPDebug(' ', session, "redirected to %s", location)
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
