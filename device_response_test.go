package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func testLogger() *LogMessage {
	lg := NewLogger().ToNowhere()
	lg.SetLevels(0)
	return &lg.LogMessage
}

func TestIppGetPrinterAttributesRejectsHugeBody(t *testing.T) {
	body := strings.Repeat("x", deviceResponseLimit+1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()

	_, _, err := ippGetPrinterAttributes(testLogger(), srv.Client(), NewQuirks(), srv.URL)
	want := fmt.Sprintf("HTTP: response exceeds %d bytes", deviceResponseLimit)
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %s", err, want)
	}
}

func TestIppGetPrinterAttributesReadsSmallBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "not-ipp")
	}))
	defer srv.Close()

	_, _, err := ippGetPrinterAttributes(testLogger(), srv.Client(), NewQuirks(), srv.URL)
	if err == nil || strings.Contains(err.Error(), "exceeds") || !strings.Contains(err.Error(), "IPP decode:") {
		t.Fatalf("got %v", err)
	}
}

func TestEsclServiceRejectsHugeBody(t *testing.T) {
	body := strings.Repeat("x", deviceResponseLimit+1)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	_, err = EsclService(testLogger(), nil, port, UsbDeviceInfo{}, nil, http.DefaultClient)
	want := fmt.Sprintf("eSCL: response exceeds %d bytes", deviceResponseLimit)
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %s", err, want)
	}
}
