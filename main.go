package main

import (
	"flag"
	"fmt"
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
	}

	// Verify arguments
	if *lport < 1 || *lport > 65535 {
		log_usage(`invalid value "%d" for flag -l`, *lport)
	}
	if flagset.NArg() > 0 {
		log_usage("Invalid argument %s", flagset.Args()[0])
	}

	// Initialize USB
	transport, err := NewUsbTransport()
	log_check(err)

	// Register in DNS-SD
	dnssdReg, err := DnsSdPublish()
	if err != nil {
		log_exit("DNS-SD: %s", err)
	}

	defer dnssdReg.Remove()

	// Create HTTP server
	addr := fmt.Sprintf("localhost:%d", *lport)
	err = HttpListenAndServe(addr, transport)
	if err != nil {
		log_exit("%s", err)
	}
}
