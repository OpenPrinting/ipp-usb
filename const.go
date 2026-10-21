/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Configuration constants
 */

package main

import (
	"time"
)

const (
	// DevInitTimeout specifies how much time to wait for
	// device initialization
	DevInitTimeout = 5 * time.Second

	// DevShutdownTimeout specifies how much time to wait for
	// device graceful shutdown
	DevShutdownTimeout = 5 * time.Second

	// DevInitRetryInterval specifies the retry interval for
	// failed device initialization
	DevInitRetryInterval = 2 * time.Second

	// DNSSdRetryInterval specifies the retry interval in a case
	// of failed DNS-SD operation
	DNSSdRetryInterval = 2 * time.Second

	// USBReadBufferSize defines buffer size of USB bulk reads
	// from the printer.
	//
	// Some versions of Linux kernel don't allow bulk transfers to
	// be larger that 16kb per URB, and libusb uses some smart-ass
	// mechanism to avoid this limitation.
	//
	// This mechanism seems not to work very reliable on Raspberry Pi
	// (see #3 for details). So just limit bulk reads to 16kb
	//
	// Note also, this size must be multiple of USB packet size,
	// which is 1024, if USB 3.0 is considered.
	USBReadBufferSize = 16384
)
