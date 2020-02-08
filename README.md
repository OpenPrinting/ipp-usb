# ipp-usb

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
