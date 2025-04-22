/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Common paths
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Effective paths, may be altered with the command-line options:
var (
	// Control socket
	PathControlSocket = DefaultPathControlSocket

	// Lock file
	PathLockFile = DefaultPathLockFile

	// Directory for per-device logs
	PathLogDir = DefaultPathLogDir

	// Directory that contains per-device state files
	PathDevStateDir = DefaultPathDevStateDir

	// Path to the program's executable file.
	// Initialized by PathInit()
	PathExecutableFile string

	// Path to the directory that contains the executable file.
	// Initialized by PathInit()
	PathExecutableDir string

	// List of configuration directories.
	// Initialized by PathInit():
	//   DefaultPathConfDir + ":" + PathExecutableDir
	PathConfDirList string

	// List of quirks directories.
	// Initialized by PathInit():
	//   DefaultPathLocalQuirksDir + ":" +
	//   DefaultPathGlobalQuirksDir + ":" +
	//   filepath.Join(PathExecutableDir, "ipp-usb-quirks")
	PathQuirksDirList string
)

// Default paths:
const (
	// DefaultPathConfDir defines path to configuration directory
	DefaultPathConfDir = "/etc/ipp-usb"

	// DefaultPathLocalQuirksDir defines path to locally administered
	// quirks files
	DefaultPathLocalQuirksDir = "/etc/ipp-usb/quirks"

	// DefaultPathGlobalQuirksDir defines path to the "global"
	// quirks files, i.e., files that comes with the ipp-usb package
	DefaultPathGlobalQuirksDir = "/usr/share/ipp-usb/quirks"

	// DefaultPathProgState defines path to program state directory
	DefaultPathProgState = "/var/ipp-usb"

	// DefaultPathLockDir defines path to directory that contains
	// lock files
	DefaultPathLockDir = DefaultPathProgState + "/lock"

	// DefaultPathLockFile defines path to lock file
	DefaultPathLockFile = DefaultPathLockDir + "/ipp-usb.lock"

	// DefaultPathControlSocket defines path to the control socket
	DefaultPathControlSocket = DefaultPathProgState + "/ctrl"

	// DefaultPathDevStateDir defines path to directory where
	// per-device state files are saved to
	DefaultPathDevStateDir = DefaultPathProgState + "/dev"

	// DefaultPathLogDir defines path to log directory
	DefaultPathLogDir = "/var/log/ipp-usb"
)

// PathsInit initializes paths handling.
func PathsInit() error {
	// Initialize PathExecutableFile and PathExecutableDir
	var err error
	PathExecutableFile, err = os.Executable()
	if err != nil {
		err = fmt.Errorf(
			"Error getting path to the executable file: %s", err)
		return err
	}

	PathExecutableDir = filepath.Dir(PathExecutableFile)

	// Initialize derived paths
	PathConfDirList =
		strings.Join(
			[]string{
				DefaultPathConfDir,
				PathExecutableDir,
			},
			string(filepath.ListSeparator),
		)

	PathQuirksDirList =
		strings.Join(
			[]string{
				DefaultPathLocalQuirksDir,
				DefaultPathGlobalQuirksDir,
				filepath.Join(PathExecutableDir,
					"ipp-usb-quirks"),
			},
			string(filepath.ListSeparator),
		)

	return nil
}

// MakeDirectory creates a directory, specified by the path,
// along with any necessary parents.
//
// Possible errors are not checked here, as there are many reasons
// while it can fail (most likely: directory already exists). Instead,
// error checking is implemented when we try to use the resulting directory.
func MakeDirectory(path string) {
	os.MkdirAll(path, 0755)
}

// MakeParentDirectory creates a parent directory for the specified path,
// along with any necessary parents.
//
// Possible errors are not checked here, as there are many reasons
// while it can fail (most likely: directory already exists). Instead,
// error checking is implemented when we try to use the resulting directory.
func MakeParentDirectory(path string) {
	parent := filepath.Dir(path)
	if parent != "" && parent != "." {
		MakeDirectory(parent)
	}
}
