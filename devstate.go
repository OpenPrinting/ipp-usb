/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Per-device persistent state
 */

package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

// DevState manages a per-device persistent state (such as HTTP
// port allocation etc)
type DevState struct {
	Ident         string // Device identification
	HTTPPort      int    // Allocated HTTP port
	DNSSdName     string // DNS-SD name, as reported by device
	DNSSdOverride string // DNS-SD name after collision resolution

	comment string // Comment in the state file
	path    string // Path to the disk file
}

// LoadDevState loads DevState from a disk file
//
// This function always succeeds, even in a case of file i/o errors.
// In a worst case we loose state persistence, not other functionality.
func LoadDevState(ident, comment string) *DevState {
	state := &DevState{
		Ident:   ident,
		comment: comment,
	}
	state.path = state.devStatePath()

	// Read state file
	ini, err := OpenIniFile(state.path)
	if err == nil {
		err = state.load(ini)
		ini.Close()
	}

	if err != nil && err != io.EOF {
		if !os.IsNotExist(err) {
			Log.Error('!', "STATE LOAD: %s", state.error("%s", err))
		}
	}

	return state
}

// LoadUsedPorts loads ports used by some of devices.
//
// The returned map contains one entry per used port. Value of this
// entry is a human-readable string, reasonable for logging
func LoadUsedPorts() (ports map[int]string) {
	ports = make(map[int]string)

	// Read the PathProgStateDev (normally "/var/ipp-usb/dev")
	// directory.
	var files []os.FileInfo
	var err error

	dir, err := os.Open(PathProgStateDev)
	if err == nil {
		files, err = dir.Readdir(0)
		dir.Close()
	}

	if err != nil {
		Log.Error('!', "Can't load existing ports allocation")
		Log.Error('!', "%s", err)
		return
	}

	if err != nil {
		return
	}

	// Scan found files
	for _, file := range files {
		Log.Debug(' ', "== %s", file.Name())
		if !file.Mode().IsRegular() {
			continue
		}

		path := filepath.Join(PathProgStateDev, file.Name())
		ini, err := OpenIniFile(path)
		if err != nil {
			Log.Error('!', "%s", err)
			continue
		}

		state := &DevState{}
		err = state.load(ini)
		ini.Close()

		if err != nil {
			Log.Error('!', "%s", err)
			continue
		}

		if state.HTTPPort != 0 {
			ports[state.HTTPPort] = file.Name()
		}
	}

	return
}

// load performs an actual work of loading the DevState file
func (state *DevState) load(ini *IniFile) error {
	err := ini.Lock(FileLockWait)
	if err == nil {
		defer ini.Unlock()
	}

	for err == nil {
		var rec *IniRecord
		rec, err = ini.Next()
		if err != nil {
			break
		}

		switch rec.Section {
		case "device":
			switch rec.Key {
			case "http-port":
				err = state.loadTCPPort(&state.HTTPPort, rec)
			case "dns-sd-name":
				state.DNSSdName = rec.Value
			case "dns-sd-override":
				state.DNSSdOverride = rec.Value
			}
		}

	}

	if err == io.EOF {
		err = nil
	}

	return err
}

// Load TCP port
func (state *DevState) loadTCPPort(out *int, rec *IniRecord) error {
	port, err := strconv.Atoi(rec.Value)

	if err != nil {
		err = state.error("%s", err)
	} else if port < 1 || port > 65535 {
		err = state.error("%s: out of range", rec.Key)
	}

	if err != nil {
		return err
	}

	*out = port

	return nil
}

// Save updates DevState on disk
func (state *DevState) Save() {
	os.MkdirAll(PathProgStateDev, 0755)

	var buf bytes.Buffer

	if state.comment != "" {
		fmt.Fprintf(&buf, "; %s\n", state.comment)
	}

	fmt.Fprintf(&buf, "[device]\n")
	fmt.Fprintf(&buf, "http-port       = %d\n", state.HTTPPort)
	fmt.Fprintf(&buf, "dns-sd-name     = %q\n", state.DNSSdName)
	fmt.Fprintf(&buf, "dns-sd-override = %q\n", state.DNSSdOverride)

	err := state.save(buf.Bytes())
	if err != nil {
		err = state.error("%s", err)
		Log.Error('!', "STATE SAVE: %s", err)
	}
}

// save performs an actual work of saving state file
func (state *DevState) save(data []byte) error {
	f, err := os.OpenFile(state.path,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)

	if err != nil {
		return err
	}

	err = FileLock(f, FileLockWait)
	if err != nil {
		f.Close()
		return err
	}

	_, err = f.Write(data)
	FileUnlock(f)

	if err != nil {
		f.Close()
		return err
	}

	return f.Close()
}

// HTTPListen allocates HTTP port and updates persistent configuration
func (state *DevState) HTTPListen() (net.Listener, error) {
	port := state.HTTPPort

	// Check that preallocated port is within the configured range
	if !(Conf.HTTPMinPort <= port && port <= Conf.HTTPMaxPort) {
		port = 0
	}

	// Try to allocate port used before
	if port != 0 {
		listener, err := NewListener(port)
		if err == nil {
			return listener, nil
		}
	}

	// Allocate a port. Don't reuse ports allocated by other
	// devices.
	ports := LoadUsedPorts()

	for port = Conf.HTTPMinPort; port <= Conf.HTTPMaxPort; port++ {
		used := ports[port]
		if used != "" {
			Log.Info(' ', "HTTP port %d used by %s", port, used)
			continue
		}

		listener, err := NewListener(port)
		if err == nil {
			state.HTTPPort = port
			state.Save()
			return listener, nil
		}
	}

	// No success so far. Repeat allocation attempt, ignoring
	// existent allocations
	for port = Conf.HTTPMinPort; port <= Conf.HTTPMaxPort; port++ {
		listener, err := NewListener(port)
		if err == nil {
			state.HTTPPort = port
			state.Save()
			return listener, nil
		}
	}

	// Give up and return an error
	err := state.error("failed to allocate HTTP port", state.Ident)
	Log.Error('!', "STATE PORT: %s", err)

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
