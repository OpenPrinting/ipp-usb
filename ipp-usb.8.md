ipp-usb(8) -- Daemon for IPP over USB printer support
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
     check configuration and exit. It also prints a list
     of all connected devices

   * `status`:
     print status of the running `ipp-usb` daemon, including information
     of all connected devices

### Options are

   * `-bg`<br>
     run in background (ignored in debug mode)

   * `-path-conf-files-srch dir1[:dir2...]`<br>
     List of directories where configuration files (ipp-usb.conf)
     are searched (/etc/ipp-usb)

   * `-path-log-dir dir`<br>
      Path to the directory where log files (main.log and per-device
      <DEVICE>.log) are written (/var/log/ipp-usb)

   * `-path-lock-file file`<br>
     Path to the program's lock file (/var/ipp-usb/lock/ipp-usb.lock)

   * `-path-dev-state-dir dir`<br>
     Path to the directory where per-device state files are written
     (/var/ipp-usb/dev)

   * `-path-ctrl-sock file`<br>
     Path to the program's control socket
     (/var/ipp-usb/ctrl)

   * `-path-quirks-files-srch dir1[:dir2...]`<br>
     List of directories where quirks files (\*.conf) is searched
     (/etc/ipp-usb/quirks:/usr/share/ipp-usb/quirks)

## NETWORKING

Essentially, `ipp-usb` makes printer or scanner accessible from the
network, converting network-side HTTP operations to the USB operations.

By default, `ipp-usb` exposes device only to the loopback interface,
using the `localhost` address (both `127.0.0.1` and `::1`, for IPv4
and IPv6, respectively). TCP ports are allocated automatically, and
allocation is persisted in the association with the particular device,
so the next time the device is plugged on, it will get the same port.
The default port range for TCP ports allocation is `60000-65535`.

This default behavior can be changed, using configuration file. See
`CONFIGURATION` section below for details.

If you decide to publish your device to the real network, the following
things should be taken into consideration:

   1. Your **private** device will become **public** and it will become
      accessible by other computers from the network
   2. Firewall rules needs to be updated appropriately. The `ipp-usb`
      daemon will not do it automatically by itself
   3. IPP over USB specification explicitly require that the
      `Host` field in the HTTP request is set to `localhost`
      or `localhost:port`. If device is accessed from the real
      network, `Host` header will reflect the real network address.
      Most of devices allow it, but some are more restrictive
      and will not work in this configuration.

## DNS-SD (AVAHI INTEGRATION)

IPP over USB is intended to be used with the automatic device discovery,
and for this purpose `ipp-usb` advertises all devices it handles, using
DNS-SD protocol. On Linux, DNS-SD is handled with a help of Avahi daemon.

DNS-SD advertising can be disabled via configuration file. Also, if Avahi
is not installed or not running, `ipp-usb` will still work correctly,
although DNS-SD advertising will not work.

For every device the following services will be advertised:

   | Instance    | Type          | Subtypes                  |
   | ----------- | ------------- | ------------------------- |
   | Device name | _ipp._tcp     | _universal._sub._ipp._tcp |
   | Device name | _printer._tcp |                           |
   | Device name | _uscan._tcp   |                           |
   | Device name | _http._tcp    |                           |
   | BBPP        | _ipp-usb._tcp |                           |


Notes:

   * `Device name` is the name under which device appears in
     the list of available devices, for example, in the printing
     dialog (it is DNS-SD device name, in another words), and for
     most of devices will match the device's model name. It
     is appended with the `" (USB)"` suffix, so if device is
     connected via network and via USB simultaneously, these
     two connections can be easily distinguished. If there
     are two devices with the same name connected simultaneously,
     the suffix becomes `" (USB NNN)"`, with NNN number unique for
     each device, for disambiguation. In another words, the single
     `"Kyocera ECOSYS M2040dn"` device will be listed as
     `"Kyocera ECOSYS M2040dn (USB)"`, and two such a devices will
     be listed as `"Kyocera ECOSYS M2040dn (USB 1)"` and
     `"Kyocera ECOSYS M2040dn (USB 2)"`
   * `_ipp._tcp` and `_printer._tcp` are only advertises for
     printer devices and MFPs
   * `_uscan._tcp` is only advertised for scanner devices and MFPs
   * for the `_ipp._tcp` service, the `_universal._sub._ipp._tcp`
     subtype is also advertised for iOS compatibility
   * `_printer._tcp` is advertised with TCP port set to 0. Other
     services are advertised with the actual port number
   * `_http._tcp` is device web-console. It is always advertises
     in assumption it is always exist
   * `BBPP`, used for the `_ipp-usb._tcp` service, is the
     USB bus (BB) and port (PP) numbers in hex. The purpose
     of this advertising is to help CUPS and other possible
     "clients" to guess which devices are handled by the
     `ipp-usb` service, to avoid possible conflicts with the
     legacy USB drivers.

## Matching DNS-SD announcements against local USB bus

Client software may need to match local devices exposed by `ipp-usb` with
those found on the local USB bus. This can be useful, for example, to
avoid offering USB devices in the print or scan dialog that cannot be
used because they are already in use by `ipp-usb`.

As a rough guideline, clients might consider USB devices that offer
interfaces with Class=7, SubClass=1, and Protocol=4 as belonging to
`ipp-usb`. However, this is not a precise method. Some devices may not
properly implement the IPP over USB protocol and could be blacklisted in
the `ipp-usb` configuration for this reason. Additionally, some devices
may be automatically recognized by `ipp-usb` as incompatible and thus
skipped. It's also possible that `ipp-usb` is disabled on the system
entirely.

A more reliable approach is to directly query the running `ipp-usb`
instance to determine which devices it actually handles.

To facilitate the matching of devices that `ipp-usb` manages and
announces via  DNS-SD against local devices found on the USB bus,
`ipp-usb` adds two TXT records to each `_ipp._tcp`, `_printer._tcp`,
and `_uscan._tcp` services it announces:

   * usb_SER=VCF9192281 - USB serial number for the device

   * usb_HWID=0482&069d - USB vendor and hardware ID for the device in hex

Please note that when matching devices discovered via DNS-SD with USB
devices, it is important to only consider DNS-SD advertisements from IP
addresses that are either loopback addresses (127.0.0.1 or ::1) or
belong to a local interface.

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
      # TCP ports for HTTP will be automatically allocated in the
      # following range
      http-min-port = 60000
      http-max-port = 65535

      # Enable or disable DNS-SD advertisement
      dns-sd = enable      # enable | disable

      # Network interface to use. Set to `all` if you want to expose you
      # printer to the local network. This way you can share your printer
      # with other computers in the network, as well as with iOS and
      # Android devices.
      interface = loopback # all | loopback

      # Enable or disable IPv6
      ipv6 = enable        # enable | disable

### Authentication

By default, `ipp-usb` exposes locally connected USB printer to all users
of the system.

Though this is reasonable behavior in most cases, when computer and printer
are both in personal use, for bigger installation this approach can be too
simple and primitive.

`ipp-usb` provides a mechanism, which allows to control local clients
access based on UID the client program runs under.

Please note, this mechanism will not work for remote connections (disabled
by default but supported). Authentication of remote users requires some
different mechanism, which is under consideration but is not yet implemented.

Note also, this mechanism may or may not work in containerized installation
(i.e., snap, flatpak and similar).  The container namespace may be isolated
from the system and/or user's namespaces, so even for local clients the UID
as seen by the `ipp-usb` may be different from the system-wide UID.

Authentication parameters are all in the [auth uid] section:

    # Local user authentication by UID/GID
    [auth uid]
      # Syntax:
      #     operations = users
      #
      # Operations are comma-separated list of following operations:
      #     all    - all operations
      #     config - configuration web-console
      #     fax    - faxing
      #     print  - printing
      #     scan   - scanning
      #
      # Users have the following suntax:
      #     user   - user name
      #     @group - all users that belongs to the group
      #
      # Users and groups may be specified either by names or by
      # numbers. "*" means any
      #
      # Note, if user/group is not known in the context of request
      # (for example, in the case of non-local network connection),
      # "*" used for matching, which will only match wildcard
      # rules.
      #
      # User/group names are resolved at the moment of request
      # processing (and cached for a couple of seconds), so running
      # daemon will see changes to the /etc/passwd and /etc/group
      #
      # Examples:
      #     fax, print = lp, @lp   # Allow CUPS to do its work
      #     scan       = *         # Allow any user to scan
      #     config     = @wheel    # Only wheel group members can do that
      all = *

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
      #   trace-ipp, trace-escl, trace-http - very detailed
      #               per-protocol traces
      #   trace-usb - hex dump of all USB traffic
      #   all       - all logs
      #   trace-all - alias to all
      #
      # Note, trace-* implies debug, debug implies info, info implies
      # error
      device-log    = all
      main-log      = debug
      console-log   = debug

      # Log rotation parameters:
      #   log-file-size    - max log file before rotation. Use suffix
      #                      M for megabytes or K for kilobytes
      #   log-backup-files - how many backup files to preserve during
      #                      rotation
      #
      max-file-size    = 256K
      max-backup-files = 5

      # Enable or disable ANSI colors on console
      console-color = enable # enable | disable

      # ipp-usb queries IPP printer attributes at the initialization time
      # for its own purposes and writes received attributes to the log.
      # By default, only necessary attributes are requested from device.
      #
      # If this parameter is set to true, all printer attributes will
      # be requested. Normally, it only affects the logging. However,
      # some enterprise-level HP printers returns such huge amount of
      # data and do it so slowly, so it can cause initialization timeout.
      # This is why this feature is not enabled by default
      get-all-printer-attrs = false # false | true

### Quirks

Some devices, due to their firmware bugs, require special handling, called
device-specific **quirks**. `ipp-usb` loads quirks from the
`/usr/share/ipp-usb/quirks/*.conf` files and from the
`/etc/ipp-usb/quirks/*.conf` files. The `/etc/ipp-usb/quirks` directory is for
system quirks overrides or admin changes. These files have .INI-file syntax
with the content that looks like this:

    # Various HP printers - match by bame
    [HP LaserJet MFP M28-M31]
      http-connection = keep-alive

    [HP OfficeJet Pro 8730]
      http-connection = close

    [HP Inc. HP Laser MFP 135a]
      blacklist = true

    # Xerox B210. Match by HWID, as model name is not reliable
    # before reset.
    [0924:42ea]
      mfg        = Xerox
      model      = B210
      init-reset = hard


    # Default configuration
    [*]
      http-connection = ""

Section name defines the device (or group of devices) quirks are
applied to, and section body contains the applied quirks.

For each discovered device, quirks are searched by the USB HWID
(Vendor:Product, in hex) and by the model name.

Section names may contain wildcards:

    `[HP OfficeJet Pro 8730]` - match the "HP OfficeJet Pro 8730" device
    `[HP OfficeJet *]`        - match all models with name started with
                                the "HP OfficeJet" prefix.
    `[0924:42ea]`             - match the device with the USB HWID 0924:42ea
    `[0924:*]`                - match all devices with the Vendor ID equal
                                to 0924 (this ID owned by Xerox).

Model names may contain glob-style wildcards: `*` that matches any sequence of
characters and `?` , that matches any single character. To match one of these
characters (`*` and `?`) literally, use backslash as escape.

HWID sections may only contain a `*` wildcard in a place of the Product ID.

Note, the simplest way to guess the exact model name for the particular
device is to use `ipp-usb check` command, which prints a list of all
connected devices. To obtain list if USB HWIDs, use the `lsusb` command.

If some parameter found in multiple matching sections, `ipp-usb` follows the
principle: most specific match wins.

To be more precise, the following prioritization algorithm is used:

* The exact HWID (non-wildcard, i.e., `[0924:42ea]`) considered the most
specific.
* The next candidates are model name match with at least one matched
non-wildcard character. If there are multiple model name matches, amount
of non-wildcard matched characters is counted, and the longer match wins.
* The next candidate is the wildcard HWID match (i.e., `[0924:*]`).
* And the least specific is the all-wildcard model name match (i.e., `[*]`).

In a case of multiple matches even after applying this algorithm, the first
definition wins. Files are loaded in alphabetical order, sections  read
sequentially

In another words, non-wildcard HWID match considered most specific, model name
matches are ranked by the amount of non-wildcard matched characters, and
wildcard HWID match considered less specific as any of above, because it can
only be applied to all devices of the particular vendor, and the least
specific is the all-wildcard model name match, used to specify defaults.

Please notice that HWID-matched quirks are loaded early and may cause some
actions to be performed before the model-name matched quirks are loaded and
applied, and the model-name matched quirks cannot withdraw these early
performed actions. It happens because some printers cannot reliably report
their model name before appropriate actions is applied. It is related to
the following quirks:

   * `blacklist = true`. Model-name quirks will not be loaded if device is
     blacklisted by HWID.

   * `init-reset = hard`. Reset will be issued, even if model-name matched
     quirk has a different setting.

   * `mfg = name` and `model = name`. These parameters are only available
     for the HWID quirks and override identification information, provided
     by the device, when searching quirks by the model name.

The following parameters are defined:

   * `blacklist = true | false`<br>
     If `true`, the matching device is ignored by the `ipp-usb`

   * `buggy-ipp-responses = reject | allow | sanitize`<br>
     Some devices send buggy (malformed) IPP responses that violate
     IPP specification. `ipp-usb` may `reject` these responses
     (so `ipp-usb` initialization will fail), `allow` them (`ipp-usb`
     initialization will succeed, but CUPS needs to accept them
     as well) or `sanitize` them (fix IPP specs violations).

   * `disable-fax = true | false`<br>
     If `true`, the matching device's fax capability is ignored.

   * `http-XXX = YYY`<br>
     Set XXX header of the HTTP requests forwarded to device to YYY.
     If YYY is empty string, XXX header is removed.

   * `ignore-ipp-status = true | false`<br>
     If `true`, IPP status of IPP requests sent by the `ipp-usb` by
     itself will be ignored. This quirk is useful, when device correctly
     handles IPP request but returned status is not reliable. Affects
     only `ipp-usb` initialization.

   * `init-delay = DELAY`<br>
     Delay, between device is opened and, optionally, reset, and the
     first request is sent to device.

   * `init-retry-partial = true | false`<br>
     Retry the initialization in case only part of the device's functions
     have been initialized, instead of continuing to operate with incomplete
     functionality.

     It can be useful if the device takes a long time to fully initialize.
     During this period, some components may respond normally while others
     are still initializing. For example, the device may quickly report its
     scanning capabilities shortly after startup, while its printing
     functionality may take several minutes to become operational.

     Some enterprise-level HP printers are known to have this problem.

   * `init-reset = none | soft | hard`<br>
     How to reset device during initialization. Default is `none`

   * `init-timeout = DELAY`<br>
     Timeout for HTTP requests send by the `ipp-usb` during initialization.

   * `mfg = name`<br>
     Overrides the USB manufacturer (vendor) name. This quirk can only
     be used in the HWID section and affects searching quirks by model
     name.

   * `model = name`<br>
     Overrides the USB model (product) name. This quirk can only
     be used in the HWID section and affects searching quirks by model
     name.

   * `request-delay = DELAY`<br>
     Delay between subsequent HTTP requests, sent to device (this is not
     the same as `usb-send-delay`, which inserts delays between each
     subsequent USB send-to-device requests).

   * `usb-max-interfaces = N`<br>
     Don't use more that N USB interfaces, even if more is available.

   * `usb-send-delay = DELAY`<br>
     Delay between low-level USB send-to-device requests (this is not
     the same as `request-delay`, which inserts delays between the
     whole HTTP-level requests).

   * `usb-send-delay-threshold = N`<br>
     `usb-send-delay` only applied if USB send-to-device request size
     exceeds this threshold.

   * `zlp-recv-hack = true | false`<br>
     Some enterprise-level HP devices, during the initialization phase
     (which can last several minutes), may respond with an HTTP 503
     status or similar, which is expected. However, the response body may
     be truncated (typically, the terminating '\n' is lost). In such
     cases, `ipp-usb` will wait indefinitely for a response to maintain
     synchronization with the device.

     At the same time, these devices send a zero-length UDP packet at the
     end of the truncated output. If the `zlp-recv-hack` quirk is enabled,
     when ipp-usb receives a zero-length packet from the USB followed by
     a receive timeout, it interprets this combination of events as a
     valid termination of the response body. It works only at the
     initialization time and doesn't affect futher operations.

   * `zlp-send = true | false`<br>
     Terminate outgoing transfers that a multiple of the endpoint's
     packet size win an extra zero length packet.

The DELAY parameter can be specified either as an unsigned integer (in
milliseconds) or as a sequence of decimal numbers with an optional
fraction and a unit suffix, such as "300ms," "0.5s," or "2m30s." Valid
time units are "ns," "us" (or "µs"), "ms" "s" "m" and "h"

If you found out about your device that it needs a quirk to work properly or it
does not work with `ipp-usb` at all, although it provides IPP-over-USB
interface, please report the issue at https://github.com/OpenPrinting/ipp-usb.
It will let us to update our collection of quirks, so helping other owners
of such a device.

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

   * `/var/ipp-usb/ctrl`:
     `ipp-usb` control socket. Currently only used to obtain the
     per-device status (printed by `ipp-usb status`), but its
     functionality may be extended in a future

   * `/usr/share/ipp-usb/quirks/*.conf`: device-specific quirks (see above)

   * `/etc/ipp-usb/quirks/*.conf`: device-specific quirks defined by sysadmin (see above)

## COPYRIGHT

Copyright (c) by Alexander Pevzner (pzz@apevzner.com, pzz@pzz.msk.ru)<br/>
All rights reserved.

This program is licensed under 2-Clause BSD license. See LICENSE file for details.

## SEE ALSO

**cups(1)**

# vim:ts=8:sw=4:et
