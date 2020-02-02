/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * USB low-level I/O. Cgo implementation on a top of libusb
 */

package main

import (
	"sync"
	"sync/atomic"
	"time"
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

// UsbError represents USB I/O error. It implements error interface
type UsbError int

const (
	UsbIO            UsbError = C.LIBUSB_ERROR_IO
	UsbEInval                 = C.LIBUSB_ERROR_INVALID_PARAM
	UsbEAccess                = C.LIBUSB_ERROR_ACCESS
	UsbENoDev                 = C.LIBUSB_ERROR_NO_DEVICE
	UsbENotFound              = C.LIBUSB_ERROR_NOT_FOUND
	UsbEBusy                  = C.LIBUSB_ERROR_BUSY
	UsbETimeout               = C.LIBUSB_ERROR_TIMEOUT
	UsbEOverflow              = C.LIBUSB_ERROR_OVERFLOW
	UsbEPipe                  = C.LIBUSB_ERROR_PIPE
	UsbEIntr                  = C.LIBUSB_ERROR_INTERRUPTED
	UsbENomem                 = C.LIBUSB_ERROR_NO_MEM
	UsbENotSupported          = C.LIBUSB_ERROR_NOT_SUPPORTED
	UsbEOther                 = C.LIBUSB_ERROR_OTHER
)

// Error describes a libusb error
func (err UsbError) Error() string {
	return "libusb: " + C.GoString(C.libusb_strerror(int32(err)))
}

var (
	// libusbContextPtr keeps a pointer to libusb_context.
	// It is initialized on demand
	libusbContextPtr *C.libusb_context

	// libusbContextLock protects libusbContextPtr initialization
	// in multithreaded context
	libusbContextLock sync.Mutex

	// Nonzero, if libusbContextPtr initialized
	libusbContextOk int32

	// libusbHotlugChan receives USB hotplug event notifications
	libusbHotlugChan = make(chan struct{})
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
		return nil, UsbError(rc)
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

// UsbGetIppOverUsbDeviceDescs return list of IPP-over-USB
// device descriptors
func UsbGetIppOverUsbDeviceDescs() ([]UsbDeviceDesc, error) {
	// Obtain libusb context
	ctx, err := libusbContext()
	if err != nil {
		return nil, err
	}

	// Obtain list of devices
	var devlist **C.libusb_device
	cnt := C.libusb_get_device_list(ctx, &devlist)
	if cnt < 0 {
		return nil, UsbError(cnt)
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
		return desc, UsbError(rc)
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

// UsbDevHandle represents libusb_device_handle
type UsbDevHandle C.libusb_device_handle

// UsbOpenDevice opens device by address
func UsbOpenDevice(addr UsbAddr, config int) (*UsbDevHandle, error) {
	// Obtain libusb context
	ctx, err := libusbContext()
	if err != nil {
		return nil, err
	}

	// Obtain list of devices
	var devlist **C.libusb_device
	cnt := C.libusb_get_device_list(ctx, &devlist)
	if cnt < 0 {
		return nil, UsbError(cnt)
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
				return nil, UsbError(rc)
			}

			// Detach kernel driver
			C.libusb_set_auto_detach_kernel_driver(devhandle, 1)

			// Set configuration
			rc = C.libusb_set_configuration(devhandle, C.int(config))
			if rc < 0 {
				C.libusb_close(devhandle)
				return nil, UsbError(rc)
			}

			return (*UsbDevHandle)(devhandle), nil
		}
	}

	return nil, nil
}

// Close a device
func (devhandle *UsbDevHandle) Close() {
	C.libusb_close((*C.libusb_device_handle)(devhandle))
}

// Reset a device
func (devhandle *UsbDevHandle) Reset() {
	C.libusb_reset_device((*C.libusb_device_handle)(devhandle))
}

// UsbDeviceInfo returns UsbDeviceInfo for the device
func (devhandle *UsbDevHandle) UsbDeviceInfo() (UsbDeviceInfo, error) {
	dev := C.libusb_get_device((*C.libusb_device_handle)(devhandle))

	var c_desc C.libusb_device_descriptor_struct
	var info UsbDeviceInfo

	// Obtain device descriptor
	rc := C.libusb_get_device_descriptor(dev, &c_desc)
	if rc < 0 {
		return info, UsbError(rc)
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

// UsbInterface opens an interface
func (devhandle *UsbDevHandle) OpenUsbInterface(addr UsbIfAddr) (
	*UsbInterface, error) {

	// Claim the interface
	rc := C.libusb_claim_interface(
		(*C.libusb_device_handle)(devhandle),
		C.int(addr.Num),
	)
	if rc < 0 {
		return nil, UsbError(rc)
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
		return nil, UsbError(rc)
	}

	return &UsbInterface{
		devhandle: devhandle,
		addr:      addr,
	}, nil
}

// UsbInterface represents IPP-over-USB interface
type UsbInterface struct {
	devhandle *UsbDevHandle // Device handle
	addr      UsbIfAddr     // Interface address
}

// Close the interface
func (iface *UsbInterface) Close() {
	C.libusb_release_interface(
		(*C.libusb_device_handle)(iface.devhandle),
		C.int(iface.addr.Num),
	)
}

// Send data to interface. Returns count of bytes actually transmitted
// and error, if any
func (iface *UsbInterface) Send(data []byte,
	timeout time.Duration) (n int, err error) {

	var transferred C.int

	rc := C.libusb_bulk_transfer(
		(*C.libusb_device_handle)(iface.devhandle),
		0, //iface.addr.Out | C.LIBUSB_ENDPOINT_OUT
		(*C.uchar)(unsafe.Pointer(&data[0])),
		C.int(len(data)),
		&transferred,
		C.uint(timeout/time.Millisecond),
	)

	if rc < 0 {
		err = UsbError(rc)
	}
	n = int(transferred)

	return
}

// Recv data from interface. Returns count of bytes actually transmitted
// and error, if any
//
// Note, if data size is not 512-byte aligned, and device has more data,
// that fits the provided buffer, LIBUSB_ERROR_OVERFLOW error may occur
func (iface *UsbInterface) Recv(data []byte,
	timeout time.Duration) (n int, err error) {

	var transferred C.int

	rc := C.libusb_bulk_transfer(
		(*C.libusb_device_handle)(iface.devhandle),
		0, //iface.addr.In | C.LIBUSB_ENDPOINT_IN
		(*C.uchar)(unsafe.Pointer(&data[0])),
		C.int(len(data)),
		&transferred,
		C.uint(timeout/time.Millisecond),
	)

	if rc < 0 {
		err = UsbError(rc)
	}
	n = int(transferred)

	return
}
