/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Demonization
 */

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"unicode"
)

// #include <unistd.h>
import "C"

// CloseStdInOutErr closes stdin/stdout/stderr handles
func CloseStdInOutErr() error {
	nul, err := syscall.Open(os.DevNull, syscall.O_RDONLY, 0644)
	if err != nil {
		return fmt.Errorf("Open %q: %s", os.DevNull, err)
	}

	defer syscall.Close(nul)

	// Note, syscall.Dup2 is not implemented on old Go
	// versions for ARM64 Linux. So we use C.dup2 as a
	// portable workaround
	C.dup2(C.int(nul), 0)
	C.dup2(C.int(nul), 1)
	C.dup2(C.int(nul), 2)

	return nil
}

// Daemon runs ipp-usb program in background
func Daemon() error {
	// Create stdout/stderr pipes
	rstdout, wstdout, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("pipe(): %s", err)
	}

	rstderr, wstderr, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("pipe(): %s", err)
	}

	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("Open %q: %s", os.DevNull, err)
	}

	// Initialize process attributes
	attr := &os.ProcAttr{
		Files: []*os.File{devnull, wstdout, wstderr},
		Sys: &syscall.SysProcAttr{
			Setsid: true,
		},
	}

	// Initialize process arguments
	args := []string{}
	for _, arg := range os.Args {
		if arg != "-bg" {
			args = append(args, arg)
		}
	}

	// Start new process
	proc, err := os.StartProcess(PathExecutableFile, args, attr)
	if err != nil {
		return err
	}

	// Collect its initialization output
	wstdout.Close()
	wstderr.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	io.Copy(stdout, rstdout)
	io.Copy(stderr, rstderr)

	if stdout.Len() != 0 {
		os.Stdout.Write(stdout.Bytes())
	}

	// Check for an error
	if stderr.Len() > 0 {
		s := strings.TrimFunc(stderr.String(), unicode.IsSpace)
		proc.Kill() // Just in case
		return errors.New(s)
	}

	proc.Release()

	return nil

}
