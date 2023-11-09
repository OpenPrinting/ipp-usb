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
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	// ConfFileName defines a name of ipp-usb configuration file
	ConfFileName = "ipp-usb.conf"
)

// Configuration represents a program configuration
type Configuration struct {
	HTTPMinPort        int            // Starting port number for HTTP to bind to
	HTTPMaxPort        int            // Ending port number for HTTP to bind to
	DNSSdEnable        bool           // Enable DNS-SD advertising
	LoopbackOnly       bool           // Use only loopback interface
	IPV6Enable         bool           // Enable IPv6 advertising
	ConfAuthUID        []*AuthUIDRule // [auth uid], parsed
	LogDevice          LogLevel       // Per-device LogLevel mask
	LogMain            LogLevel       // Main log LogLevel mask
	LogConsole         LogLevel       // Console  LogLevel mask
	LogMaxFileSize     int64          // Maximum log file size
	LogMaxBackupFiles  uint           // Count of files preserved during rotation
	LogAllPrinterAttrs bool           // Get *all* printer attrs, for logging
	ColorConsole       bool           // Enable ANSI colors on console
	Quirks             QuirksSet      // Device quirks
}

// Conf contains a global instance of program configuration
var Conf = Configuration{
	HTTPMinPort:        60000,
	HTTPMaxPort:        65535,
	DNSSdEnable:        true,
	LoopbackOnly:       true,
	IPV6Enable:         true,
	ConfAuthUID:        nil,
	LogDevice:          LogDebug,
	LogMain:            LogDebug,
	LogConsole:         LogDebug,
	LogMaxFileSize:     256 * 1024,
	LogMaxBackupFiles:  5,
	LogAllPrinterAttrs: false,
	ColorConsole:       true,
}

// ConfLoad loads the program configuration
func ConfLoad() error {
	// Obtain path to executable directory
	exepath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("conf: %s", err)
	}

	exepath = filepath.Dir(exepath)

	// Build list of configuration files
	files := []string{
		filepath.Join(PathConfDir, ConfFileName),
		filepath.Join(exepath, ConfFileName),
	}

	// Load file by file
	for _, file := range files {
		err = confLoadInternal(file)
		if err != nil {
			return err
		}
	}

	// Load quirks
	quirksDirs := []string{
		PathQuirksDir,
		PathConfQuirksDir,
		filepath.Join(exepath, "ipp-usb-quirks"),
	}

	if err == nil {
		Conf.Quirks, err = LoadQuirksSet(quirksDirs...)
	}

	return err
}

// Load the program configuration -- internal version
func confLoadInternal(path string) error {
	// Open configuration file
	ini, err := OpenIniFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return err
	}

	defer ini.Close()

	// Extract options
	for err == nil {
		var rec *IniRecord
		rec, err = ini.Next()
		if err != nil {
			break
		}

		switch {
		case confMatchName(rec.Section, "network"):
			switch {
			case confMatchName(rec.Key, "http-min-port"):
				err = rec.LoadIPPort(&Conf.HTTPMinPort)
			case confMatchName(rec.Key, "http-max-port"):
				err = rec.LoadIPPort(&Conf.HTTPMaxPort)
			case confMatchName(rec.Key, "dns-sd"):
				err = rec.LoadNamedBool(&Conf.DNSSdEnable, "disable", "enable")
			case confMatchName(rec.Key, "interface"):
				err = rec.LoadNamedBool(&Conf.LoopbackOnly, "all", "loopback")
			case confMatchName(rec.Key, "ipv6"):
				err = rec.LoadNamedBool(&Conf.IPV6Enable, "disable", "enable")
			}

		case confMatchName(rec.Section, "auth uid"):
			err = rec.LoadAuthUIDRules(&Conf.ConfAuthUID)

		case confMatchName(rec.Section, "logging"):
			switch {
			case confMatchName(rec.Key, "device-log"):
				err = rec.LoadLogLevel(&Conf.LogDevice)
			case confMatchName(rec.Key, "main-log"):
				err = rec.LoadLogLevel(&Conf.LogMain)
			case confMatchName(rec.Key, "console-log"):
				err = rec.LoadLogLevel(&Conf.LogConsole)
			case confMatchName(rec.Key, "console-color"):
				err = rec.LoadNamedBool(&Conf.ColorConsole, "disable", "enable")
			case confMatchName(rec.Key, "max-file-size"):
				err = rec.LoadSize(&Conf.LogMaxFileSize)
			case confMatchName(rec.Key, "max-backup-files"):
				err = rec.LoadUint(&Conf.LogMaxBackupFiles)
			case confMatchName(rec.Key, "get-all-printer-attrs"):
				err = rec.LoadBool(&Conf.LogAllPrinterAttrs)
			}
		}
	}

	if err != nil && err != io.EOF {
		return err
	}

	// Validate configuration
	if Conf.HTTPMinPort >= Conf.HTTPMaxPort {
		return errors.New("http-min-port must be less that http-max-port")
	}

	return nil
}

// confMatchName tells if section or key name matches
// the pattern
//   - match is case-insensitive
//   - difference in amount of free space is ignored
//   - leading and trailing space is ignored
func confMatchName(name, pattern string) bool {
	name = strings.TrimSpace(name)
	pattern = strings.TrimSpace(pattern)

	for name != "" && pattern != "" {
		c1 := rune(name[0])
		c2 := rune(pattern[0])

		switch {
		case unicode.IsSpace(c1):
			if !unicode.IsSpace(c2) {
				return false
			}

			name = strings.TrimSpace(name)
			pattern = strings.TrimSpace(pattern)

		case c1 == c2:
			name = name[1:]
			pattern = pattern[1:]

		default:
			return false
		}
	}

	return true
}
