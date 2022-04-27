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
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// ConfFileName defines a name of ipp-usb configuration file
	ConfFileName = "ipp-usb.conf"
)

// Configuration represents a program configuration
type Configuration struct {
	HTTPMinPort       int       // Starting port number for HTTP to bind to
	HTTPMaxPort       int       // Ending port number for HTTP to bind to
	DNSSdEnable       bool      // Enable DNS-SD advertising
	LoopbackOnly      bool      // Use only loopback interface
	IPV6Enable        bool      // Enable IPv6 advertising
	LogDevice         LogLevel  // Per-device LogLevel mask
	LogMain           LogLevel  // Main log LogLevel mask
	LogConsole        LogLevel  // Console  LogLevel mask
	LogMaxFileSize    int64     // Maximum log file size
	LogMaxBackupFiles uint      // Count of files preserved during rotation
	ColorConsole      bool      // Enable ANSI colors on console
	Quirks            QuirksSet // Device quirks
}

// Conf contains a global instance of program configuration
var Conf = Configuration{
	HTTPMinPort:       60000,
	HTTPMaxPort:       65535,
	DNSSdEnable:       true,
	LoopbackOnly:      true,
	IPV6Enable:        true,
	LogDevice:         LogDebug,
	LogMain:           LogDebug,
	LogConsole:        LogDebug,
	LogMaxFileSize:    256 * 1024,
	LogMaxBackupFiles: 5,
	ColorConsole:      true,
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
			return fmt.Errorf("conf: %s", err)
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

// Create "bad value" error
func confBadValue(rec *IniRecord, format string, args ...interface{}) error {
	return fmt.Errorf(rec.Key+": "+format, args...)
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

		switch rec.Section {
		case "network":
			switch rec.Key {
			case "http-min-port":
				err = confLoadIPPortKey(&Conf.HTTPMinPort, rec)
			case "http-max-port":
				err = confLoadIPPortKey(&Conf.HTTPMaxPort, rec)
			case "dns-sd":
				err = confLoadBinaryKey(&Conf.DNSSdEnable, rec, "disable", "enable")
			case "interface":
				err = confLoadBinaryKey(&Conf.LoopbackOnly, rec, "all", "loopback")
			case "ipv6":
				err = confLoadBinaryKey(&Conf.IPV6Enable, rec, "disable", "enable")
			}
		case "logging":
			switch rec.Key {
			case "device-log":
				err = confLoadLogLevelKey(&Conf.LogDevice, rec)
			case "main-log":
				err = confLoadLogLevelKey(&Conf.LogMain, rec)
			case "console-log":
				err = confLoadLogLevelKey(&Conf.LogConsole, rec)
			case "console-color":
				err = confLoadBinaryKey(&Conf.ColorConsole, rec, "disable", "enable")
			case "max-file-size":
				err = confLoadSizeKey(&Conf.LogMaxFileSize, rec)
			case "max-backup-files":
				err = confLoadUintKey(&Conf.LogMaxBackupFiles, rec)
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

// Load IP port key
func confLoadIPPortKey(out *int, rec *IniRecord) error {
	port, err := strconv.Atoi(rec.Value)
	if err == nil && (port < 1 || port > 65535) {
		err = confBadValue(rec, "must be in range 1...65535")
	}
	if err != nil {
		return err
	}

	*out = int(port)
	return nil
}

// Load the binary key
func confLoadBinaryKey(out *bool, rec *IniRecord, vFalse, vTrue string) error {
	switch rec.Value {
	case vFalse:
		*out = false
		return nil
	case vTrue:
		*out = true
		return nil
	default:
		return confBadValue(rec, "must be %s or %s", vFalse, vTrue)
	}
}

// Load LogLevel key
func confLoadLogLevelKey(out *LogLevel, rec *IniRecord) error {
	var mask LogLevel
	for _, s := range strings.Split(rec.Value, ",") {
		s = strings.TrimSpace(s)
		switch s {
		case "":
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
		case "trace-usb":
			mask |= LogTraceUSB | LogDebug | LogInfo | LogError
		case "all", "trace-all":
			mask |= LogAll & ^LogTraceUSB
		default:
			return confBadValue(rec, "invalid log level %q", s)
		}
	}

	*out = mask
	return nil
}

// Load QuirksResetMethod key
func confLoadQuirksResetMethodKey(out *QuirksResetMethod, rec *IniRecord) error {
	switch rec.Value {
	case "none":
		*out = QuirksResetNone
		return nil
	case "soft":
		*out = QuirksResetSoft
		return nil
	case "hard":
		*out = QuirksResetHard
		return nil
	default:
		return confBadValue(rec, "must be none, soft or hard")
	}
}

// Load size key
func confLoadSizeKey(out *int64, rec *IniRecord) error {
	units := uint64(1)

	if l := len(rec.Value); l > 0 {
		switch rec.Value[l-1] {
		case 'k', 'K':
			units = 1024
		case 'm', 'M':
			units = 1024 * 1024
		}

		if units != 1 {
			rec.Value = rec.Value[:l-1]
		}
	}

	sz, err := strconv.ParseUint(rec.Value, 10, 64)
	if err != nil {
		return confBadValue(rec, "%q: invalid size", rec.Value)
	}

	if sz > uint64(math.MaxInt64/units) {
		return confBadValue(rec, "size too large")
	}

	*out = int64(sz * units)
	return nil
}

// Load unsigned integer key
func confLoadUintKey(out *uint, rec *IniRecord) error {
	num, err := strconv.ParseUint(rec.Value, 10, 0)
	if err != nil {
		return confBadValue(rec, "%q: invalid number", rec.Value)
	}

	*out = uint(num)
	return nil
}

// Load unsigned integer key within the range
func confLoadUintKeyRange(out *uint, rec *IniRecord, min, max uint) error {
	var val uint
	err := confLoadUintKey(&val, rec)
	if err == nil && (val < min || val > max) {
		err = confBadValue(rec, "must be in range %d...%d", min, max)
	}

	if err == nil {
		*out = val
	}

	return err
}
