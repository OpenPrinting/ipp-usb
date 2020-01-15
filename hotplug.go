/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Handling USB hotplug events
 */

package main

// #cgo pkg-config: libusb-1.0
// #include <libusb.h>
//
// // Go callback for hotplug events
// void usbHotplugCallback(int bus, int addr, libusb_hotplug_event event);
//
// // C-to-Go adapter for hotplug callback
// static int
// usb_hotplug_callback (libusb_context *ctx, libusb_device *device,
//         libusb_hotplug_event event, void *user_data)
// {
//     int bus = libusb_get_bus_number(device);
//     int addr = libusb_get_device_address(device);
//     usbHotplugCallback(bus, addr, event);
//     return 0;
// }
//
// // Subscribe to hotplug notifications
// static void
// usb_hotplug_init (void)
// {
//     libusb_hotplug_register_callback(
//         NULL,                                  // libusb_context
//         LIBUSB_HOTPLUG_EVENT_DEVICE_ARRIVED |  // events mask
//             LIBUSB_HOTPLUG_EVENT_DEVICE_LEFT,
//         LIBUSB_HOTPLUG_NO_FLAGS,               // flags
//         LIBUSB_HOTPLUG_MATCH_ANY,              // vendor_id
//         LIBUSB_HOTPLUG_MATCH_ANY,              // product_id
//         LIBUSB_HOTPLUG_MATCH_ANY,              // dev_class
//         usb_hotplug_callback,                  // callback
//         NULL,                                  // callback's data
//         NULL                                   // deregister handle
//     );
// }
import "C"

// UsbHotPlugChan gets signalled on any hotplug event
var UsbHotPlugChan = make(chan struct{})

// Called by libusb on hotplug event
//
//export usbHotplugCallback
func usbHotplugCallback(bus, addr C.int, event C.libusb_hotplug_event) {
	switch event {
	case C.LIBUSB_HOTPLUG_EVENT_DEVICE_ARRIVED:
		log_debug("+ HOTPLUG %d %d", bus, addr)
	case C.LIBUSB_HOTPLUG_EVENT_DEVICE_LEFT:
		log_debug("- HOTPLUG %d %d", bus, addr)
	}

	select {
	case UsbHotPlugChan <- struct{}{}:
	default:
	}
}

// Initialize USB hotplug handling
func init() {
	C.usb_hotplug_init()
}
