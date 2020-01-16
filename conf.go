/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Program configuration
 */

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/ini.v1"
)

const (
	// Configuration file name
	ConfFileName = "ipp-usb.conf"

	// Configuration directory
	ConfFileDir = "/etc/ipp-usb"
)

// Conf represents a program configuration
type Configuration struct {
	HttpMinPort int  // Starting port number for HTTP to bind to
	HttpMaxPort int  // Ending port number for HTTP to bind to
	DnsSdEnable bool // Enable DNS-SD advertising
}

var Conf = Configuration{
	HttpMinPort: 60000,
	DnsSdEnable: true,
}

// Load the program configuration
func ConfLoad() error {
	err := confLoadInternal()
	if err != nil {
		err = fmt.Errorf("conf: %s", err)
	}

	return err
}

// Create "bad value" error
func confBadValue(key *ini.Key, format string, args ...interface{}) error {
	return fmt.Errorf(key.Name()+": "+format, args...)
}

// Load the program configuration -- internal version
func confLoadInternal() error {
	// Obtain path to executable directory
	exepath, err := os.Executable()
	if err != nil {
		return err
	}

	exepath = filepath.Dir(exepath)

	// Load configuration file
	inifile, err := ini.LooseLoad(
		filepath.Join(ConfFileDir, ConfFileName),
		filepath.Join(exepath, ConfFileName))

	if err != nil {
		return err
	}

	// Extract options
	var section *ini.Section
	var key *ini.Key

	section, err = inifile.GetSection("network")
	if section != nil {
		key, err = section.GetKey("http-min-port")
		if key != nil {
			port, err := key.Uint()
			if err == nil && (port < 1 || port > 65535) {
				err = confBadValue(key, "must be in range 1...65535")
			}
			if err != nil {
				return err
			}

			Conf.HttpMinPort = int(port)
		}

		key, err = section.GetKey("http-max-port")
		if key != nil {
			port, err := key.Uint()
			if err == nil && (port < 1 || port > 65535) {
				err = confBadValue(key, "must be in range 1...65535")
			}
			if err != nil {
				return err
			}

			Conf.HttpMaxPort = int(port)
		}

		key, err = section.GetKey("dns-sd")
		if key != nil {
			switch key.String() {
			case "enable":
				Conf.DnsSdEnable = true
			case "disable":
				Conf.DnsSdEnable = false
			default:
				return confBadValue(key, "must be enable or disable")
			}
		}
	}

	// Validate configuration
	if Conf.HttpMinPort >= Conf.HttpMaxPort {
		return errors.New("http-min-port must be less that http-max-port")
	}

	return nil
}
