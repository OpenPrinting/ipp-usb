# ipp-usb

![GitHub](https://img.shields.io/github/license/OpenPrinting/ipp-usb)
[![Go Report Card](https://goreportcard.com/badge/github.com/OpenPrinting/ipp-usb)](https://goreportcard.com/badge/github.com/OpenPrinting/ipp-usb)

## Introduction

[IPP-over-USB](https://www.usb.org/document-library/ipp-protocol-10) allows
using IPP protocol, normally designed for the network printers, to be used
with USB printers as well.

The idea behind this standard is simple: it allows to send HTTP requests
to the device via USB connection, so enabling IPP, eSCL (AirScan) and web
console on devices without Ethernet or WiFi connections.

Unfortunately, the naive implementation, which simply relays TCP connection
to USB, doesn't work. It happens because closing TCP connection on a client
side has a useful side effect of discarding all data sent to this connection from
the server side, but it doesn't happen with USB connections. In a case of USB,
all data, not received by a client, will remain in USB buffers, and the next
time client connects to device, it will receive unexpected data, left from
a previous abnormally completed request.

Actually, it's an obvious flaw in the IPP-over-USB standard, but we have
to live with it.

So implementation, once HTTP request is sent, must read the entire HTTP
response, which means that implementation must understand HTTP protocol,
and effectively implement a HTTP reverse proxy, backed by IPP-over-USB
connection to device.

And this is what **ipp-usb** program actually does.

## Features in detail

* Implements HTTP proxy, backed by USB connection to IPP-over-USB device
* Full support of IPP printing, eSCL scanning and web admin interface
* DNS-SD advertising for all supported services
* DNS-SD parameters for IPP based on IPP Get-Printer-Attributes query
* DNS-SD parameters for eSCL based on parsing GET /eSCL/ScannerCapabilities response
* TCP port allocation for device is bound to particular device (combination of
VendorID, ProductID and device serial number), so if user has multiple
devices, they will receive the same TCP port when connected. This allocation
is persisted on a disk
* Automatic DNS-SD name conflict resolution. The finally chosen device's
network name persisted on a disk
* Can be started by **udev** or run in standalone mode
* Can share printer to other computers on a network, or use loopback interface only
* Can generate very detailed logs for possible troubleshooting

## External dependencies

This program has very little external dependencies, namely:
* `libusb` for USB access
* `libavahi-common` and `libavahi-client` for DNS-SD
* Running Avahi daemon

## Avahi Notes (exposing printer to localhost)

IPP-over-USB normally exposes printer to localhost only, hence it
requires DNS-SD announces to work for localhost.

Unfortunately, upstream ("official") Avahi doesn't support announcing
to localhost.

Patches that fix this problem exist for several years, but still not
included into the official Avahi source tree

* https://github.com/lathiat/avahi/pull/125
* https://github.com/lathiat/avahi/pull/161

Some Linux distros (for example, recent Ububtu and Fedora versions)
include these patches into Avahi that comes with distros, others
(for example, Debian) wait until Avahi upstream will be patched.

To determine if your Avahi needs patching, run the following command
in one terminal session:

    avahi-publish -s test _test._tcp 1234

And simultaneously the following command in another terminal session
on a same machine:

    avahi-browse _test._tcp -r

If you see localhost in the avahi-browse output, like this:

    =     lo IPv4 test                                          _test._tcp           local
       hostname = [localhost]
       address = [127.0.0.1]
       port = [1234]
       txt = []

your Avahi is OK. Otherwise, patching is required.

So users of distros that ship unpatched Avahi have two variants:
1. Apply patch by themself, rebuild and reinstall Avahi daemon
2. Configure `ipp-usb` to run on all network interfaces, not only loopback

Second variant is simple to do (just replace `interface = loopback` with
`interface = all` in the ipp-usb.conf file, but it has a disadvantage
of exposing your local USB-connected printer to the entire network,
which can be unwanted side effect, especially in a big corporative
network.
