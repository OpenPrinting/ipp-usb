/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Tests for HTTP proxy helpers
 */

package main

import (
	"net/http"
	"testing"
)

func TestEsclJobPathFromNextDocumentPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{
			path: "/eSCL/ScanJobs/urn:uuid:4509a320-0062-00f6-0121-0055a6372214/NextDocument",
			want: "/eSCL/ScanJobs/urn:uuid:4509a320-0062-00f6-0121-0055a6372214",
		},
		{
			path: "/eSCL/ScanJobs/job-1/NextDocument",
			want: "/eSCL/ScanJobs/job-1",
		},
		{
			path: "/eSCL/ScanJobs//NextDocument",
			want: "",
		},
		{
			path: "/eSCL/ScanJobs/job-1/doc/NextDocument",
			want: "",
		},
		{
			path: "/eSCL/ScanJobs/job-1",
			want: "",
		},
		{
			path: "/eSCL/ScannerStatus",
			want: "",
		},
	}

	for _, test := range tests {
		got := esclJobPathFromNextDocumentPath(test.path)
		if got != test.want {
			t.Errorf("esclJobPathFromNextDocumentPath(%q): expected %q, got %q",
				test.path, test.want, got)
		}
	}
}

func TestEsclJobPathValid(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{
			path: "/eSCL/ScanJobs/urn:uuid:4509a320-0062-00f6-0121-0055a6372214",
			want: true,
		},
		{
			path: "/eSCL/ScanJobs/job-1",
			want: true,
		},
		{
			path: "/eSCL/ScanJobs/",
			want: false,
		},
		{
			path: "/eSCL/ScanJobs/job-1/NextDocument",
			want: false,
		},
		{
			path: "/eSCL/ScannerStatus",
			want: false,
		},
	}

	for _, test := range tests {
		got := esclJobPathValid(test.path)
		if got != test.want {
			t.Errorf("esclJobPathValid(%q): expected %v, got %v",
				test.path, test.want, got)
		}
	}
}

func TestEsclNeedsSynchronousExhaustedJobRelease(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{
			method: "GET",
			path:   "/eSCL/ScannerStatus",
			want:   false,
		},
		{
			method: "GET",
			path:   "/eSCL/ScannerCapabilities",
			want:   true,
		},
		{
			method: "POST",
			path:   "/eSCL/ScanJobs",
			want:   true,
		},
		{
			method: "GET",
			path:   "/eSCL/ScanJobs/job-1/NextDocument",
			want:   false,
		},
		{
			method: "DELETE",
			path:   "/eSCL/ScanJobs/job-1",
			want:   false,
		},
		{
			method: "POST",
			path:   "/eSCL/ScanJobs/job-1",
			want:   false,
		},
	}

	for _, test := range tests {
		r, err := http.NewRequest(test.method,
			"http://localhost"+test.path, nil)
		if err != nil {
			t.Fatal(err)
		}

		got := esclNeedsSynchronousExhaustedJobRelease(r)
		if got != test.want {
			t.Errorf("esclNeedsSynchronousExhaustedJobRelease(%s %s): expected %v, got %v",
				test.method, test.path, test.want, got)
		}
	}
}

func TestEsclNeedsPostResponseExhaustedJobRelease(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{
			method: "GET",
			path:   "/eSCL/ScannerStatus",
			want:   true,
		},
		{
			method: "POST",
			path:   "/eSCL/ScanJobs",
			want:   false,
		},
		{
			method: "GET",
			path:   "/eSCL/ScanJobs/job-1/NextDocument",
			want:   false,
		},
	}

	for _, test := range tests {
		r, err := http.NewRequest(test.method,
			"http://localhost"+test.path, nil)
		if err != nil {
			t.Fatal(err)
		}

		got := esclNeedsPostResponseExhaustedJobRelease(r)
		if got != test.want {
			t.Errorf("esclNeedsPostResponseExhaustedJobRelease(%s %s): expected %v, got %v",
				test.method, test.path, test.want, got)
		}
	}
}
