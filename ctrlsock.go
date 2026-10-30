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

	// ctrlsockServer is a HTTP server that runs on a top of
	// the status socket
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
	Disables bool `json:"disabled,omitempty"`
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
type ctrlsockListener struct {
	*net.UnixListener // Underlying net.UnixListener
}

// ctrlsockListen returns new ctrlsockListener
func ctrlsockListen(addr *net.UnixAddr) (net.Listener, error) {
	l, err := net.ListenUnix("unix", CtrlsockAddr)
	if err != nil {
		return nil, err
	}

	return ctrlsockListener{l}, nil
}

// Accept returns new connection
func (l ctrlsockListener) Accept() (net.Conn, error) {
	conn, err := l.UnixListener.AcceptUnix()
	if err != nil {
		return nil, err
	}

	return newCtrlsockConn(conn), nil
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

	switch {
	case r.Method == "GET" && r.URL.Path == "/devices":
		mime, data, err = ctrlsockGETDevices()
	case r.Method == "GET" && r.URL.Path == "/status":
		mime, data, err = ctrlsockGETStatus()
	case r.Method == "GET" && r.URL.Path == "/version":
		mime, data, err = ctrlsockGETVersion()
	case r.Method == "POST" && r.URL.Path == "/get/conf":
		mime, data, err = ctrlsockPOSTGetconf(rq)
	default:
		ctrlsockRequestError(w, r, http.StatusNotFound, "Not found")
		return
	}

	// Send the response
	if err != nil {
		ctrlsockRequestError(w, r,
			http.StatusInternalServerError, err.Error())
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
func ctrlsockGETDevices() (string, []byte, error) {
	return "", nil, errors.New("not implemented")
}

// ctrlsockGETStatus handles the GET /status request
func ctrlsockGETStatus() (string, []byte, error) {
	return ctrlSockMIMEText, StatusFormat(), nil
}

// ctrlsockGETVersion handles the GET /version request
func ctrlsockGETVersion() (string, []byte, error) {
	return ctrlSockMIMEText, []byte(CtrlsockAPIVersion), nil
}

// ctrlsockPOSTGetconf handles the POST /get/conf request
func ctrlsockPOSTGetconf(rq ctrlsockCONFData) (string, []byte, error) {
	states := DevStateSearch(rq.MFG, rq.MDL, rq.SN, rq.HWID, rq.URI)
	items := make([]ctrlsockCONFData, 0, len(states))

	for _, state := range states {
		if state.MDL == "" || state.MFG == "" || state.SN == "" {
			continue
		}

		item := ctrlsockCONFData{
			MDL:  state.MDL,
			MFG:  state.MFG,
			SN:   state.SN,
			HWID: state.HWID,
			URI:  state.URI(),
		}

		items = append(items, item)
	}

	response := ctrlsockJSONResponse{Data: items}
	buf := bytes.Buffer{}
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.Encode(response)

	return ctrlSockMIMEJSON, buf.Bytes(), nil
}

// CtrlsockStart starts control socket server
func CtrlsockStart() error {
	Log.Debug(' ', "ctrlsock: listening at %q", PathControlSocket)

	// Listen the socket
	os.Remove(PathControlSocket)

	listener, err := ctrlsockListen(CtrlsockAddr)
	if err != nil {
		return err
	}

	// Make socket accessible to everybody. Error is ignored,
	// it's not a reason to abort ipp-usb
	os.Chmod(PathControlSocket, 0777)

	// Start HTTP server on a top of the listening socket
	go func() {
		ctrlsockServer.Serve(listener)
	}()

	return nil
}

// CtrlsockStop stops the control socket server
func CtrlsockStop() {
	Log.Debug(' ', "ctrlsock: shutdown")
	ctrlsockServer.Close()
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
