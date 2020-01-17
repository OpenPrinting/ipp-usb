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
	"os"
)

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
			flag.CommandLine.Usage()
			flagset.PrintDefaults()
		} else {
			log_usage("")
		}
		return
	}

	// Verify arguments
	if *lport < 1 || *lport > 65535 {
		log_usage(`invalid value "%d" for flag -l`, *lport)
	}
	if flagset.NArg() > 0 {
		log_usage("Invalid argument %s", flagset.Args()[0])
	}

	// Load configuration file
	err = ConfLoad()
	log_check(err)

	// Run PnP manager
	PnPStart()
}
