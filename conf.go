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
	// ConfFileName defines a name of ipp-usb configuration file
	ConfFileName = "ipp-usb.conf"
)

// Configuration represents a program configuration
type Configuration struct {
	HTTPMinPort  int      // Starting port number for HTTP to bind to
	HTTPMaxPort  int      // Ending port number for HTTP to bind to
	DNSSdEnable  bool     // Enable DNS-SD advertising
	LoopbackOnly bool     // Use only loopback interface
	IPV6Enable   bool     // Enable IPv6 advertising
	LogDevice    LogLevel // Per-device LogLevel mask
	LogMain      LogLevel // Main log LogLevel mask
	LogConsole   LogLevel // Console  LogLevel mask
	ColorConsole bool     // Enable ANSI colors on console
}

// Conf contains a global instance of program configuration
var Conf = Configuration{
	HTTPMinPort:  60000,
	HTTPMaxPort:  65535,
	DNSSdEnable:  true,
	LoopbackOnly: true,
	IPV6Enable:   true,
	LogDevice:    LogDebug,
	LogMain:      LogDebug,
	LogConsole:   LogDebug,
	ColorConsole: true,
}

// ConfLoad loads the program configuration
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
		filepath.Join(PathConfDir, ConfFileName),
		filepath.Join(exepath, ConfFileName))

	if err != nil {
		return err
	}

	// Extract options
	if section, _ := inifile.GetSection("network"); section != nil {
		err = confLoadIPPortKey(&Conf.HTTPMinPort, section, "http-min-port")
		if err != nil {
			return err
		}

		err = confLoadIPPortKey(&Conf.HTTPMaxPort, section, "http-max-port")
		if err != nil {
			return err
		}

		err = confLoadBinaryKey(&Conf.DNSSdEnable, section,
			"dns-sd", "disable", "enable")
		if err != nil {
			return err
		}

		err = confLoadBinaryKey(&Conf.LoopbackOnly, section,
			"interface", "all", "loopback")
		if err != nil {
			return err
		}

		err = confLoadBinaryKey(&Conf.DNSSdEnable, section,
			"ipv6-sd", "disable", "enable")
		if err != nil {
			return err
		}

		err = confLoadBinaryKey(&Conf.IPV6Enable, section,
			"ipv6", "disable", "enable")
		if err != nil {
			return err
		}
	}

	if section, _ := inifile.GetSection("logging"); section != nil {
		err = confLoadLogLevelKey(&Conf.LogDevice, section, "device-log")
		if err != nil {
			return err
		}

		err = confLoadLogLevelKey(&Conf.LogMain, section, "main-log")
		if err != nil {
			return err
		}

		err = confLoadLogLevelKey(&Conf.LogConsole, section, "console-log")
		if err != nil {
			return err
		}

		err = confLoadBinaryKey(&Conf.ColorConsole, section,
			"console-color", "disable", "enable")
		if err != nil {
			return err
		}
	}

	// Validate configuration
	if Conf.HTTPMinPort >= Conf.HTTPMaxPort {
		return errors.New("http-min-port must be less that http-max-port")
	}

	return nil
}

// Load IP port key
func confLoadIPPortKey(out *int, section *ini.Section, name string) error {
	key, _ := section.GetKey(name)
	if key != nil {
		port, err := key.Uint()
		if err == nil && (port < 1 || port > 65535) {
			err = confBadValue(key, "must be in range 1...65535")
		}
		if err != nil {
			return err
		}

		*out = int(port)
	}

	return nil // Missed key is not error
}

// Load the binary key
func confLoadBinaryKey(out *bool,
	section *ini.Section, name, vFalse, vTrue string) error {

	key, _ := section.GetKey(name)
	if key != nil {
		switch key.String() {
		case vFalse:
			*out = false
			return nil
		case vTrue:
			*out = true
			return nil
		default:
			return confBadValue(key,
				"must be %s or %s", vFalse, vTrue)
		}
	}

	return nil // Missed key is not error
}

// Load LogLevel key
func confLoadLogLevelKey(out *LogLevel, section *ini.Section, name string) error {
	key, _ := section.GetKey(name)
	if key != nil {
		var mask LogLevel
		for _, s := range key.Strings(",") {
			switch s {
			case "error":
				mask |= LogError
			case "info":
				mask |= LogInfo | LogError
			case "debug":
				mask |= LogDebug | LogInfo | LogError
			case "trace-ipp":
				mask |= LogTraceIPP | LogDebug | LogInfo | LogError
			case "trace-escl":
				mask |= LogTraceESCL | LogDebug | LogInfo | LogError
			case "trace-http":
				mask |= LogTraceHTTP | LogDebug | LogInfo | LogError
			case "all", "trace-all":
				mask |= LogAll
			default:
				return confBadValue(key, "invalid log level %q", s)
			}
		}
		*out = mask
	}

	return nil
}
