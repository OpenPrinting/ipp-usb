/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Control socket handler
 *
 * ipp-usb runs a HTTP server on a top of the unix domain control
 * socket.
 *
 * Currently it is only used to obtain a per-device status from the
 * running daemon. Using HTTP here sounds as overkill, but taking
 * in account that it costs us virtually nothing and this mechanism
 * is well-extendable, this is a good choice
 */

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"
)

const (
	// CtrlsockAPIVersion is the current ipp-usb API version.
	//
	// The protocol is versioned independently of the *ipp-usb* software
	// release. The protocol version identifies the *ipp-usb* release in
	// which the latest protocol changes were introduced. It remains
	// unchanged across software releases that do not change the protocol.
	CtrlsockAPIVersion = "0.9.35"

	// Common MIME types
	ctrlSockMIMEJSON = "application/json"
	ctrlSockMIMEText = "text/plain; charset=utf-8"

	// ctrlSockMAXRequestSize is the size limit for the POST request body
	ctrlSockMAXRequestSize = 65536
)

var (
	// CtrlsockAddr contains control socket address in
	// a form of the net.UnixAddr structure
	CtrlsockAddr = &net.UnixAddr{Name: PathControlSocket, Net: "unix"}

	// ctrlsockSocket is the control socket's listener
	ctrlsockSocket *ctrlsockListener

	// ctrlsockServer is a HTTP server that runs on a top of
	// the control socket
	ctrlsockServer = http.Server{
		Handler:  http.HandlerFunc(ctrlsockHandler),
		ErrorLog: log.New(Log.LineWriter(LogError, '!'), "", 0),
	}

	// ctrlsockConnTable tracks control socket connections,
	// indexed by remote address
	ctrlsockConnTable = map[string]*ctrlsockConn{}

	// ctrlsockConnTableLock protects the ctrlsockConnTable
	ctrlsockConnTableLock sync.Mutex
)

// ctrlsockCONFData is the data record for POST /get/conf and POST /set/conf
// requests and response.
type ctrlsockCONFData struct {
	// Device identification
	MFG  string `json:"usb_MFG,omitempty"`
	MDL  string `json:"usb_MDL,omitempty"`
	SN   string `json:"usb_SN,omitempty" `
	HWID string `json:"usb_HWID,omitempty"`
	URI  string `json:"usb_URI,omitempty"`

	// Device configuration
	Disabled *bool `json:"disabled,omitempty"`
}

// ctrlsockCONFDatafromDevState fills ctrlsockCONFData from
// the DevState structure
func ctrlsockCONFDatafromDevState(state *DevState) ctrlsockCONFData {
	disabled := state.Disabled
	data := ctrlsockCONFData{
		MDL:      state.MDL,
		MFG:      state.MFG,
		SN:       state.SN,
		HWID:     state.HWID,
		URI:      state.URIString(),
		Disabled: &disabled,
	}
	return data
}

// ctrlsockJSONResponse is the common structure for all JSON responses.
type ctrlsockJSONResponse struct {
	Err  string      `json:"err,omitempty"`
	Data interface{} `json:"data,omitempty"`
}

// ctrlsockConnAddress is the unique fake address for
// the control socket connection.
type ctrlsockConnAddress string

// newCtrlsockConnAddress returns the new unique ctrlsockConnAddress.
func newCtrlsockConnAddress() ctrlsockConnAddress {
	var buf [16]byte
	rand.Read(buf[:])

	const format = "" +
		"%8.8x-" +
		"%4.4x-" +
		"%4.4x-" +
		"%4.4x-" +
		"%12.12x"

	s := fmt.Sprintf(
		format,
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16])

	return ctrlsockConnAddress(s)
}

// Network returns network name for the ctrlsockConnAddress.
// It implements the net.Addr interface.
func (ctrlsockConnAddress) Network() string {
	return "unix"
}

// String returns string form of ctrlsockConnAddress.
// It implements the net.Addr interface.
func (addr ctrlsockConnAddress) String() string {
	return string(addr)
}

// ctrlsockConn wraps net.UnixConn and adds unique remote
// address.
type ctrlsockConn struct {
	*net.UnixConn                     // Underlying connection
	remote        ctrlsockConnAddress // Remote address
}

// newCtrlsockConn wraps net.UnixConn into ctrlsockConn
func newCtrlsockConn(conn *net.UnixConn) net.Conn {
	addr := newCtrlsockConnAddress()
	conn2 := &ctrlsockConn{UnixConn: conn, remote: addr}

	Log.Debug('+', "ctrlsock: connection accepted: %s", addr)

	ctrlsockConnTableLock.Lock()
	ctrlsockConnTable[addr.String()] = conn2
	ctrlsockConnTableLock.Unlock()

	return conn2
}

// getCtrlsockConn returns ctrlsockConn for the incoming http.Request
func getCtrlsockConn(r *http.Request) *ctrlsockConn {
	ctrlsockConnTableLock.Lock()
	defer ctrlsockConnTableLock.Unlock()

	return ctrlsockConnTable[r.RemoteAddr]
}

// Close closes the connection.
func (conn ctrlsockConn) Close() error {
	addr := conn.remote
	Log.Debug('-', "ctrlsock: connection closed: %s", addr)

	ctrlsockConnTableLock.Lock()
	delete(ctrlsockConnTable, addr.String())
	ctrlsockConnTableLock.Unlock()

	return conn.UnixConn.Close()
}

// RemoteAddr returns remote address of ctrlsockConn.
func (conn ctrlsockConn) RemoteAddr() net.Addr {
	return conn.remote
}

// Ucred returns peer credential for the connection
// represented by ctrlsockConn.
func (conn ctrlsockConn) Ucred() (*syscall.Ucred, error) {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}

	var ucred *syscall.Ucred
	var err2 error
	err = rawConn.Control(func(fd uintptr) {
		ucred, err2 = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})

	switch {
	case err != nil:
		return nil, err
	case err2 != nil:
		return nil, err2
	}

	return ucred, nil
}

// ctrlsockListener wraps net.UnixListener and
// adds unique remote address to each accepted connection.
//
// This address is later available as the http.Request.RemoteAddr
// for every HTTP requests received from the control socket.
//
// It allows us to match HTTP-level requests against the
// underlying AF_UNIX socket. It is required to implement
// the SO_PEERCRED-based authentication of the control
// socket clients.
//
// It also reports its idle time (time since the last incoming
// connection accepted) and can be paused and resumed.
type ctrlsockListener struct {
	ln          *net.UnixListener // Underlying net.UnixListener
	cond        *sync.Cond        // Protects state
	paused      bool              // Listener is paused
	pauseWait   sync.WaitGroup    // To wait for active Accept
	timestamp   time.Time         // Last successful Accept timestamp
	wasAccepted bool              // At least one connection was accepted
}

// ctrlsockListen returns new ctrlsockListener
func ctrlsockListen(addr *net.UnixAddr) (*ctrlsockListener, error) {
	ln, err := net.ListenUnix("unix", CtrlsockAddr)
	if err != nil {
		return nil, err
	}

	l := &ctrlsockListener{
		ln:        ln,
		cond:      sync.NewCond(&sync.Mutex{}),
		timestamp: time.Now(),
	}

	return l, nil
}

// Accept returns new connection.
func (l *ctrlsockListener) Accept() (net.Conn, error) {
	l.cond.L.Lock()
	defer l.cond.L.Unlock()

	for {
		if l.paused {
			l.cond.Wait()
		} else {
			l.pauseWait.Add(1)
			l.cond.L.Unlock()
			conn, err := l.ln.AcceptUnix()
			l.cond.L.Lock()
			l.pauseWait.Done()

			if err == nil {
				l.timestamp = time.Now()
				l.wasAccepted = true
				return newCtrlsockConn(conn), nil
			}

			if err != nil {
				err2, ok := err.(net.Error)
				if ok && !err2.Timeout() {
					return nil, err
				}
			}
		}
	}
}

// Close closes the listener.
func (l *ctrlsockListener) Close() error {
	err := l.ln.Close()
	l.Resume()
	return err
}

// Addr returns the listener's local network address.
func (l *ctrlsockListener) Addr() net.Addr {
	return l.ln.Addr()
}

// Pause puts listener on pause. All pending Accept calls
// will just wait until listener is resumed.
func (l *ctrlsockListener) Pause() {
	// Put listener into the paused state and wake up
	// pending l.ln.Accept(), if any
	l.cond.L.Lock()
	l.paused = true
	l.ln.SetDeadline(time.Now()) // Will time-out pending Accept
	l.cond.L.Unlock()

	// Wait for pending l.ln.Accept() to return
	l.pauseWait.Wait()
}

// Resume resumes the normal listener operations.
func (l *ctrlsockListener) Resume() {
	l.cond.L.Lock()
	l.paused = false
	l.ln.SetDeadline(time.Time{})
	l.cond.L.Unlock()

	l.cond.Signal()
}

// Idle reports how much time has elapsed since last connection
// has been successfully accepted.
//
// Additionally it returns an indication if the control socket
// connection was ever accepted at all.
func (l *ctrlsockListener) Idle() (time.Duration, bool) {
	l.cond.L.Lock()
	defer l.cond.L.Unlock()

	return time.Now().Sub(l.timestamp), l.wasAccepted
}

// ctrlsockHandler handles HTTP requests that come over the
// control socket
func ctrlsockHandler(w http.ResponseWriter, r *http.Request) {
	// Catch panics to log
	defer func() {
		v := recover()
		if v != nil {
			Log.Panic(v)
		}
	}()

	// Obtain peer credentials
	conn := getCtrlsockConn(r)
	if conn == nil {
		ctrlsockRequestError(w, r, http.StatusInternalServerError,
			"unknown connection from "+r.RemoteAddr)
		return
	}

	ucred, err := conn.Ucred()
	if err != nil {
		ctrlsockRequestError(w, r, http.StatusInternalServerError,
			err.Error())
		return
	}

	Log.Debug(' ', "ctrlsock: request from UID=%v", ucred.Uid)

	// Check request method
	if r.Method != "GET" && r.Method != "POST" {
		message := r.Method + ": method not supported"
		ctrlsockRequestError(w, r, http.StatusMethodNotAllowed, message)
		return
	}

	// Parse POST request parameters
	var rq ctrlsockCONFData

	if r.Method == "POST" {
		// Validate content type
		contentType := r.Header.Get("Content-Type")
		mediatype, _, err := mime.ParseMediaType(contentType)

		if err != nil {
			ctrlsockRequestError(w, r, http.StatusBadRequest,
				"Invalid or missing Content-Type")
			return
		}

		if mediatype != "application/json" {
			ctrlsockRequestError(w, r, http.StatusUnsupportedMediaType,
				"Content-Type must be application/json")
			return
		}

		// Parse body
		rd := io.LimitReader(r.Body, ctrlSockMAXRequestSize)
		err = json.NewDecoder(rd).Decode(&rq)
		if err != nil {
			ctrlsockRequestError(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}

	// Dispatch the request
	var mime string
	var data []byte
	var status int

	switch {
	case r.Method == "GET" && r.URL.Path == "/devices":
		mime, data, status, err = ctrlsockGETDevices()
	case r.Method == "GET" && r.URL.Path == "/status":
		mime, data, status, err = ctrlsockGETStatus()
	case r.Method == "GET" && r.URL.Path == "/version":
		mime, data, status, err = ctrlsockGETVersion()
	case r.Method == "POST" && r.URL.Path == "/get/conf":
		mime, data, status, err = ctrlsockPOSTGetConf(rq)
	case r.Method == "POST" && r.URL.Path == "/set/conf":
		status, err = AuthAdminRequest(Log, int(ucred.Uid))
		if err == nil {
			mime, data, status, err = ctrlsockPOSTSetConf(rq)
		}
	default:
		ctrlsockRequestError(w, r, http.StatusNotFound, "Not found")
		return
	}

	// Send the response
	if err != nil {
		ctrlsockRequestError(w, r, status, err.Error())
		return
	}

	ctrlsockRequestComplete(w, r, http.StatusOK, data, mime)
}

// ctrlsockRequestComplete completes the control socket request
func ctrlsockRequestComplete(w http.ResponseWriter, r *http.Request,
	status int, data []byte, mime string) {

	ctrlsockRequestLog(r, status, "")

	w.Header().Set("Content-Type", mime)
	httpNoCache(w)
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// ctrlsockRequestError completes control socket request with the error.
func ctrlsockRequestError(w http.ResponseWriter, r *http.Request,
	status int, message string) {

	if message == "" {
		message = http.StatusText(status)
	}

	ctrlsockRequestLog(r, status, message)
	http.Error(w, message, status)
}

// ctrlsockRequestLog writes logging information related to
// the request processing.
func ctrlsockRequestLog(r *http.Request, status int, message string) {
	Log.Debug(' ', "ctrlsock: %s %s -- %3.3d %s",
		r.Method, r.URL, status, message)
}

// ctrlsockGETDevices handles the GET /devices request
func ctrlsockGETDevices() (string, []byte, int, error) {
	return "", nil, http.StatusNotImplemented, errors.New("not implemented")
}

// ctrlsockGETStatus handles the GET /status request
func ctrlsockGETStatus() (string, []byte, int, error) {
	return ctrlSockMIMEText, StatusFormat(), http.StatusOK, nil
}

// ctrlsockGETVersion handles the GET /version request
func ctrlsockGETVersion() (string, []byte, int, error) {
	return ctrlSockMIMEText, []byte(CtrlsockAPIVersion), http.StatusOK, nil
}

// ctrlsockPOSTGetConf handles the POST /get/conf request
func ctrlsockPOSTGetConf(rq ctrlsockCONFData) (string, []byte, int, error) {
	states := DevStateSearch(rq.MFG, rq.MDL, rq.SN, rq.HWID, rq.URI)
	items := make([]ctrlsockCONFData, 0, len(states))

	for _, state := range states {
		if state.MDL == "" || state.MFG == "" || state.SN == "" {
			continue
		}

		item := ctrlsockCONFDatafromDevState(state)
		items = append(items, item)
	}

	response := ctrlsockJSONResponse{Data: items}
	buf := bytes.Buffer{}
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.Encode(response)

	return ctrlSockMIMEJSON, buf.Bytes(), http.StatusOK, nil
}

// ctrlsockPOSTSetConf handles the POST /get/conf request
func ctrlsockPOSTSetConf(rq ctrlsockCONFData) (string, []byte, int, error) {
	var err error

	switch {
	case rq.MFG != "" && rq.MDL != "" && rq.SN != "":
	case rq.HWID != "" && rq.SN != "":
	case rq.URI != "":
	default:
		err = errors.New("ambiguous request")
	}

	response := ctrlsockJSONResponse{}
	if err == nil {
		states := DevStateSearch(rq.MFG, rq.MDL, rq.SN, rq.HWID, rq.URI)
		if len(states) == 0 {
			err = errors.New("device not found")
		} else {
			state := states[0]
			save := false

			if rq.Disabled != nil {
				state.Disabled = *rq.Disabled
				save = true
			}

			if save {
				err = PnPUpdateState(state)
				if err == nil {
					state.Save()
				}
			}

			if err == nil {
				response.Data = ctrlsockCONFDatafromDevState(state)
			}
		}
	}

	if err != nil {
		response.Err = err.Error()
	}

	buf := bytes.Buffer{}
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.Encode(response)

	return ctrlSockMIMEJSON, buf.Bytes(), http.StatusOK, nil
}

// CtrlsockStart starts control socket server
func CtrlsockStart() error {
	Log.Debug(' ', "ctrlsock: listening at %q", PathControlSocket)

	// Listen the socket
	os.Remove(PathControlSocket)

	var err error
	ctrlsockSocket, err = ctrlsockListen(CtrlsockAddr)
	if err != nil {
		return err
	}

	// Make socket accessible to everybody. Error is ignored,
	// it's not a reason to abort ipp-usb
	os.Chmod(PathControlSocket, 0777)

	// Start HTTP server on a top of the listening socket
	ctrlsockServer.SetKeepAlivesEnabled(false)
	go func() {
		ctrlsockServer.Serve(ctrlsockSocket)
	}()

	return nil
}

// CtrlsockStop stops the control socket server
func CtrlsockStop() {
	Log.Debug(' ', "ctrlsock: shutdown")
	ctrlsockServer.Close()
}

// CtrlsockIdle reports how much time has elapsed since last connection
// has been successfully accepted.
//
// Additionally it returns an indication if the control socket
// connection was ever accepted at all.
func CtrlsockIdle() (time.Duration, bool) {
	return ctrlsockSocket.Idle()
}

// CtrlsockPause puts listener on pause. All pending Accept calls
// will just wait until listener is resumed.
func CtrlsockPause() {
	Log.Debug(' ', "ctrlsock: pause")
	ctrlsockSocket.Pause()
}

// CtrlsockResume resumes the normal listener operations.
func CtrlsockResume() {
	Log.Debug(' ', "ctrlsock: resume")
	ctrlsockSocket.Resume()
}

// CtrlsockDial connects to the control socket of the running
// ipp-usb daemon
func CtrlsockDial() (net.Conn, error) {
	conn, err := net.DialUnix("unix", nil, CtrlsockAddr)

	if err == nil {
		return conn, err
	}

	if neterr, ok := err.(*net.OpError); ok {
		if syserr, ok := neterr.Err.(*os.SyscallError); ok {
			switch syserr.Err {
			case syscall.ECONNREFUSED, syscall.ENOENT:
				err = ErrNoIppUsb

			case syscall.EACCES, syscall.EPERM:
				err = ErrAccess
			}
		}
	}

	return conn, err
}
