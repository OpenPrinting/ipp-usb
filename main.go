/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * The main function
 */

package main

import (
	"flag"
	"fmt"
	"os"
)

// usage_error prints usage error and exits
func usage_error(format string, args ...interface{}) {
	if format != "" {
		fmt.Printf(format+"\n", args...)
	}

	fmt.Printf("Try %s -h for more information\n", os.Args[0])
	os.Exit(1)
}

// The main function
func main() {
	// Parse arguments
	flagset := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flagset.SetOutput(os.Stdout)
	flagset.Usage = func() {
	}

	lport := flagset.Int("l", 60000, "HTTP port to listen to")

	err := flagset.Parse(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			fmt.Printf("Usage of %s:\n", os.Args[0])
			flagset.PrintDefaults()
		} else {
			usage_error("")
		}
		return
	}

	// Verify arguments
	if *lport < 1 || *lport > 65535 {
		usage_error(`invalid value "%d" for flag -l`, *lport)
	}
	if flagset.NArg() > 0 {
		usage_error("Invalid argument %s", flagset.Args()[0])
	}

	// Check user privileges
	if os.Geteuid() != 0 {
		Log.Exit(0, "This program requires root privileges")
	}

	// Prevent multiple copies of ipp-usb from being running
	// in a same time
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

	// Load configuration file
	err = ConfLoad()
	Log.Check(err)

	// Run PnP manager
	PnPStart()
}
