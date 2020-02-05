/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * The main function
 */

package main

import (
	"fmt"
	"os"
)

const usageText = `Usage:
    %s mode [options]

Modes are:
    standalone  - run forever, automatically discover IPP-over-USB
                  devices and serve them all
    udev        - like standalone, but exit when last IPP-over-USB
                  device is disconnected
    debug       - logs duplicated on console, -bg option is
                  ignored
    check       - check configuration and exit

Options are
    -bg         - run in background (ignored in debug mode)
`

// RunMode represents the program run mode
type RunMode int

const (
	RunDefault RunMode = iota
	RunStandalone
	RunUdev
	RunDebug
	RunCheck
)

// RunParameters represents the program run parameters
type RunParameters struct {
	Mode       RunMode // Run mode
	Background bool    // Run in background
}

// usage prints detailed usage and exits
func usage() {
	fmt.Printf(usageText, os.Args[0])
	os.Exit(0)
}

// usage_error prints usage error and exits
func usageError(format string, args ...interface{}) {
	if format != "" {
		fmt.Printf(format+"\n", args...)
	}

	fmt.Printf("Try %s -h for more information\n", os.Args[0])
	os.Exit(1)
}

// parseArgv parses program parameters. In a case of usage error,
// it prints a error message and exits
func parseArgv() (params RunParameters) {
	modes := 0
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-h", "-help", "--help":
			usage()
		case "standalone":
			params.Mode = RunStandalone
			modes++
		case "udev":
			params.Mode = RunUdev
			modes++
		case "debug":
			params.Mode = RunDebug
			modes++
		case "check":
			params.Mode = RunCheck
			modes++
		case "-bg":
			params.Background = true
		default:
			usageError("Invalid argument %s", arg)
		}
	}

	if modes > 1 {
		usageError("Conflicting run modes")
	}

	return
}

// The main function
func main() {
	var err error

	// Parse arguments
	params := parseArgv()

	// Check user privileges
	if os.Geteuid() != 0 {
		Log.Exit(0, "This program requires root privileges")
	}

	// Prevent multiple copies of ipp-usb from being running
	// in a same time
	if params.Mode != RunCheck {
		os.MkdirAll(PathLockDir, 0755)
		lock, err := os.OpenFile(PathLockFile,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		Log.Check(err)
		defer lock.Close()

		err = FileLock(lock, true, false)
		if err == ErrLockIsBusy {
			Log.Exit(0, "ipp-usb already running")
		}
		Log.Check(err)
	}

	// Load configuration file
	err = ConfLoad()
	Log.Check(err)

	// Initialize USB
	err = UsbInit()
	Log.Check(err)

	// Run PnP manager
	PnPStart(params.Mode == RunUdev)
}
