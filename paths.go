/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Common paths
 */

package main

const (
	// PathConfDir defines path to configuration directory
	PathConfDir = "/etc/ipp-usb"

	// PathQuirksDir defines path to quirks files
	PathQuirksDir = "/usr/share/ipp-usb/quirks"

	// PathProgState defines path to program state directory
	PathProgState = "/var/ipp-usb"

	// PathLockDir defines path to directory that contains lock files
	PathLockDir = PathProgState + "/lock"

	// PathLockFile defines path to lock file
	PathLockFile = PathLockDir + "/ipp-usb.lock"

	// PathProgStateDev defines path to directory where per-device state
	// files are saved to
	PathProgStateDev = PathProgState + "/dev"

	// PathLogDir defines path to log directory
	PathLogDir = "/var/log/ipp-usb"

	// PathLogFile defines path to the main log file
	PathLogFile = PathLogDir + "/main.log"
)
