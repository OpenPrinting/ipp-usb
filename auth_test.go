/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Tests for UID authentication
 */

package main

import (
	"testing"
)

// TestAuthUIDrequiresUID tests authUIDrequiresUID
func TestAuthUIDrequiresUID(t *testing.T) {
	type testData struct {
		name  string
		rules []*AuthUIDRule
		need  bool
	}

	tests := []testData{
		{
			name:  "not configured",
			rules: nil,
			need:  false,
		},
		{
			name: "wildcards only",
			rules: []*AuthUIDRule{
				{Name: "*", Allowed: AuthOpsAll},
				{Name: "@*", Allowed: AuthOpsPrint},
			},
			need: false,
		},
		{
			// Default ipp-usb.conf: admin rules apply to the
			// control socket only, so HTTP doesn't need the UID
			name: "default config",
			rules: []*AuthUIDRule{
				{Name: "*", Allowed: AuthOpsAll},
				{Name: "root", Allowed: AuthOpsAdmin},
				{Name: "@lpadmin", Allowed: AuthOpsAdmin},
			},
			need: false,
		},
		{
			name: "user rule for HTTP operation",
			rules: []*AuthUIDRule{
				{Name: "alice", Allowed: AuthOpsScan},
			},
			need: true,
		},
		{
			name: "group rule for HTTP operation",
			rules: []*AuthUIDRule{
				{Name: "*", Allowed: AuthOpsPrint},
				{Name: "@lpadmin", Allowed: AuthOpsConfig},
			},
			need: true,
		},
		{
			name: "admin combined with HTTP operation",
			rules: []*AuthUIDRule{
				{Name: "root", Allowed: AuthOpsAdmin | AuthOpsConfig},
			},
			need: true,
		},
	}

	saved := Conf
	defer func() { Conf = saved }()

	// The shipped ipp-usb.conf must not require the client UID for
	// HTTP requests (it can't be obtained under snap confinement)
	if err := confLoadInternal("ipp-usb.conf"); err != nil {
		t.Fatalf("ipp-usb.conf: %s", err)
	}
	if Conf.ConfAuthUID == nil {
		t.Fatalf("ipp-usb.conf: no [auth uid] rules loaded")
	}
	if authUIDrequiresUID() {
		t.Errorf("ipp-usb.conf: authUIDrequiresUID() = true, expected false")
	}

	for _, test := range tests {
		Conf.ConfAuthUID = test.rules
		need := authUIDrequiresUID()
		if need != test.need {
			t.Errorf("%s: authUIDrequiresUID() = %v, expected %v",
				test.name, need, test.need)
		}
	}
}
