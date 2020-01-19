# ipp-usb

[IPP-over-USB](https://www.usb.org/document-library/ipp-protocol-10) allows
using IPP protocol, normally designed for the network printers, to be used
with USB printers as well.

The idea behind this standard is simple: it allows to send HTTP requests
to the device via USB connection, so enabling IPP, eSCL (AirScan) and web
console on devices without Ethernet or WiFi connections.

Unfortunately, the naive implementation, which simply relays TCP connection
to USB, doesn't work. It happens because closing TCP connection on a client
side has a side effect of discarding all data sent to this connection from
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
