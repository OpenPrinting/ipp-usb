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

// In udev mode, systemd may start ipp-usb either when a compatible
// USB device is connected or when a control socket request is made.
//
// When started, ipp-usb cannot tell which of these events triggered
// its activation. Therefore, finding no connected devices immediately
// after startup does not necessarily mean that there is nothing to do:
// a device may have been disconnected shortly before startup, or
// systemd may have started ipp-usb to handle a control socket request.
//
// For this reason, ipp-usb does not exit immediately when no devices
// are connected, but waits for a while to allow pending control socket
// requests to arrive.
//
// The following parameters define how long the program will remain
// idle in udev mode before exiting.
//
//   - UdevStayIdleAfteCtrlsockInit specifies how long to wait after
//     the control socket has been initialized.
//   - UdevStayIdleAfterCtrlsockActivity specifies how long to wait
//     after the last control socket activity.
//
// The program exits only when both timeouts have expired.
const (
	UdevStayIdleAfteCtrlsockInit      = 15 * time.Second
	UdevStayIdleAfterCtrlsockActivity = 5 * time.Minute
)
