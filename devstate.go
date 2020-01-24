/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Per-device persistent state
 */

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/ini.v1"
)

// DevState manages a per-device persistent state (such as HTTP
// port allocation etc)
type DevState struct {
	Ident         string // Device identification
	Comment       string // Device comment
	HttpPort      int    // Allocated HTTP port
	DnsSdName     string // DNS-SD name, as reported by device
	DnsSdOverride string // DNS-SD name after collision resolution

	path string // Path to the disk file
}

// LoadDevState loads DevState from a disk file
func LoadDevState(ident string) *DevState {
	state := &DevState{
		Ident: ident,
	}
	state.path = state.devStatePath()

	// Load state file
	inifile, err := ini.Load(state.path)
	if err != nil {
		err = state.error("%s", err)
		log_debug("! STATE LOAD: %s", err)
		state.Save()
		return state
	}

	// Extract data
	var update bool
	if section, _ := inifile.GetSection("device"); section != nil {
		state.Comment = section.Comment

		err = state.loadTCPPort(section, &state.HttpPort, "http-port")
		if err != nil {
			err = state.error("%s", err)
			log_debug("! STATE LOAD: %s", err)
			update = true
		}

		state.DnsSdName = state.loadString(section, "dns-sd-name")
		state.DnsSdOverride = state.loadString(section, "dns-sd-override")
	}

	if update {
		state.Save()
	}

	return state
}

// Load TCP port
func (state *DevState) loadTCPPort(section *ini.Section,
	out *int, name string) error {

	if key, _ := section.GetKey(name); key != nil {
		port, err := key.Int()

		if err != nil {
			err = state.error("%s", err)
		} else if port < 1 || port > 65535 {
			err = state.error("%s: out of range", key.Name())
		}

		if err != nil {
			return err
		}

		*out = port
	}

	return nil
}

// Load string, defaults to ""
func (state *DevState) loadString(section *ini.Section,
	name string) string {

	if key, _ := section.GetKey(name); key != nil {
		return key.String()
	}

	return ""
}

// Save updates DevState on disk
func (state *DevState) Save() {
	os.MkdirAll(PathProgStateDev, 0755)

	inifile := ini.Empty()
	section, _ := inifile.NewSection("device")
	section.Comment = state.Comment

	if state.HttpPort > 0 {
		section.NewKey("http-port", strconv.Itoa(state.HttpPort))
	}

	if state.DnsSdName != "" {
		section.NewKey("dns-sd-name", state.DnsSdName)
	}

	if state.DnsSdOverride != "" {
		section.NewKey("dns-sd-override", state.DnsSdOverride)
	}

	err := inifile.SaveTo(state.path)
	if err != nil {
		err = state.error("%s", err)
		log_debug("! STATE SAVE: %s", err)
	}
}

// HttpPort allocates HTTP port and updates persistent configuration
func (state *DevState) HttpListen() (net.Listener, error) {
	port := state.HttpPort

	// Check that preallocated port is within the configured range
	if !(Conf.HttpMinPort <= port && port <= Conf.HttpMaxPort) {
		port = 0
	}

	// Try to allocate port used before
	if port != 0 {
		listener, err := NewListener(port)
		if err == nil {
			return listener, nil
		}
	}

	// Allocate a port
	for port = Conf.HttpMinPort; port <= Conf.HttpMaxPort; port++ {
		listener, err := NewListener(port)
		if err == nil {
			state.HttpPort = port
			state.Save()
			return listener, nil
		}
	}

	err := state.error("failed to allocate HTTP port", state.Ident)
	log_debug("! STATE PORT: %s", err)

	return nil, err
}

// devStatePath returns a path to the DevState file
func (state *DevState) devStatePath() string {
	return filepath.Join(PathProgStateDev, state.Ident+".state")
}

// error creates a state-related error
func (state *DevState) error(format string, args ...interface{}) error {
	return fmt.Errorf(state.Ident+": "+format, args...)
}

// Set device comment
func (state *DevState) SetComment(comment string) {
	if comment != state.Comment {
		state.Comment = comment
		state.Save()
	}
}
