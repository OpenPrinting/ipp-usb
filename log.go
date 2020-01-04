// Logging

package main

import (
	"fmt"
	"os"
)

// Print debug message
func log_debug(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...) + "\n"
	print(s)
}

// Print log message and exit
func log_exit(format string, args ...interface{}) {
	log_debug(format, args...)
	os.Exit(1)
}

// If error is not nil, print error message and exit
func log_check(err error) {
	if err != nil {
		log_exit(err.Error())
	}
}

// Print usage error and exit
func log_usage(format string, args ...interface{}) {
	log_debug(format, args...)
	log_debug("Try %s -h for more information", os.Args[0])
	os.Exit(1)
}
