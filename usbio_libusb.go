/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * USB low-level I/O. Cgo implementation on a top of libusb
 */

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// #cgo pkg-config: libusb-1.0
// #include <libusb.h>
//
// int libusbHotplugCallback (libusb_context *ctx, libusb_device *device,
//     libusb_hotplug_event event, void *user_data);
// void libusbTransferCallback (struct libusb_transfer *transfer);
//
// typedef struct libusb_device_descriptor libusb_device_descriptor_struct;
// typedef struct libusb_config_descriptor libusb_config_descriptor_struct;
// typedef struct libusb_interface libusb_interface_struct;
// typedef struct libusb_interface_descriptor libusb_interface_descriptor_struct;
// typedef struct libusb_endpoint_descriptor libusb_endpoint_descriptor_struct;
// typedef struct libusb_transfer libusb_transfer_struct;
//
// // Note, libusb_strerror accepts enum libusb_error argument, which
// // unfortunately behaves differently depending on target OS and compiler
// // version (sometimes as C.int, sometimes as int32). Looks like cgo
// // bug. Wrapping this function into this simple wrapper should
// // fix the problem. See #18 for details
// static inline const char*
// libusb_strerror_wrapper (int code) {
//     return libusb_strerror(code);
// }
import "C"

// UsbError represents USB error
type UsbError struct {
	Func string
	Code UsbErrCode
}

// Error describes a libusb error. It implements error interface
func (err UsbError) Error() string {
	return err.Func + ": " + err.Code.String()
}

// UsbErrCode represents USB I/O error code
type UsbErrCode int

// UsbErrCode constants
const (
	UsbEIO           UsbErrCode = C.LIBUSB_ERROR_IO
	UsbEInval                   = C.LIBUSB_ERROR_INVALID_PARAM
	UsbEAccess                  = C.LIBUSB_ERROR_ACCESS
	UsbENoDev                   = C.LIBUSB_ERROR_NO_DEVICE
	UsbENotFound                = C.LIBUSB_ERROR_NOT_FOUND
	UsbEBusy                    = C.LIBUSB_ERROR_BUSY
	UsbETimeout                 = C.LIBUSB_ERROR_TIMEOUT
	UsbEOverflow                = C.LIBUSB_ERROR_OVERFLOW
	UsbEPipe                    = C.LIBUSB_ERROR_PIPE
	UsbEIntr                    = C.LIBUSB_ERROR_INTERRUPTED
	UsbENomem                   = C.LIBUSB_ERROR_NO_MEM
	UsbENotSupported            = C.LIBUSB_ERROR_NOT_SUPPORTED
	UsbEOther                   = C.LIBUSB_ERROR_OTHER
)

// String returns string representation of error code
func (err UsbErrCode) String() string {
	return C.GoString(C.libusb_strerror_wrapper(C.int(err)))
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

	// libusbTransferDoneMap contains a map of completion channels,
	// associated with each active libusb_transfer.
	//
	// The libusbTransferCallback uses this map to indicate transfer
	// completion
	//
	// This is required, because CGo is very restrictive in whatever
	// can be saved in pointer passed to the C side.
	libusbTransferDoneMap = make(map[*C.libusb_transfer_struct]chan struct{})

	// libusbTransferDoneLock protects multithreaded access to
	// the libusbTransferDoneMap
	libusbTransferDoneLock sync.Mutex

	// UsbHotPlugChan receives USB hotplug event notifications
	UsbHotPlugChan = make(chan struct{}, 1)
)

// UsbInit initializes low-level USB I/O
func UsbInit(nopnp bool) error {
	_, err := libusbContext(nopnp)
	return err
}

// libusbContext returns libusb_context. It
// initializes context on demand.
func libusbContext(nopnp bool) (*C.libusb_context, error) {
	if atomic.LoadInt32(&libusbContextOk) != 0 {
		return libusbContextPtr, nil
	}

	libusbContextLock.Lock()
	defer libusbContextLock.Unlock()

	// Obtain libusb_context
	rc := C.libusb_init(&libusbContextPtr)
	if rc != 0 {
		return nil, UsbError{"libusb_init", UsbErrCode(rc)}
	}

	// Subscribe to hotplug events
	if !nopnp {
		C.libusb_hotplug_register_callback(
			libusbContextPtr, // libusb_context
			C.LIBUSB_HOTPLUG_EVENT_DEVICE_ARRIVED| // events mask
				C.LIBUSB_HOTPLUG_EVENT_DEVICE_LEFT,
			C.LIBUSB_HOTPLUG_NO_FLAGS,  // flags
			C.LIBUSB_HOTPLUG_MATCH_ANY, // vendor_id
			C.LIBUSB_HOTPLUG_MATCH_ANY, // product_id
			C.LIBUSB_HOTPLUG_MATCH_ANY, // dev_class
			C.libusb_hotplug_callback_fn(unsafe.Pointer(C.libusbHotplugCallback)),
			nil, // callback's data
			nil, // deregister handle
		)
	}

	// Start libusb thread (required for hotplug and asynchronous I/O)
	go func() {
		runtime.LockOSThread()
		for {
			C.libusb_handle_events(libusbContextPtr)
		}
	}()

	atomic.StoreInt32(&libusbContextOk, 1)
	return libusbContextPtr, nil
}

// Called by libusb on hotplug event
//
//export libusbHotplugCallback
func libusbHotplugCallback(ctx *C.libusb_context, dev *C.libusb_device,
	event C.libusb_hotplug_event, p unsafe.Pointer) C.int {

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
	case UsbHotPlugChan <- struct{}{}:
	default:
	}

	return 0
}

// Called by libusb on libusb_transfer completion
//
//export libusbTransferCallback
func libusbTransferCallback(xfer *C.libusb_transfer_struct) {
	// Obtain signaling channel
	libusbTransferDoneLock.Lock()
	done := libusbTransferDoneMap[xfer]
	libusbTransferDoneLock.Unlock()

	// Indicate transfer completion by closing the channel
	close(done)
}

// libusbTransferStatusDecode decodes libusb_transfer completion status.
//
// It returns either non-negative actual transfer length or error.
//
// When computing an error, it consults context.Context cancellation
// and expiration status.
func libusbTransferStatusDecode(ctx context.Context,
	xfer *C.libusb_transfer_struct) (int, error) {

	var rc C.int
	switch xfer.status {
	// Handle special cases
	case C.LIBUSB_TRANSFER_COMPLETED:
		// Successful completion. Return no error regardless
		// of the context.Context status.
		return int(xfer.actual_length), nil

	case C.LIBUSB_TRANSFER_CANCELLED:
		switch {
		case ctx.Err() != nil:
			return 0, ctx.Err()
		default:
			rc = C.LIBUSB_ERROR_IO
		}

	case C.LIBUSB_TRANSFER_TIMED_OUT:
		// There may be a race between context.Context
		// expiration and libusb timeout. Be consistent
		// in returned error.
		return 0, context.DeadlineExceeded

	// Handle other cases
	case C.LIBUSB_TRANSFER_STALL:
		rc = C.LIBUSB_ERROR_PIPE

	case C.LIBUSB_TRANSFER_OVERFLOW:
		rc = C.LIBUSB_ERROR_OVERFLOW

	case C.LIBUSB_TRANSFER_NO_DEVICE:
		rc = C.LIBUSB_ERROR_NO_DEVICE

	case C.LIBUSB_TRANSFER_ERROR:
		rc = C.LIBUSB_ERROR_IO

	default:
		rc = C.LIBUSB_ERROR_OTHER
	}

	return 0, UsbError{"libusb_submit_transfer", UsbErrCode(rc)}
}

// libusbTransferAlloc allocates a libusb_transfer.
//
// On success, it allocates a completion channel as well and adds
// it into the libusbTransferDoneMap.
func libusbTransferAlloc() (*C.libusb_transfer_struct, chan struct{}, error) {
	xfer := C.libusb_alloc_transfer(0)
	if xfer == nil {
		return nil, nil, UsbError{"libusb_alloc_transfer", UsbENomem}
	}

	doneChan := make(chan struct{})

	libusbTransferDoneLock.Lock()
	libusbTransferDoneMap[xfer] = doneChan
	libusbTransferDoneLock.Unlock()

	return xfer, doneChan, nil
}

// libusbTransferFree removed libusb_transfer from the libusbTransferDoneMap
// and releases its memory.
func libusbTransferFree(xfer *C.libusb_transfer_struct) {
	libusbTransferDoneLock.Lock()
	delete(libusbTransferDoneMap, xfer)
	libusbTransferDoneLock.Unlock()

	C.libusb_free_transfer(xfer)
}

// UsbCheckIppOverUsbDevices returns true if there are some IPP-over-USB devices
func UsbCheckIppOverUsbDevices() bool {
	descs, _ := UsbGetIppOverUsbDeviceDescs()
	return len(descs) != 0
}

// UsbGetIppOverUsbDeviceDescs return list of IPP-over-USB
// device descriptors
func UsbGetIppOverUsbDeviceDescs() (map[UsbAddr]UsbDeviceDesc, error) {
	// Obtain libusb context
	ctx, err := libusbContext(false)
	if err != nil {
		return nil, err
	}

	// Obtain list of devices
	var devlist **C.libusb_device
	cnt := C.libusb_get_device_list(ctx, &devlist)
	if cnt < 0 {
		return nil, UsbError{"libusb_get_device_list", UsbErrCode(cnt)}
	}
	defer C.libusb_free_device_list(devlist, 1)

	// Convert devlist to slice.
	// See https://github.com/golang/go/wiki/cgo#turning-c-arrays-into-go-slices
	devs := (*[1 << 28]*C.libusb_device)(unsafe.Pointer(devlist))[:cnt:cnt]

	// Now build list of addresses
	descs := make(map[UsbAddr]UsbDeviceDesc)

	for _, dev := range devs {
		desc, err := libusbBuildUsbDeviceDesc(dev)

		// Note, ignore devices, if we don't have
		// at least 2 IPP over USB interfaces
		// (which should not happen in real life,
		// but just in case...
		if err == nil && len(desc.IfAddrs) >= 2 {
			descs[desc.UsbAddr] = desc
		}
	}

	return descs, nil
}

// libusbBuildUsbDeviceDesc builds device descriptor
func libusbBuildUsbDeviceDesc(dev *C.libusb_device) (UsbDeviceDesc, error) {
	var cDesc C.libusb_device_descriptor_struct
	var desc UsbDeviceDesc

	// Obtain device descriptor
	rc := C.libusb_get_device_descriptor(dev, &cDesc)
	if rc < 0 {
		return desc, UsbError{"libusb_get_device_descriptor", UsbErrCode(rc)}
	}

	// Decode device descriptor
	desc.Bus = int(C.libusb_get_bus_number(dev))
	desc.Address = int(C.libusb_get_device_address(dev))
	desc.Config = -1

	// Roll over configs/interfaces/alt settings/endpoins
	for cfgNum := 0; cfgNum < int(cDesc.bNumConfigurations); cfgNum++ {
		var conf *C.libusb_config_descriptor_struct
		rc = C.libusb_get_config_descriptor(dev, C.uint8_t(cfgNum), &conf)
		if rc == 0 {
			// Make sure we use the same configuration for all interfaces
			if desc.Config >= 0 && desc.Config != int(conf.bConfigurationValue) {
				continue
			}

			ifcnt := conf.bNumInterfaces
			ifaces := (*[256]C.libusb_interface_struct)(
				unsafe.Pointer(conf._interface))[:ifcnt:ifcnt]

			for _, iface := range ifaces {
				altcnt := iface.num_altsetting
				alts := (*[256]C.libusb_interface_descriptor_struct)(
					unsafe.Pointer(iface.altsetting))[:altcnt:altcnt]

				for _, alt := range alts {
					// Build and append UsbIfDesc
					ifdesc := UsbIfDesc{
						Vendor:   uint16(cDesc.idVendor),
						Product:  uint16(cDesc.idProduct),
						Config:   int(conf.bConfigurationValue),
						IfNum:    int(alt.bInterfaceNumber),
						Alt:      int(alt.bAlternateSetting),
						Class:    int(alt.bInterfaceClass),
						SubClass: int(alt.bInterfaceSubClass),
						Proto:    int(alt.bInterfaceProtocol),
					}

					desc.IfDescs = append(desc.IfDescs, ifdesc)

					// We are only interested in IPP-over-USB
					// interfaces, i.e., LIBUSB_CLASS_PRINTER,
					// SubClass 1, Protocol 4
					if ifdesc.IsIppOverUsb() {
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
							desc.Config = int(conf.bConfigurationValue)
							addr := UsbIfAddr{
								UsbAddr: desc.UsbAddr,
								Num:     int(alt.bInterfaceNumber),
								Alt:     int(alt.bAlternateSetting),
								In:      in,
								Out:     out,
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

// UsbDevHandle represents libusb_device_handle
type UsbDevHandle C.libusb_device_handle

// UsbOpenDevice opens device by device descriptor
func UsbOpenDevice(desc UsbDeviceDesc) (*UsbDevHandle, error) {
	// Obtain libusb context
	ctx, err := libusbContext(false)
	if err != nil {
		return nil, err
	}

	// Obtain list of devices
	var devlist **C.libusb_device
	cnt := C.libusb_get_device_list(ctx, &devlist)
	if cnt < 0 {
		return nil, UsbError{"libusb_get_device_list", UsbErrCode(cnt)}
	}
	defer C.libusb_free_device_list(devlist, 1)

	// Convert devlist to slice.
	devs := (*[1 << 28]*C.libusb_device)(unsafe.Pointer(devlist))[:cnt:cnt]

	// Find and open a device
	for _, dev := range devs {
		bus := int(C.libusb_get_bus_number(dev))
		address := int(C.libusb_get_device_address(dev))

		if desc.Bus == bus && desc.Address == address {
			// Open device
			var devhandle *C.libusb_device_handle
			rc := C.libusb_open(dev, &devhandle)
			if rc < 0 {
				return nil, UsbError{"libusb_open", UsbErrCode(rc)}
			}

			return (*UsbDevHandle)(devhandle), nil
		}
	}

	return nil, UsbError{"libusb_get_device_list", UsbENotFound}
}

// Configure prepares the device for further work:
//   - set proper USB configuration
//   - detach kernel driver
func (devhandle *UsbDevHandle) Configure(desc UsbDeviceDesc) error {
	// Detach kernel driver
	err := (*UsbDevHandle)(devhandle).detachKernelDriver()
	if err != nil {
		return err
	}

	// Set configuration
	rc := C.libusb_set_configuration(
		(*C.libusb_device_handle)(devhandle), C.int(desc.Config))

	if rc < 0 {
		return UsbError{"libusb_set_configuration", UsbErrCode(rc)}
	}

	// Printer may require some time to switch configuration
	time.Sleep(time.Second / 4)

	return nil
}

// detachKernelDriver detaches kernel driver from all interfaces
// of current configuration
func (devhandle *UsbDevHandle) detachKernelDriver() error {
	C.libusb_set_auto_detach_kernel_driver(
		(*C.libusb_device_handle)(devhandle), 1)

	ifnums, err := devhandle.currentInterfaces()
	if err != nil {
		return err
	}

	for _, ifnum := range ifnums {
		rc := C.libusb_detach_kernel_driver(
			(*C.libusb_device_handle)(devhandle), C.int(ifnum))
		if rc == C.LIBUSB_ERROR_NOT_FOUND {
			rc = 0
		}

		if rc < 0 {
			return UsbError{"libusb_detach_kernel_driver", UsbErrCode(rc)}
		}
	}

	return nil
}

// libusbCurrentInterfaces builds list of interfaces in current configuration
func (devhandle *UsbDevHandle) currentInterfaces() ([]int, error) {
	dev := C.libusb_get_device((*C.libusb_device_handle)(devhandle))

	// Obtain device descriptor
	var cDesc C.libusb_device_descriptor_struct
	rc := C.libusb_get_device_descriptor(dev, &cDesc)
	if rc < 0 {
		return nil, UsbError{"libusb_get_device_descriptor", UsbErrCode(rc)}
	}

	// Get current configuration
	var config C.int
	rc = C.libusb_get_configuration((*C.libusb_device_handle)(devhandle), &config)
	if rc < 0 {
		return nil, UsbError{"libusb_get_configuration", UsbErrCode(rc)}
	}

	// Get configuration descriptor
	var conf *C.libusb_config_descriptor_struct

	for cfgNum := 0; cfgNum < int(cDesc.bNumConfigurations); cfgNum++ {
		rc = C.libusb_get_config_descriptor(dev, C.uint8_t(cfgNum), &conf)
		if rc < 0 {
			return nil, UsbError{"libusb_get_configuration", UsbErrCode(rc)}
		}

		if conf.bConfigurationValue == C.uint8_t(config) {
			break
		}

		C.libusb_free_config_descriptor(conf)
		conf = nil
	}

	if conf == nil {
		return nil, errors.New("libusb: unable to find current configuration in device descriptor")
	}

	defer C.libusb_free_config_descriptor(conf)

	// Build list of interface numbers
	ifcnt := conf.bNumInterfaces
	ifaces := (*[256]C.libusb_interface_struct)(
		unsafe.Pointer(conf._interface))[:ifcnt:ifcnt]
	ifnumbers := make([]int, 0, ifcnt)

	for _, iface := range ifaces {
		alt := iface.altsetting

		ifnumbers = append(ifnumbers, int(alt.bInterfaceNumber))
	}

	return ifnumbers, nil
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

	var cDesc C.libusb_device_descriptor_struct
	var info UsbDeviceInfo

	// Obtain device descriptor
	rc := C.libusb_get_device_descriptor(dev, &cDesc)
	if rc < 0 {
		return info, UsbError{"libusb_get_device_descriptor", UsbErrCode(rc)}
	}

	// Decode device descriptor
	info.Vendor = uint16(cDesc.idVendor)
	info.Product = uint16(cDesc.idProduct)
	info.BasicCaps = devhandle.usbIppBasicCaps()

	buf := make([]byte, 256)

	strings := []struct {
		idx C.uint8_t
		str *string
	}{
		{cDesc.iManufacturer, &info.Manufacturer},
		{cDesc.iProduct, &info.ProductName},
		{cDesc.iSerialNumber, &info.SerialNumber},
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

	info.PortNum = int(C.libusb_get_port_number(dev))

	info.FixUp()

	return info, nil
}

// usbIppBasicCaps reads and decodes printer's
// Class-specific Device Info Descriptor to obtain device
// capabilities; see IPP USB specification, section 4.3 for details
//
// This function never fails. In a case of errors, it fall backs
// to the reasonable default
func (devhandle *UsbDevHandle) usbIppBasicCaps() (caps UsbIppBasicCaps) {
	// Safe default
	caps = UsbIppBasicCapsPrint |
		UsbIppBasicCapsScan |
		UsbIppBasicCapsFax |
		UsbIppBasicCapsAnyHTTP

	// Buffer length
	const bufLen = 256

	// Obtain class-specific Device Info Descriptor
	// See IPP USB specification, section 4.3 for details
	buf := make([]byte, bufLen)
	rc := C.libusb_get_descriptor(
		(*C.libusb_device_handle)(devhandle),
		0x21, 0,
		(*C.uchar)(unsafe.Pointer(&buf[0])),
		bufLen)

	if rc < 0 {
		// Some devices doesn't properly return class-specific
		// device descriptor, so ignore an error
		return
	}

	if rc < 10 {
		// Malformed response, fall back to default
		return
	}

	// Decode basic capabilities bits
	bits := binary.LittleEndian.Uint16(buf[6:8])
	if bits == 0 {
		// Paranoia. If no caps, return default
		return
	}

	return UsbIppBasicCaps(bits)
}

// OpenUsbInterface opens an interface
func (devhandle *UsbDevHandle) OpenUsbInterface(addr UsbIfAddr,
	quirks Quirks) (*UsbInterface, error) {

	// Claim the interface
	rc := C.libusb_claim_interface(
		(*C.libusb_device_handle)(devhandle),
		C.int(addr.Num),
	)
	if rc < 0 {
		return nil, UsbError{"libusb_claim_interface", UsbErrCode(rc)}
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
		return nil, UsbError{"libusb_set_interface_alt_setting", UsbErrCode(rc)}
	}

	return &UsbInterface{
		devhandle: devhandle,
		addr:      addr,
		quirks:    quirks,
	}, nil
}

// UsbInterface represents IPP-over-USB interface
type UsbInterface struct {
	devhandle *UsbDevHandle // Device handle
	addr      UsbIfAddr     // Interface address
	quirks    Quirks        // Device quirks
}

// Close the interface
func (iface *UsbInterface) Close() {
	C.libusb_release_interface(
		(*C.libusb_device_handle)(iface.devhandle),
		C.int(iface.addr.Num),
	)
}

// SoftReset performs interface soft reset, using class-specific
// SOFT_RESET request
//
// This code was inspired by CUPS, and the original comment follows:
//
//	This soft reset is specific to the printer device class and is much less
//	invasive than the general USB reset libusb_reset_device(). Especially it
//	does never happen that the USB addressing and configuration changes. What
//	is actually done is that all buffers get flushed and the bulk IN and OUT
//	pipes get reset to their default states. This clears all stall conditions.
//	See http://cholla.mmto.org/computers/linux/usb/usbprint11.
func (iface *UsbInterface) SoftReset() error {
	rc := C.libusb_control_transfer(
		(*C.libusb_device_handle)(iface.devhandle),
		C.LIBUSB_REQUEST_TYPE_CLASS|
			C.LIBUSB_ENDPOINT_OUT|
			C.LIBUSB_RECIPIENT_OTHER,
		2, 0, C.ushort(iface.addr.Num), nil, 0, 5000)

	if rc < 0 {
		rc = C.libusb_control_transfer(
			(*C.libusb_device_handle)(iface.devhandle),
			C.LIBUSB_REQUEST_TYPE_CLASS|
				C.LIBUSB_ENDPOINT_OUT|
				C.LIBUSB_RECIPIENT_INTERFACE,
			2, 0, C.ushort(iface.addr.Num), nil, 0, 5000)
	}

	if rc < 0 {
		return UsbError{"libusb_control_transfer", UsbErrCode(rc)}
	}

	return nil
}

// Send data to interface. Returns count of bytes actually transmitted
// and error, if any
func (iface *UsbInterface) Send(ctx context.Context,
	data []byte) (n int, err error) {

	// Don't even bother to send, if context already expired
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	// Allocate a libusb_transfer.
	xfer, doneChan, err := libusbTransferAlloc()
	if err != nil {
		return
	}

	defer libusbTransferFree(xfer)

	// Setup bulk transfer
	C.libusb_fill_bulk_transfer(
		xfer,
		(*C.libusb_device_handle)(iface.devhandle),
		C.uint8_t(iface.addr.Out|C.LIBUSB_ENDPOINT_OUT),
		(*C.uchar)(unsafe.Pointer(&data[0])),
		C.int(len(data)),
		C.libusb_transfer_cb_fn(unsafe.Pointer(C.libusbTransferCallback)),
		nil,
		0,
	)

	// Submit transfer
	rc := C.libusb_submit_transfer(xfer)
	if rc < 0 {
		return 0, UsbError{"libusb_submit_transfer", UsbErrCode(rc)}
	}

	// Wait for completion
	select {
	case <-ctx.Done():
		C.libusb_cancel_transfer(xfer)
	case <-doneChan:
	}

	<-doneChan
	n, err = libusbTransferStatusDecode(ctx, xfer)

	return
}

// Recv data from interface. Returns count of bytes actually transmitted
// and error, if any
//
// Note, if data size is not 512-byte aligned, and device has more data,
// that fits the provided buffer, LIBUSB_ERROR_OVERFLOW error may occur
func (iface *UsbInterface) Recv(ctx context.Context,
	data []byte) (n int, err error) {

	// Don't even bother to recv, if context already expired
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	// Some versions of Linux kernel don't allow bulk transfers to
	// be larger that 16kb per URB, and libusb uses some smart-ass
	// mechanism to avoid this limitation.
	//
	// This mechanism seems not to work very reliable on Raspberry Pi
	// (see #3 for details). So just limit bulk reads to 16kb
	const MaxBulkRead = 16384
	if len(data) > MaxBulkRead {
		data = data[0:MaxBulkRead]
	}

	// Allocate a libusb_transfer.
	xfer, doneChan, err := libusbTransferAlloc()
	if err != nil {
		return
	}

	defer libusbTransferFree(xfer)

	// Setup bulk transfer
	C.libusb_fill_bulk_transfer(
		xfer,
		(*C.libusb_device_handle)(iface.devhandle),
		C.uint8_t(iface.addr.In|C.LIBUSB_ENDPOINT_IN),
		(*C.uchar)(unsafe.Pointer(&data[0])),
		C.int(len(data)),
		C.libusb_transfer_cb_fn(unsafe.Pointer(C.libusbTransferCallback)),
		nil,
		0,
	)

	// Submit transfer
	rc := C.libusb_submit_transfer(xfer)
	if rc < 0 {
		return 0, UsbError{"libusb_submit_transfer", UsbErrCode(rc)}
	}

	C.libusb_interrupt_event_handler(libusbContextPtr)

	// Wait for completion
	select {
	case <-ctx.Done():
		C.libusb_cancel_transfer(xfer)
	case <-doneChan:
	}

	<-doneChan
	n, err = libusbTransferStatusDecode(ctx, xfer)

	return
}

// ClearHalt clears "halted" condition of either input or output endpoint
func (iface *UsbInterface) ClearHalt(in bool) error {
	var ep C.uint8_t

	if in {
		ep = C.uint8_t(iface.addr.In | C.LIBUSB_ENDPOINT_IN)
	} else {
		ep = C.uint8_t(iface.addr.Out | C.LIBUSB_ENDPOINT_OUT)
	}

	rc := C.libusb_clear_halt(
		(*C.libusb_device_handle)(iface.devhandle),
		ep)

	if rc < 0 {
		return UsbError{"libusb_clear_halt", UsbErrCode(rc)}
	}

	return nil
}
