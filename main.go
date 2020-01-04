package main

import (
	"flag"
	"fmt"
)

// ----- Flags (program options) -----
var (
	flag_lport = flag.Int("l", 60000, "HTTP port to listen to")
)

// The main function
func main() {
	// Parse arguments
	flag.Parse()
	if *flag_lport < 1 || *flag_lport > 65535 {
		log_usage("Invalid value for option -l")
	}
	if flag.NArg() > 0 {
		log_usage("Invalid argument %s", flag.Args()[0])
	}

	// Initialize USB
	transport, err := NewUsbTransport()
	log_check(err)

	// Create HTTP server
	addr := fmt.Sprintf("localhost:%d", *flag_lport)
	err = HttpListenAndServe(addr, transport)
	if err != nil {
		log_exit("%s", err)
	}
}
