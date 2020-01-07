// Handling USB hotplug events

package main

// #cgo pkg-config: libusb-1.0
// #include <libusb.h>
//
// void usbHotplugCallback(int bus, int addr, libusb_hotplug_event event);
//
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
// static void
// usb_hotplug_init (void)
// {
//     libusb_hotplug_register_callback(NULL,
//             LIBUSB_HOTPLUG_EVENT_DEVICE_ARRIVED | LIBUSB_HOTPLUG_EVENT_DEVICE_LEFT,
//             LIBUSB_HOTPLUG_NO_FLAGS, LIBUSB_HOTPLUG_MATCH_ANY, LIBUSB_HOTPLUG_MATCH_ANY,
//             LIBUSB_HOTPLUG_MATCH_ANY, usb_hotplug_callback,
//             NULL, NULL
//     );
// }
import "C"

// libusb_hotplug_callback_handle
// libusb_hotplug_register_callback
// libusb_hotplug_callback_fn
// typedef int (LIBUSB_CALL *libusb_hotplug_callback_fn)(libusb_context *ctx,
//						libusb_device *device,
//						libusb_hotplug_event event,
//						void *user_data);

// Called by libusb on hotplug event
//export usbHotplugCallback
func usbHotplugCallback(bus, addr C.int, event C.libusb_hotplug_event) {
	switch event {
	case C.LIBUSB_HOTPLUG_EVENT_DEVICE_ARRIVED:
		log_debug("+ HOTPLUG %d %d", bus, addr)
	case C.LIBUSB_HOTPLUG_EVENT_DEVICE_LEFT:
		log_debug("- HOTPLUG %d %d", bus, addr)
	}
}

// Initialize USB hotplug handling
func init() {
	C.usb_hotplug_init()
}
