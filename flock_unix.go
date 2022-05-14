// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris

/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * File locking -- UNIX version
 */

package main

/*
#include <errno.h>
#include <unistd.h>

static inline int do_lockf (int fd, int cmd, off_t len) {
    int rc = lockf(fd, cmd, len);
    if (rc < 0) {
        rc = -errno;
    }
    return rc;
}
*/
import "C"

import (
	"os"
	"syscall"
)

// FileLockCmd represents set of possible values for the
// FileLock argument
type FileLockCmd C.int

const (
	// Lock the file; wait if it is busy
	FileLockWait = C.F_LOCK

	// Lock the file; fail with ErrLockIsBusy if it is busy
	FileLockNoWait = C.F_TLOCK

	// Test the lock. Return immediately with ErrLockIsBusy
	// if file is busy or with the nil error, if file is not
	// busy. File locking state is not affected in both cases
	FileLockTest = C.F_TEST

	// Unlock the file
	FileLockUnlock = C.F_ULOCK
)

// FileLock manages file lock
func FileLock(file *os.File, cmd FileLockCmd) error {
	rc := C.do_lockf(C.int(file.Fd()), C.int(cmd), 0)
	if rc == 0 {
		return nil
	}

	var err error = syscall.Errno(-rc)
	switch err {
	case syscall.EACCES, syscall.EAGAIN:
		err = ErrLockIsBusy
	}

	return err
}

// FileUnlock releases file lock
func FileUnlock(file *os.File) error {
	return FileLock(file, FileLockUnlock)
}
