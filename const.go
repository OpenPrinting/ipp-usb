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
	//
	// Note, EPSON ET 4750 Series takes A LOT of time to
	// respond for the first IPP query, and it needs to be
	// better investigated, see logs in #17 for details:
	//    https://github.com/OpenPrinting/ipp-usb/issues/17
	//
	// For now, just set timeout big enough
	DevInitTimeout = 20 * time.Second

	// DevShutdownTimeout specifies how much time to wait for
	// device graceful shutdown
	DevShutdownTimeout = 5 * time.Second

	// DevInitRetryInterval specifies the retry interval for
	// failed device initialization
	DevInitRetryInterval = 2 * time.Second

	// DNSSdRetryInterval specifies the retry interval in a case
	// of failed DNS-SD operation
	DNSSdRetryInterval = 1 * time.Second
)
