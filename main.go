/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * The main function
 */

package main

import (
	"bytes"
	"fmt"
	"os"
	"sort"
)

const usageText = `Usage:
    %s mode [options]

Modes are:
    standalone  - run forever, automatically discover IPP-over-USB
                  devices and serve them all
    udev        - like standalone, but exit when last IPP-over-USB
                  device is disconnected
    debug       - logs duplicated on console, -bg option is
                  ignored
    check       - check configuration and exit
    status      - print ipp-usb status and exit

Options are
    -bg         - run in background (ignored in debug mode)
`

// RunMode represents the program run mode
type RunMode int

// Run modes:
//
//	RunStandalone - run forever, automatically discover IPP-over-USB
//	                devices and serve them all
//	RunUdev       - like RunStandalone, but exit when last IPP-over-USB
//	                device is disconnected
//	RunDebug      - logs duplicated on console, -bg option is ignored
//	RunCheck      - check configuration and exit
//	RunStatus     - print ipp-usb status and exit
const (
	RunDefault RunMode = iota
	RunStandalone
	RunUdev
	RunDebug
	RunCheck
	RunStatus
)

// String returns RunMode name
func (m RunMode) String() string {
	switch m {
	case RunDefault:
		return "default"
	case RunStandalone:
		return "standalone"
	case RunUdev:
		return "udev"
	case RunDebug:
		return "debug"
	case RunCheck:
		return "check"
	case RunStatus:
		return "status"
	}

	return fmt.Sprintf("unknown (%d)", int(m))
}

// RunParameters represents the program run parameters
type RunParameters struct {
	Mode       RunMode // Run mode
	Background bool    // Run in background
}

// usage prints detailed usage and exits
func usage() {
	fmt.Printf(usageText, os.Args[0])
	os.Exit(0)
}

// usage_error prints usage error and exits
func usageError(format string, args ...interface{}) {
	if format != "" {
		fmt.Printf(format+"\n", args...)
	}

	fmt.Printf("Try %s -h for more information\n", os.Args[0])
	os.Exit(1)
}

// parseArgv parses program parameters. In a case of usage error,
// it prints a error message and exits
func parseArgv() (params RunParameters) {
	// Catch panics to log
	defer func() {
		v := recover()
		if v != nil {
			Log.Panic(v)
		}
	}()

	// For now, default mode is debug mode. It may change in a future
	params.Mode = RunDebug

	modes := 0
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-h", "-help", "--help":
			usage()
		case "standalone":
			params.Mode = RunStandalone
			modes++
		case "udev":
			params.Mode = RunUdev
			modes++
		case "debug":
			params.Mode = RunDebug
			modes++
		case "check":
			params.Mode = RunCheck
			modes++
		case "status":
			params.Mode = RunStatus
			modes++
		case "-bg":
			params.Background = true
		default:
			usageError("Invalid argument %s", arg)
		}
	}

	if modes > 1 {
		usageError("Conflicting run modes")
	}

	if params.Mode == RunDebug {
		params.Background = false
	}

	return
}

// printStatus prints status of running ipp-usb daemon, if any
func printStatus() {
	// Fetch status
	text, err := StatusRetrieve()

	if err != nil {
		InitLog.Info(0, "%s", err)
		return
	}

	// Split into lines
	text = bytes.Trim(text, "\n")
	lines := bytes.Split(text, []byte("\n"))

	// Strip empty lines at the end
	for len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[0 : len(lines)-1]
	}

	// Write to log, line by line
	for _, line := range lines {
		InitLog.Info(0, "%s", line)
	}
}

// The main function
func main() {
	var err error

	// Parse arguments
	params := parseArgv()

	// Load configuration file
	err = ConfLoad()
	InitLog.Check(err)

	// Setup logging
	if params.Mode != RunDebug &&
		params.Mode != RunCheck &&
		params.Mode != RunStatus {
		Console.ToNowhere()
	} else if Conf.ColorConsole {
		Console.ToColorConsole()
	}

	Log.SetLevels(Conf.LogMain)
	Console.SetLevels(Conf.LogConsole)
	Log.Cc(Console)

	// In RunCheck mode, list IPP-over-USB devices
	if params.Mode == RunCheck {
		// If we are here, configuration is OK
		InitLog.Info(0, "Configuration files: OK")

		var descs map[UsbAddr]UsbDeviceDesc
		err = UsbInit(true)
		if err == nil {
			descs, err = UsbGetIppOverUsbDeviceDescs()
		}

		if err != nil {
			InitLog.Info(0, "Can't read list of USB devices: %s", err)
		} else if descs == nil || len(descs) == 0 {
			InitLog.Info(0, "No IPP over USB devices found")
		} else {
			// Repack into the sorted list
			var list []UsbDeviceDesc
			var buf bytes.Buffer

			for _, desc := range descs {
				list = append(list, desc)
			}
			sort.Slice(list, func(i, j int) bool {
				return list[i].UsbAddr.Less(list[j].UsbAddr)
			})

			InitLog.Info(0, "IPP over USB devices:")
			InitLog.Info(0, " Num  Device              Vndr:Prod  Model")
			for i, dev := range list {
				buf.Reset()
				fmt.Fprintf(&buf, "%3d. %s", i+1, dev.UsbAddr)
				if info, err := dev.GetUsbDeviceInfo(); err == nil {
					fmt.Fprintf(&buf, "  %4.4x:%.4x  %q",
						info.Vendor, info.Product, info.MfgAndProduct)
				}

				InitLog.Info(0, " %s", buf.String())
			}
		}
	}

	// In RunStatus mode, print ipp-usb status, and we are done
	if params.Mode == RunStatus {
		printStatus()
		os.Exit(0)
	}

	// Check user privileges
	if os.Geteuid() != 0 {
		InitLog.Exit(0, "This program requires root privileges")
	}

	// If mode is "check", we are done
	if params.Mode == RunCheck {
		os.Exit(0)
	}

	// If background run is requested, it's time to fork
	if params.Background {
		err = Daemon()
		InitLog.Check(err)
		os.Exit(0)
	}

	// Prevent multiple copies of ipp-usb from being running
	// in a same time
	os.MkdirAll(PathLockDir, 0755)
	lock, err := os.OpenFile(PathLockFile,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	InitLog.Check(err)
	defer lock.Close()

	err = FileLock(lock, FileLockNoWait)
	if err == ErrLockIsBusy {
		if params.Mode == RunUdev {
			// It's not an error in udev mode
			os.Exit(0)
		} else {
			InitLog.Exit(0, "ipp-usb already running")
		}
	}
	InitLog.Check(err)

	// Write to log that we are here
	if params.Mode != RunCheck && params.Mode != RunStatus {
		Log.Info(' ', "===============================")
		Log.Info(' ', "ipp-usb started in %q mode, pid=%d",
			params.Mode, os.Getpid())
		defer Log.Info(' ', "ipp-usb finished")
	}

	// Initialize USB
	err = UsbInit(false)
	InitLog.Check(err)

	// Close stdin/stdout/stderr, unless running in debug mode
	if params.Mode != RunDebug {
		err = CloseStdInOutErr()
		InitLog.Check(err)
	}

	// Run PnP manager
	for {
		exitReason := PnPStart(params.Mode == RunUdev)

		// The following race is possible here:
		// 1) last device disappears, ipp-usb is about to exit
		// 2) new device connected, new ipp-usb started
		// 3) new ipp-usp exits, because lock is still held
		//    by the old ipp-usb
		// 4) old ipp-usb finally exits
		//
		// So after releasing a lock, we rescan for IPP-over-USB
		// devices, and if something was found, we try to reacquire
		// the lock, and if it succeeds, we continue to serve
		// these devices instead of exiting
		if exitReason == PnPIdle && params.Mode == RunUdev {
			err = FileUnlock(lock)
			Log.Check(err)

			if UsbCheckIppOverUsbDevices() &&
				FileLock(lock, FileLockNoWait) == nil {
				Log.Info(' ', "New IPP-over-USB device found")
				continue
			}
		}

		break
	}
}
