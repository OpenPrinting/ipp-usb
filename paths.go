/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Common paths
 */

package main

const (
	// Path to configuration directory
	PathConfDir = "/etc/ipp-usb"

	// Path to program state directory
	PathProgState = "/var/ipp-usb"

	// Path to directory that contains lock files
	PathLockDir = PathProgState + "/lock"

	// Path to lock file
	PathLockFile = PathLockDir + "/ipp-usb.lock"

	// Path to directory where per-device state is saved to
	PathProgStateDev = PathProgState + "/dev"
)
