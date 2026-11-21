/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Miscellaneous formatters
 */

package main

import (
	"fmt"
	"time"
)

// fmtDuration formats time.Duration for logging.
func fmtDuration(d time.Duration) string {
	d += time.Millisecond / 2
	sec := d / time.Second
	msec := (d % time.Second) / time.Millisecond
	return fmt.Sprintf("%d.%3.3d", sec, msec)
}
