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
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	httpSessionID int32
)

const (
	esclScanJobsPrefix           = "/eSCL/ScanJobs/"
	esclScanJobsPath             = "/eSCL/ScanJobs"
	esclNextDocumentSuffix       = "/NextDocument"
	esclScannerCapabilitiesPath  = "/eSCL/ScannerCapabilities"
	esclScannerStatusPath        = "/eSCL/ScannerStatus"
	esclAutoReleasedJobMaxAge    = time.Hour
	esclAutoReleasedJobLogFormat = "eSCL job %s already auto-released"
)

type esclPendingJob struct {
	host string
	when time.Time
}

type esclReleasingJob struct {
	host string
	done chan struct{}
}

// HTTPProxy represents HTTP protocol proxy backed by the
// specified http.RoundTripper. It implements http.Handler
// interface
type HTTPProxy struct {
	log       *Logger       // Logger instance
	server    *http.Server  // HTTP server
	enable    bool          // Proxy can handle incoming requests
	transport *UsbTransport // Transport for outgoing requests
	closeWait chan struct{} // Closed at server close

	esclAutoReleasedJobs      map[string]time.Time
	esclAutoReleasedJobsMutex sync.Mutex
	esclExhaustedJobs         map[string]esclPendingJob
	esclReleasingJobs         map[string]esclReleasingJob
}

// NewHTTPProxy creates new HTTP proxy
func NewHTTPProxy(logger *Logger,
	listener net.Listener, transport *UsbTransport) *HTTPProxy {

	proxy := &HTTPProxy{
		log:                  logger,
		transport:            transport,
		closeWait:            make(chan struct{}),
		esclAutoReleasedJobs: make(map[string]time.Time),
		esclExhaustedJobs:    make(map[string]esclPendingJob),
		esclReleasingJobs:    make(map[string]esclReleasingJob),
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

// Enable indicates that initialization is completed and
// incoming requests can be handled
func (proxy *HTTPProxy) Enable() {
	proxy.enable = true
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
	if !proxy.enable {
		proxy.httpError(session, w, r, http.StatusServiceUnavailable,
			errors.New("ipp-usb is not ready for this device"))
		return
	}

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

	// Obtain request's client and server addresses
	var clientAddr, serverAddr *net.TCPAddr

	clientAddr, err := net.ResolveTCPAddr("tcp", r.RemoteAddr)
	if err != nil {
		proxy.httpError(session, w, r, http.StatusInternalServerError,
			errors.New("Unable to get client address for request"))
		return
	}

	if v := r.Context().Value(http.LocalAddrContextKey); v != nil {
		if v != nil {
			serverAddr, _ = v.(*net.TCPAddr)
		}
	}

	if serverAddr == nil {
		proxy.httpError(session, w, r, http.StatusInternalServerError,
			errors.New("Unable to get server address for request"))
		return
	}

	// Authenticate
	if status, err := AuthHTTPRequest(proxy.log,
		clientAddr, serverAddr, r); err != nil {
		proxy.httpError(session, w, r, status, err)
		return
	}

	// Adjust request headers
	httpRemoveHopByHopHeaders(r.Header)

	if r.Host == "" {
		if serverAddr.IP.IsLoopback() {
			r.Host = fmt.Sprintf("localhost:%d", serverAddr.Port)
		} else {
			r.Host = serverAddr.String()
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
	if serverAddr.IP.IsLoopback() &&
		(r.Method == "GET" || r.Method == "HEAD") {

		host := strings.ToLower(r.Host)
		if host != "localhost" &&
			!strings.HasPrefix(host, "localhost:") {

			url := *r.URL
			url.Host = fmt.Sprintf("localhost:%d", serverAddr.Port)

			proxy.httpRedirect(session, w, r, http.StatusFound, &url)
			return
		}
	}

	// If the escl-auto-release quirk has already released this scan job,
	// satisfy the client's later DELETE locally.
	if proxy.transport.Quirks().GetEsclAutoRelease() {
		if r.Method == "DELETE" {
			if proxy.takeAutoReleasedESCLJob(r.URL.Path) {
				proxy.httpLocalStatus(session, w, r, http.StatusOK,
					esclAutoReleasedJobLogFormat, r.URL.Path)
				return
			}

			proxy.waitForReleasingESCLJob(r.URL.Path)
			if proxy.takeAutoReleasedESCLJob(r.URL.Path) {
				proxy.httpLocalStatus(session, w, r, http.StatusOK,
					esclAutoReleasedJobLogFormat, r.URL.Path)
				return
			}

			proxy.forgetExhaustedESCLJob(r.URL.Path)
		}

		if esclNeedsSynchronousExhaustedJobRelease(r) {
			proxy.waitForReleasingESCLJobs()
			proxy.autoReleaseExhaustedESCLJobs(session)
		} else if !esclNeedsPostResponseExhaustedJobRelease(r) &&
			r.Method != "DELETE" {
			proxy.autoReleaseExhaustedESCLJobsAsync(session)
		}
	}

	// Send request and obtain response status and header
	resp, err := proxy.transport.RoundTripWithSession(session, r)
	if err != nil {
		proxy.httpError(session, w, r, http.StatusServiceUnavailable, err)
		return
	}

	// Some clients fail to DELETE eSCL scan jobs after NextDocument
	// returns 404. With the escl-auto-release quirk enabled, remember that
	// this job is exhausted and release it if the client asks for scanner
	// status or tries to start another scan without deleting it first.
	if proxy.transport.Quirks().GetEsclAutoRelease() &&
		resp.StatusCode == http.StatusNotFound {

		jobPath := esclJobPathFromNextDocumentPath(r.URL.Path)
		if r.Method == "GET" && jobPath != "" {
			proxy.rememberExhaustedESCLJob(jobPath, r.Host)
			proxy.log.HTTPDebug(' ', session,
				"eSCL auto-release: remembered exhausted job %s",
				jobPath)
		}
	}

	releaseExhaustedAfterResponse := proxy.transport.Quirks().GetEsclAutoRelease() &&
		esclNeedsPostResponseExhaustedJobRelease(r)

	httpRemoveHopByHopHeaders(resp.Header)
	httpCopyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	// Obtain response body, if any
	_, err = io.Copy(w, resp.Body)

	if err != nil {
		proxy.log.HTTPError('!', session, "%s", err)
	}

	resp.Body.Close()

	if releaseExhaustedAfterResponse {
		proxy.autoReleaseExhaustedESCLJobsAsync(session)
	}
}

// Respond to request locally with a status and no body.
func (proxy *HTTPProxy) httpLocalStatus(session int, w http.ResponseWriter,
	r *http.Request, status int, format string, args ...interface{}) {

	proxy.log.Begin().
		HTTPRqParams(LogDebug, '>', session, r).
		HTTPRequest(LogTraceHTTP, '>', session, r).
		Commit()

	w.WriteHeader(status)

	if format != "" {
		proxy.log.HTTPDebug(' ', session, format, args...)
	}
}

// autoReleaseESCLJob sends a synthetic DELETE for the eSCL scan job and
// remembers successful releases, so a client DELETE that follows can still
// receive 200 OK.
func (proxy *HTTPProxy) autoReleaseESCLJob(session int, host, jobPath string) bool {
	u := &url.URL{
		Scheme: "http",
		Host:   host,
		Path:   jobPath,
	}

	r, err := http.NewRequest("DELETE", u.String(), nil)
	if err != nil {
		proxy.log.HTTPError('!', session, "eSCL auto-release: %s", err)
		return false
	}

	autoSession := int(atomic.AddInt32(&httpSessionID, 1)-1) % 1000
	resp, err := proxy.transport.RoundTripWithSession(autoSession, r)
	if err != nil {
		proxy.log.HTTPError('!', session, "eSCL auto-release: %s", err)
		return false
	}

	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		proxy.log.HTTPError('!', autoSession, "%s", err)
	}

	resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		proxy.log.HTTPError('!', session, "eSCL auto-release: DELETE %s: %s",
			jobPath, resp.Status)
		return false
	}

	proxy.rememberAutoReleasedESCLJob(jobPath)
	proxy.log.HTTPDebug(' ', session, "eSCL auto-release: DELETE %s: %s",
		jobPath, resp.Status)

	return true
}

// autoReleaseExhaustedESCLJobs releases all eSCL jobs known to have no more
// documents, before handling requests that need the scanner to become idle.
func (proxy *HTTPProxy) autoReleaseExhaustedESCLJobs(session int) {
	jobs := proxy.takeExhaustedESCLJobsForRelease()
	for jobPath, job := range jobs {
		if !proxy.autoReleaseESCLJob(session, job.host, jobPath) {
			proxy.rememberExhaustedESCLJob(jobPath, job.host)
		}

		proxy.finishReleasingESCLJob(jobPath)
	}
}

// autoReleaseExhaustedESCLJobsAsync starts automatic release for all eSCL jobs
// known to have no more documents, without blocking the client request.
func (proxy *HTTPProxy) autoReleaseExhaustedESCLJobsAsync(session int) {
	jobs := proxy.takeExhaustedESCLJobsForRelease()
	if len(jobs) == 0 {
		return
	}

	go func() {
		defer func() {
			v := recover()
			if v != nil {
				Log.Panic(v)
			}
		}()

		for jobPath, job := range jobs {
			if !proxy.autoReleaseESCLJob(session, job.host, jobPath) {
				proxy.rememberExhaustedESCLJob(jobPath, job.host)
			}

			proxy.finishReleasingESCLJob(jobPath)
		}
	}()
}

// takeAutoReleasedESCLJob reports if path is a DELETE for a previously
// auto-released eSCL scan job, and forgets it after it is consumed.
func (proxy *HTTPProxy) takeAutoReleasedESCLJob(path string) bool {
	if !esclJobPathValid(path) {
		return false
	}

	proxy.esclAutoReleasedJobsMutex.Lock()
	defer proxy.esclAutoReleasedJobsMutex.Unlock()

	proxy.cleanupAutoReleasedESCLJobs()

	_, found := proxy.esclAutoReleasedJobs[path]
	delete(proxy.esclAutoReleasedJobs, path)
	return found
}

// rememberAutoReleasedESCLJob records a successfully auto-released eSCL job.
func (proxy *HTTPProxy) rememberAutoReleasedESCLJob(path string) {
	proxy.esclAutoReleasedJobsMutex.Lock()
	defer proxy.esclAutoReleasedJobsMutex.Unlock()

	proxy.cleanupAutoReleasedESCLJobs()
	proxy.esclAutoReleasedJobs[path] = time.Now()
}

// rememberExhaustedESCLJob records an eSCL job that returned 404 for
// NextDocument, but has not been deleted by the client yet.
func (proxy *HTTPProxy) rememberExhaustedESCLJob(path, host string) {
	if !esclJobPathValid(path) {
		return
	}

	proxy.esclAutoReleasedJobsMutex.Lock()
	defer proxy.esclAutoReleasedJobsMutex.Unlock()

	proxy.cleanupAutoReleasedESCLJobs()
	proxy.esclExhaustedJobs[path] = esclPendingJob{
		host: host,
		when: time.Now(),
	}
}

// forgetExhaustedESCLJob cancels automatic release for a job that a client is
// going to DELETE itself.
func (proxy *HTTPProxy) forgetExhaustedESCLJob(path string) {
	if !esclJobPathValid(path) {
		return
	}

	proxy.esclAutoReleasedJobsMutex.Lock()
	defer proxy.esclAutoReleasedJobsMutex.Unlock()

	delete(proxy.esclExhaustedJobs, path)
}

// takeExhaustedESCLJobsForRelease returns all known exhausted eSCL jobs,
// clears them from the exhausted set, and marks them as releasing.
func (proxy *HTTPProxy) takeExhaustedESCLJobsForRelease() map[string]esclReleasingJob {
	proxy.esclAutoReleasedJobsMutex.Lock()
	defer proxy.esclAutoReleasedJobsMutex.Unlock()

	proxy.cleanupAutoReleasedESCLJobs()

	jobs := make(map[string]esclReleasingJob)
	for path, job := range proxy.esclExhaustedJobs {
		releasing := esclReleasingJob{
			host: job.host,
			done: make(chan struct{}),
		}

		jobs[path] = releasing
		proxy.esclReleasingJobs[path] = releasing
		delete(proxy.esclExhaustedJobs, path)
	}

	return jobs
}

// finishReleasingESCLJob marks an eSCL job synthetic release as finished.
func (proxy *HTTPProxy) finishReleasingESCLJob(path string) {
	proxy.esclAutoReleasedJobsMutex.Lock()
	defer proxy.esclAutoReleasedJobsMutex.Unlock()

	releasing := proxy.esclReleasingJobs[path]
	delete(proxy.esclReleasingJobs, path)

	if releasing.done != nil {
		close(releasing.done)
	}
}

// waitForReleasingESCLJob waits if this job is being released synthetically.
func (proxy *HTTPProxy) waitForReleasingESCLJob(path string) {
	if !esclJobPathValid(path) {
		return
	}

	for {
		proxy.esclAutoReleasedJobsMutex.Lock()
		releasing := proxy.esclReleasingJobs[path]
		proxy.esclAutoReleasedJobsMutex.Unlock()

		if releasing.done == nil {
			return
		}

		<-releasing.done
	}
}

// waitForReleasingESCLJobs waits for all in-progress synthetic releases.
func (proxy *HTTPProxy) waitForReleasingESCLJobs() {
	for {
		proxy.esclAutoReleasedJobsMutex.Lock()
		done := make([]chan struct{}, 0, len(proxy.esclReleasingJobs))
		for _, releasing := range proxy.esclReleasingJobs {
			done = append(done, releasing.done)
		}
		proxy.esclAutoReleasedJobsMutex.Unlock()

		if len(done) == 0 {
			return
		}

		for _, ch := range done {
			<-ch
		}
	}
}

// cleanupAutoReleasedESCLJobs drops old eSCL job records.
func (proxy *HTTPProxy) cleanupAutoReleasedESCLJobs() {
	now := time.Now()

	for path, when := range proxy.esclAutoReleasedJobs {
		if now.Sub(when) > esclAutoReleasedJobMaxAge {
			delete(proxy.esclAutoReleasedJobs, path)
		}
	}

	for path, job := range proxy.esclExhaustedJobs {
		if now.Sub(job.when) > esclAutoReleasedJobMaxAge {
			delete(proxy.esclExhaustedJobs, path)
		}
	}
}

// esclJobPathFromNextDocumentPath returns the scan job path for
// /eSCL/ScanJobs/<id>/NextDocument requests, or an empty string otherwise.
func esclJobPathFromNextDocumentPath(path string) string {
	if !strings.HasPrefix(path, esclScanJobsPrefix) ||
		!strings.HasSuffix(path, esclNextDocumentSuffix) {
		return ""
	}

	jobPath := strings.TrimSuffix(path, esclNextDocumentSuffix)
	if !esclJobPathValid(jobPath) {
		return ""
	}

	return jobPath
}

// esclJobPathValid reports if path is exactly /eSCL/ScanJobs/<id>.
func esclJobPathValid(path string) bool {
	if !strings.HasPrefix(path, esclScanJobsPrefix) {
		return false
	}

	id := strings.TrimPrefix(path, esclScanJobsPrefix)
	return id != "" && !strings.Contains(id, "/")
}

// esclNeedsSynchronousExhaustedJobRelease reports if request requires
// exhausted jobs to be released before forwarding it to the device.
func esclNeedsSynchronousExhaustedJobRelease(r *http.Request) bool {
	return r.Method == "POST" && r.URL.Path == esclScanJobsPath ||
		r.Method == "GET" && r.URL.Path == esclScannerCapabilitiesPath
}

// esclNeedsPostResponseExhaustedJobRelease reports if request requires
// exhausted jobs to be released after forwarding the device response.
func esclNeedsPostResponseExhaustedJobRelease(r *http.Request) bool {
	return r.Method == "GET" && r.URL.Path == esclScannerStatusPath
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
