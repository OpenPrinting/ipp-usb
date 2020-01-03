package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ----- Flags (program options) -----
var (
	flag_cport   = flag.Int("c", 60000, "HTTP port to connect to")
	flag_lport   = flag.Int("l", 60001, "HTTP port to listen to")
	flag_timeout = flag.Int("t", 5, "Idle connection timeout, seconds")
)

// ----- HTTP proxy -----
// Handle HTTP request
func httpHandler(w http.ResponseWriter, r *http.Request) {
	log_debug("< %s %s", r.Method, r.URL)

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

	// Adjust request headers
	httpRemoveHopByHopHeaders(r.Header)
	r.Header.Add("Connection", "close")

	r.URL.Scheme = "http"
	r.URL.Host = fmt.Sprintf("localhost:%d", *flag_cport)

	// Serve the request
	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		httpError(w, r, http.StatusServiceUnavailable, err.Error())
		return
	}

	httpRemoveHopByHopHeaders(resp.Header)
	httpCopyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if resp.ContentLength >= 0 {
		io.Copy(w, resp.Body)
	} else {
		httpCopyDataWithTimeout(w, resp.Body)
	}
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

// Copy HTTP response body with timeout
func httpCopyDataWithTimeout(dst io.Writer, src io.ReadCloser) {
	var buf [32 * 1024]byte
	var sz, off, n int
	var err error

	duration := time.Second * time.Duration(*flag_timeout)
	tmr := time.NewTimer(duration)

	c := make(chan struct{})

	go httpTimeGoroutine(tmr, c, src)

	for err == nil {
		if sz > 0 {
			n, err = dst.Write(buf[off:])
			sz -= n
			off += n
		} else {
			sz, err = src.Read(buf[:])
			off = 0
		}

		tmr.Stop()
		tmr.Reset(duration)
	}

	close(c)
}

// ----- Logging -----
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

// Print usage error and exit
func log_usage(format string, args ...interface{}) {
	log_debug(format, args...)
	log_debug("Try %s -h for more information", os.Args[0])
	os.Exit(1)
}

func main() {
	// Parse arguments
	flag.Parse()
	if *flag_lport < 1 || *flag_lport > 65535 {
		log_usage("Invalid value for option -l")
	}
	if *flag_cport < 1 || *flag_cport > 65535 {
		log_usage("Invalid value for option -c")
	}
	if flag.NArg() > 0 {
		log_usage("Invalid argument %s", flag.Args()[0])
	}

	// Create HTTP server
	addr := fmt.Sprintf("localhost:%d", *flag_lport)
	log_debug("Starting HTTP server at http://%s", addr)
	err := http.ListenAndServe(addr, http.HandlerFunc(httpHandler))
	if err != nil {
		log_exit("%s", err)
	}
}
