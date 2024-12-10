/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Common errors
 */

package main

import (
	"errors"
)

// Error values for ipp-usb
var (
	ErrLockIsBusy   = errors.New("Lock is busy")
	ErrNoMemory     = errors.New("Not enough memory")
	ErrShutdown     = errors.New("Shutdown requested")
	ErrBlackListed  = errors.New("Device is blacklisted")
	ErrInitTimedOut = errors.New("Device initialization timed out")
	ErrUnusable     = errors.New("Device doesn't implement print or scan service")
	ErrNoIppUsb     = errors.New("ipp-usb daemon not running")
	ErrAccess       = errors.New("Access denied")
	ErrPartialInit  = errors.New("Some parts of device not ready yet")
)
