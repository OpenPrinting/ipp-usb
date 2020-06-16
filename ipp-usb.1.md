ipp-usb(1) -- Daemon for IPP over USB printer support
=====================================================

## DESCRIPTION

`ipp-usb` daemon enables driver-less printing and scanning on
USB-only AirPrint-compatible printers and MFPs.

It works by connecting to the device by USB using IPP-over-USB
protocol, and exposing the device to the network, including
DNS-SD (ZeroConf) advertising.

IPP printing, eSCL scanning and web console are fully supported.

## SYNOPSIS

### Usage:

`ipp-usb mode [options]`

### Modes are:

   * `standalone`:
     run forever, automatically discover IPP-over-USB
     devices and serve them all

   * `udev`:
     like standalone, but exit when last IPP-over-USB
     device is disconnected

   * `debug`:
     logs duplicated on console, -bg option is ignored

   * `check`:
     check configuration and exit

### Options are

   * `-bg`:
     run in background (ignored in debug mode)

## CONFIGURATION

`ipp-usb` searched for its configuration file in two places:
1. `/etc/ipp-usb/ipp-usb.conf`
2. `ipp-usb.conf` in the directory where executable file is located

Configuration file syntax is very similar to .INI files syntax.
It consist of named sections, and each section contains a set of
named variables. Comments are started from # or ; characters and
continues until end of line:

    # This is a comment
    [section 1]
    variable 1 = value 1  ; and another comment
    variable 2 = value 2

### Network parameters

Network parameters are all in the `[network]` section:

    [network]
      # TCP ports for HTTP will be automatically allocated in the following range
      http-min-port = 60000
      http-max-port = 65535

      # Enable or disable DNS-SD advertisement
      dns-sd = enable      # enable | disable

      # Network interface to use. Set to `all` if you want to expose you
      # printer to the local network. This way you can share your printer
      # with other computers in the network, as well as with iOS and Android
      # devices.
      interface = loopback # all | loopback

      # Enable or disable IPv6
      ipv6 = enable        # enable | disable

### Logging configuration

Logging parameters are all in the `[logging]` section:

    [logging]
      # device-log  - what logs are generated per device
      # main-log    - what common logs are generated
      # console-log - what of generated logs goes to console
      #
      # parameter contains a comma-separated list of
      # the following keywords:
      #   error     - error messages
      #   info      - informative messages
      #   debug     - debug messages
      #   trace-ipp, trace-escl, trace-http - very detailed per-protocol traces
      #   all       - all logs
      #   trace-all - alias to all
      #
      # Note, trace-* implies debug, debug implies info, info implies error
      device-log    = all
      main-log      = debug
      console-log   = debug

      # Log rotation parameters:
      #   log-file-size    - max log file before rotation. Use suffix M
      #                      for megabytes or K for kilobytes
      #   log-backup-files - how many backup files to preserve during rotation
      #
      max-file-size    = 256K
      max-backup-files = 5

      # Enable or disable ANSI colors on console
      console-color = enable # enable | disable


## FILES

   * `/etc/ipp-usb/ipp-usb.conf`:
     the daemon configuration file

   * `/var/log/ipp-usb/main.log`:
     the main log file

   * `/var/log/ipp-usb/<DEVICE>.log`:
     per-device log files

   * `/var/ipp-usb/dev/<DEVICE>.state`:
     device state (HTTP port allocation, DNS-SD name)

   * `/var/ipp-usb/lock/ipp-usb.lock`:
     lock file, that helps to prevent multiple copies of daemon to run simultaneously

## COPYRIGHT

Copyright (c) by Alexander Pevzner (pzz@apevzner.com)<br/>
All rights reserved.

This program is licensed under 2-Clause BSD license. See LICENSE file for details.

## SEE ALSO

cups(1)

# vim:ts=8:sw=4:et
