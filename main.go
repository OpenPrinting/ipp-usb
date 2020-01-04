package main

import (
	"flag"
	"fmt"
)

// ----- Flags (program options) -----
var (
	flag_cport   = flag.Int("c", 60000, "HTTP port to connect to")
	flag_lport   = flag.Int("l", 60001, "HTTP port to listen to")
	flag_timeout = flag.Int("t", 5, "Idle connection timeout, seconds")
)

// The main function
func main() {
	// Parse arguments
	flag.Parse()
	if *flag_lport < 1 || *flag_lport > 65535 {
		log_usage("Invalid value for option -l")
	}
	if *flag_cport < 1 || *flag_cport > 65535 {
		log_usage("Invalid value for option -c")
	}
	if flag.NArg() > 0 {
		log_usage("Invalid argument %s", flag.Args()[0])
	}

	// Initialize USB
	err := usbInit()
	log_check(err)

	// Create HTTP server
	addr := fmt.Sprintf("localhost:%d", *flag_lport)
	err = HttpListenAndServe(addr)
	if err != nil {
		log_exit("%s", err)
	}
}
