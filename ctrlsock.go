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
	"log"
	"net"
	"net/http"
	"os"
	"syscall"
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
)

// ctrlsockHandler handles HTTP requests that come over the
// control socket
func ctrlsockHandler(w http.ResponseWriter, r *http.Request) {
	Log.Debug(' ', "ctrlsock: %s %s", r.Method, r.URL)

	// Catch panics to log
	defer func() {
		v := recover()
		if v != nil {
			Log.Panic(v)
		}
	}()

	// Check request method
	if r.Method != "GET" {
		http.Error(w, r.Method+": method not supported",
			http.StatusMethodNotAllowed)
		return
	}

	// Check request path
	if r.URL.Path != "/status" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Handle the request
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	httpNoCache(w)
	w.WriteHeader(http.StatusOK)
	w.Write(StatusFormat())
}

// CtrlsockStart starts control socket server
func CtrlsockStart() error {
	Log.Debug(' ', "ctrlsock: listening at %q", PathControlSocket)

	// Listen the socket
	os.Remove(PathControlSocket)

	listener, err := net.ListenUnix("unix", CtrlsockAddr)
	if err != nil {
		return err
	}

	// Make socket accessible to everybody. Error is ignores,
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
