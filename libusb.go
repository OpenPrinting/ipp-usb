/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Cgo binding for libusb
 */

package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/google/gousb"
)

// #cgo pkg-config: libusb-1.0
// #include <libusb.h>
//
// void libusbHotplugCallback (libusb_context *ctx, libusb_device *device,
//     libusb_hotplug_event event, void *user_data);
//
// typedef struct libusb_device_descriptor libusb_device_descriptor_struct;
// typedef struct libusb_config_descriptor libusb_config_descriptor_struct;
// typedef struct libusb_interface libusb_interface_struct;
// typedef struct libusb_interface_descriptor libusb_interface_descriptor_struct;
// typedef struct libusb_endpoint_descriptor libusb_endpoint_descriptor_struct;
import "C"

// LibusbError implements error interface for
// libusb error codes
type LibusbError int

// Error describes a libusb error
func (err LibusbError) Error() string {
	var s string

	if err < 0 {
		s = C.GoString(C.libusb_strerror(int32(err)))
	}

	switch err {
	case 0:
		s = "OK"
	case C.LIBUSB_TRANSFER_ERROR:
		s = "transfer failed"
	case C.LIBUSB_TRANSFER_TIMED_OUT:
		s = "transfer timed out"
	case C.LIBUSB_TRANSFER_CANCELLED:
		s = "transfer was cancelled"
	case C.LIBUSB_TRANSFER_STALL:
		s = "transfer stalled"
	case C.LIBUSB_TRANSFER_NO_DEVICE:
		s = "device was disconnected"
	case C.LIBUSB_TRANSFER_OVERFLOW:
		s = "transfer overflow"
	default:
		s = fmt.Sprintf("unknown %d", err)
	}

	return "libusb: " + s
}

// libusbContextPtr keeps a pointer to libusb_context.
// It is initialized on demand
var (
	libusbContextPtr  *C.libusb_context
	libusbContextLock sync.Mutex
	libusbContextOk   int32
	libusbHotlugChan  = make(chan struct{})
)

// libusbContext returns libusb_context. It
// initializes context on demand.
func libusbContext() (*C.libusb_context, error) {
	if atomic.LoadInt32(&libusbContextOk) != 0 {
		return libusbContextPtr, nil
	}

	libusbContextLock.Lock()
	defer libusbContextLock.Unlock()

	// Obtain libusb_context
	rc := C.libusb_init(&libusbContextPtr)
	if rc != 0 {
		return nil, LibusbError(rc)
	}

	// Subscribe to hotplug events
	C.libusb_hotplug_register_callback(
		libusbContextPtr, // libusb_context
		C.LIBUSB_HOTPLUG_EVENT_DEVICE_ARRIVED| // events mask
			C.LIBUSB_HOTPLUG_EVENT_DEVICE_LEFT,
		C.LIBUSB_HOTPLUG_NO_FLAGS,  // flags
		C.LIBUSB_HOTPLUG_MATCH_ANY, // vendor_id
		C.LIBUSB_HOTPLUG_MATCH_ANY, // product_id
		C.LIBUSB_HOTPLUG_MATCH_ANY, // dev_class
		C.libusb_hotplug_callback_fn( // callback
			unsafe.Pointer(C.libusbHotplugCallback)),
		nil, // callback's data
		nil, // deregister handle
	)

	atomic.StoreInt32(&libusbContextOk, 1)
	return libusbContextPtr, nil
}

// Called by libusb on hotplug event
//
//export libusbHotplugCallback
func libusbHotplugCallback(ctx *C.libusb_context, dev *C.libusb_device,
	event C.libusb_hotplug_event, p unsafe.Pointer) {

	usbaddr := UsbAddr{
		Bus:     int(C.libusb_get_bus_number(dev)),
		Address: int(C.libusb_get_device_address(dev)),
	}

	switch event {
	case C.LIBUSB_HOTPLUG_EVENT_DEVICE_ARRIVED:
		Log.Debug('+', "HOTPLUG: added %s", usbaddr)
	case C.LIBUSB_HOTPLUG_EVENT_DEVICE_LEFT:
		Log.Debug('-', "HOTPLUG: removed %s", usbaddr)
	}

	select {
	case libusbHotlugChan <- struct{}{}:
	default:
	}
}

// LibusbBuildUsbAddrList return list of IPP-over-USB
// device descriptors
func LibusbGetIppOverUsbDeviceDescs() ([]UsbDeviceDesc, error) {
	// Obtain libusb context
	ctx, err := libusbContext()
	if err != nil {
		return nil, err
	}

	// Obtain list of devices
	var devlist **C.libusb_device
	cnt := C.libusb_get_device_list(ctx, &devlist)
	if cnt < 0 {
		return nil, LibusbError(cnt)
	}
	defer C.libusb_free_device_list(devlist, 1)

	// Convert devlist to slice.
	// See https://github.com/golang/go/wiki/cgo#turning-c-arrays-into-go-slices
	devs := (*[1 << 28]*C.libusb_device)(unsafe.Pointer(devlist))[:cnt:cnt]

	// Now build list of addresses
	var descs []UsbDeviceDesc

	for _, dev := range devs {
		desc, err := libusbBuildUsbDeviceDesc(dev)
		if err == nil && len(desc.IfAddrs) >= 2 {
			descs = append(descs, desc)
		}
	}

	return descs, nil
}

// libusbBuildUsbDeviceDesc builds device descriptor
func libusbBuildUsbDeviceDesc(dev *C.libusb_device) (UsbDeviceDesc, error) {
	var c_desc C.libusb_device_descriptor_struct
	var desc UsbDeviceDesc

	// Obtain device descriptor
	rc := C.libusb_get_device_descriptor(dev, &c_desc)
	if rc < 0 {
		return desc, LibusbError(rc)
	}

	// Decode device descriptor
	desc.Bus = int(C.libusb_get_bus_number(dev))
	desc.Address = int(C.libusb_get_device_address(dev))
	desc.Vendor = uint16(c_desc.idVendor)
	desc.Product = uint16(c_desc.idProduct)
	desc.Config = -1

	// Roll over configs/interfaces/alt settings/endpoins
	for cfgNum := 0; cfgNum < int(c_desc.bNumConfigurations); cfgNum++ {
		// Make sure we use the same configuration for all interfaces
		if desc.Config >= 0 && desc.Config != cfgNum {
			continue
		}

		var conf *C.libusb_config_descriptor_struct
		rc = C.libusb_get_config_descriptor(dev, C.uint8_t(cfgNum), &conf)
		if rc == 0 {
			ifcnt := conf.bNumInterfaces
			ifaces := (*[256]C.libusb_interface_struct)(
				unsafe.Pointer(conf._interface))[:ifcnt:ifcnt]

			for ifnum, iface := range ifaces {
				altcnt := iface.num_altsetting
				alts := (*[256]C.libusb_interface_descriptor_struct)(
					unsafe.Pointer(iface.altsetting))[:altcnt:altcnt]

				for altnum, alt := range alts {
					// We are only interested in IPP-over-USB
					// interfaces, i.e., LIBUSB_CLASS_PRINTER,
					// SubClass 1, Protocol 4
					if alt.bInterfaceClass == C.LIBUSB_CLASS_PRINTER &&
						alt.bInterfaceSubClass == 1 &&
						alt.bInterfaceProtocol == 4 {

						epnum := alt.bNumEndpoints
						endpoints := (*[256]C.libusb_endpoint_descriptor_struct)(
							unsafe.Pointer(alt.endpoint))[:epnum:epnum]

						in, out := -1, -1
						for _, ep := range endpoints {
							num := int(ep.bEndpointAddress & 0xf)
							dir := int(ep.bEndpointAddress & 0x80)
							switch dir {
							case C.LIBUSB_ENDPOINT_IN:
								if in == -1 {
									in = num
								}
							case C.LIBUSB_ENDPOINT_OUT:
								if out == -1 {
									out = num
								}
							}
						}

						// Build and append UsbIfAddr
						if in >= 0 && out >= 0 {
							desc.Config = cfgNum
							addr := UsbIfAddr{
								UsbAddr: desc.UsbAddr,
								CfgNum:  cfgNum,
								Num:     ifnum,
								Alt:     altnum,
							}
							desc.IfAddrs.Add(addr)
						}
					}
				}
			}

			C.libusb_free_config_descriptor(conf)
		}
	}

	return desc, nil
}

// UsbDeviceDesc represents a device descriptor
type UsbDeviceDesc struct {
	UsbAddr                       // Device address
	Vendor, Product uint16        // Vendor and product
	Config          int           // Used configuration
	IfAddrs         UsbIfAddrList // IPP-over-USB interfaces
}

// LibUsbDevHandle represents libusb_device_handle
type LibUsbDevHandle C.libusb_device_handle

// LibusbOpenDevice opens device by address
func LibusbOpenDevice(addr UsbAddr, config int) (*LibUsbDevHandle, error) {
	// Obtain libusb context
	ctx, err := libusbContext()
	if err != nil {
		return nil, err
	}

	// Obtain list of devices
	var devlist **C.libusb_device
	cnt := C.libusb_get_device_list(ctx, &devlist)
	if cnt < 0 {
		return nil, LibusbError(cnt)
	}
	defer C.libusb_free_device_list(devlist, 1)

	// Convert devlist to slice.
	devs := (*[1 << 28]*C.libusb_device)(unsafe.Pointer(devlist))[:cnt:cnt]

	// Find and open a device
	for _, dev := range devs {
		bus := int(C.libusb_get_bus_number(dev))
		address := int(C.libusb_get_device_address(dev))

		if addr.Bus == bus && addr.Address == address {
			// Open device
			var devhandle *C.libusb_device_handle
			rc := C.libusb_open(dev, &devhandle)
			if rc < 0 {
				return nil, LibusbError(rc)
			}

			// Detach kernel driver
			C.libusb_set_auto_detach_kernel_driver(devhandle, 1)

			// Set configuration
			rc = C.libusb_set_configuration(devhandle, C.int(config))
			if rc < 0 {
				C.libusb_close(devhandle)
				return nil, LibusbError(rc)
			}

			return (*LibUsbDevHandle)(devhandle), nil
		}
	}

	return nil, nil
}

// Close a device
func (devhandle *LibUsbDevHandle) Close() {
	C.libusb_close((*C.libusb_device_handle)(devhandle))
}

// Reset a device
func (devhandle *LibUsbDevHandle) Reset() {
	C.libusb_reset_device((*C.libusb_device_handle)(devhandle))
}

// UsbDeviceInfo returns UsbDeviceInfo for the device
func (devhandle *LibUsbDevHandle) UsbDeviceInfo() (UsbDeviceInfo, error) {
	dev := C.libusb_get_device((*C.libusb_device_handle)(devhandle))

	var c_desc C.libusb_device_descriptor_struct
	var info UsbDeviceInfo

	// Obtain device descriptor
	rc := C.libusb_get_device_descriptor(dev, &c_desc)
	if rc < 0 {
		return info, LibusbError(rc)
	}

	// Decode device descriptor
	info.Vendor = gousb.ID(c_desc.idVendor)
	info.Product = gousb.ID(c_desc.idProduct)

	buf := make([]byte, 256)

	strings := []struct {
		idx C.uint8_t
		str *string
	}{
		{c_desc.iManufacturer, &info.Manufacturer},
		{c_desc.iProduct, &info.ProductName},
		{c_desc.iSerialNumber, &info.SerialNumber},
	}

	for _, s := range strings {
		rc := C.libusb_get_string_descriptor_ascii(
			(*C.libusb_device_handle)(devhandle),
			s.idx,
			(*C.uchar)(unsafe.Pointer(&buf[0])),
			C.int(len(buf)),
		)

		if rc > 0 {
			*s.str = string(buf[:rc])
		}
	}

	return info, nil
}

// OpenLibusbIface opens an interface
func (devhandle *LibUsbDevHandle) OpenLibusbIface(
	addr UsbIfAddr) (*LibusbInterface, error) {

	// Claim the interface
	rc := C.libusb_claim_interface(
		(*C.libusb_device_handle)(devhandle),
		C.int(addr.Num),
	)
	if rc < 0 {
		return nil, LibusbError(rc)
	}

	// Activate alternate setting
	rc = C.libusb_set_interface_alt_setting(
		(*C.libusb_device_handle)(devhandle),
		C.int(addr.Num),
		C.int(addr.Alt),
	)

	if rc < 0 {
		C.libusb_release_interface(
			(*C.libusb_device_handle)(devhandle),
			C.int(addr.Num),
		)
		return nil, LibusbError(rc)
	}

	return &LibusbInterface{
		devhandle: devhandle,
		addr:      addr,
	}, nil
}

// LibusbIface represents IPP-over-USB interface
type LibusbInterface struct {
	devhandle *LibUsbDevHandle // Device handle
	addr      UsbIfAddr        // Interface address
}

// Close the interface
func (iface *LibusbInterface) Close() {
	C.libusb_release_interface(
		(*C.libusb_device_handle)(iface.devhandle),
		C.int(iface.addr.Num),
	)
}
