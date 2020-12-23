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
	DNSSdRetryInterval = 1 * time.Second
)
