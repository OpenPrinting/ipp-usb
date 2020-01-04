// HTTP Proxy

package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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
	log_debug("< %s %s %s", r.Method, r.URL, r.Proto)

	// Perform sanity checking
	if r.Method == "CONNECT" {
		httpError(w, r, http.StatusMethodNotAllowed,
			"CONNECT not allowed")
		return
	}

	if r.Header.Get("Upgrade") != "" {
		httpError(w, r, http.StatusServiceUnavailable,
			"Protocol upgrade is not implemented")
		return
	}

	if r.URL.IsAbs() {
		httpError(w, r, http.StatusServiceUnavailable,
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
		httpError(w, r, http.StatusServiceUnavailable, err.Error())
		return
	}

	httpRemoveHopByHopHeaders(resp.Header)
	httpCopyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	resp.Body.Close()

	log_debug("> %s %s", resp.Proto, resp.Status)
}

// Reject request with a error
func httpError(w http.ResponseWriter, r *http.Request,
	status int, format string, args ...interface{}) {

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	httpNoCache(w)
	w.WriteHeader(status)

	msg := fmt.Sprintf(format, args...)
	msg += "\n"

	w.Write([]byte(msg))
	log_debug("> HTTP/1.1 %d %s", status, http.StatusText(status))
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
