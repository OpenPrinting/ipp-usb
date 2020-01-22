/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * File locking -- UNIX version
 */

package main

import (
	"os"
	"syscall"
)

//
// Lock the file
//
func FileLock(file *os.File, exclusive, wait bool) error {
	var how int

	if exclusive {
		how = syscall.LOCK_EX
	} else {
		how = syscall.LOCK_SH
	}

	if !wait {
		how |= syscall.LOCK_NB
	}

	err := syscall.Flock(int(file.Fd()), how)
	if err == syscall.Errno(syscall.EWOULDBLOCK) {
		err = ErrLockIsBusy
	}

	return err
}

//
// Unlock the file
//
func FileUnlock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
