//go:build darwin || dragonfly || freebsd || linux || nacl || netbsd || openbsd || solaris
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
#include <sys/file.h>

static inline int do_flock (int fd, int op) {
    int rc = flock(fd, op);
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
	// FileLockWait command used to lock the file; wait if it is busy
	FileLockWait = C.LOCK_EX

	// FileLockNoWait command used to lock the file without wait.
	// If file is busy it fails with ErrLockIsBusy error
	FileLockNoWait = C.LOCK_EX | C.LOCK_NB

	// FileLockUnlock command used to unlock the file
	FileLockUnlock = C.LOCK_UN
)

// FileLock manages file lock
func FileLock(file *os.File, cmd FileLockCmd) error {
	rc := C.do_flock(C.int(file.Fd()), C.int(cmd))
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
