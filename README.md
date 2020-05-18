# ipp-usb

![GitHub](https://img.shields.io/github/license/OpenPrinting/ipp-usb)
[![Go Report Card](https://goreportcard.com/badge/github.com/OpenPrinting/ipp-usb)](https://goreportcard.com/badge/github.com/OpenPrinting/ipp-usb)

## Introduction

[IPP-over-USB](https://www.usb.org/document-library/ipp-protocol-10)
allows using the IPP protocol, normally designed for network printers,
to be used with USB printers as well.

The idea behind this standard is simple: It allows to send HTTP
requests to the device via a USB connection, so enabling IPP, eSCL
(AirScan) and web console on devices without Ethernet or WiFi
connections.

Unfortunately, the naive implementation, which simply relays a TCP
connection to USB, does not work. It happens because closing the TCP
connection on the client side has a useful side effect of discarding
all data sent to this connection from the server side, but it does not
happen with USB connections. In the case of USB, all data not received
by the client will remain in the USB buffers, and the next time the
client connects to the device, it will receive unexpected data, left
from the previous abnormally completed request.

Actually, it is an obvious flaw in the IPP-over-USB standard, but we
have to live with it.

So the implementation, once the HTTP request is sent, must read the
entire HTTP response, which means that the implementation must
understand the HTTP protocol, and effectively implement a HTTP reverse
proxy, backed by the IPP-over-USB connection to the device.

And this is what the **ipp-usb** program actually does.

## Features in detail

* Implements HTTP proxy, backed by USB connection to IPP-over-USB device
* Full support of IPP printing, eSCL scanning, and web admin interface
* DNS-SD advertising for all supported services
* DNS-SD parameters for IPP based on IPP get-printer-attributes query
* DNS-SD parameters for eSCL based on parsing GET /eSCL/ScannerCapabilities response
* TCP port allocation for device is bound to particular device (combination of
VendorID, ProductID and device serial number), so if the user has multiple
devices, they will receive the same TCP port when connected. This allocation
is persisted on a disk
* Automatic DNS-SD name conflict resolution. The finally chosen device's
network name is persisted on a disk
* Can be started by **UDEV** or run in standalone mode
* Can share printer to other computers on a network, or use the loopback interface only
* Can generate very detailed logs for possible troubleshooting

## Under the hood

Though looks simple, ipp-usb does many non obvious things under the hood

* Client-side HTTP connections are completely decoupled from printer-side HTTP-over-USB connections
* HTTP requests are sanitized, missed headers are added
* HTTP protocol upgraded from 1.0 to 1.1, if needed
* Attempts to upgrade HTTP connection to winsock, if unwisely made by web console, are
prohibited, because it can steal USB connection for a long time
* Client HTTP requests are fairly balanced between all available 2-3 USB connections,
regardless of number and persistence of client connections
* Dropping connection by client properly handled in all cases, even in a middle of sending.
In a worst case, printer may receive truncated document, but HTTP transaction will always be
performed correctly

## Memory footprint

Being written on Go, ipp-usb has a large executable size. However, its
memory consumption is not very high. When single device is connected,
ipp-usb RSS is similar or even slightly less in comparison to ippusbxd.
And because ipp-usb handles all devices in a single process, it uses noticeably
less memory that ippusbxd, when serving 2 or more devices.

## External dependencies

This program has very few external dependencies, namely:
* `libusb` for USB access
* `libavahi-common` and `libavahi-client` for DNS-SD
* Running Avahi daemon

## Binary packages

Binary packages available for the following Linux distros:
* **Debian** (10)
* **Fedora** (29, 30, 31 and 32)
* **openSUSE** (Tumbleweed)
* **Ubuntu** (18.04, 19.04, 19.10 and 20.04)

**Linux Mint** users may use Ubuntu packages:
* Linux Mint 18.x - use packages for Ubuntu 16.04
* Linux Mint 19.x - use packages for Ubuntu 18.04

Follow this link for downloads: https://download.opensuse.org/repositories/home:/pzz/

## Installation from source

You will need to install the following packages (exact name depends
of your Linux distro):
* libusb development files
* libavahi-client and libavahi-common development files
* gcc
* Go compiler
* pkg-config
* git, make and so on

Building is really simple:
* Clone this repo
* Enter its directory
* make

Then you may `make install` or just try `ipp-usb` directly from
build directory

## Avahi Notes (exposing printer to localhost)

IPP-over-USB normally exposes printer to localhost only, hence it
requires DNS-SD announces to work for localhost.

This requires Avahi 0.8.0 or newer. Older Avahi versions do not
support announcing to localhost.

Some Linux distros (for example recent Ubuntu and Fedora versions)
have their Avahi patched to support localhost, others (for example
Debian) not.

To determine if your Avahi supports localhost, run the following
command in one terminal session:
```
    avahi-publish -s test _test._tcp 1234
```
And simultaneously the following command in another terminal session
on the same machine:
```
    avahi-browse _test._tcp -r
```
If you see localhost in the avahi-browse output, like this:
```
    =     lo IPv4 test                                          _test._tcp           local
       hostname = [localhost]
       address = [127.0.0.1]
       port = [1234]
       txt = []
```
your Avahi is OK. Otherwise, update or patching is required.

So users of distros that ship a too old Avahi and without the patch
have three possibilities:
1. Update Avahi to 0.8.0 or newer
2. Apply the patch by themself, rebuild and reinstall avahi-daemon
3. Configure `ipp-usb` to run on all network interfaces, not only on loopback

If you decide to apply the patch, get it as `avahi/avahi-localhost.patch`
in this package or [download it here](https://raw.githubusercontent.com/OpenPrinting/ipp-usb/master/avahi/avahi-localhost.patch).

The third method is simple to do, just replace `interface = loopback`
with `interface = all` in the `ipp-usb.conf` file, but this has the
disadvantage of exposing your local USB-connected printer to the
entire local network, which can be an unwanted side effect, especially
in a big corporative network.
