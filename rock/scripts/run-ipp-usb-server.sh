#!/bin/sh

#set -e -x

# Create needed directories (ignore errors)
mkdir -p /etc/ipp-usb 
mkdir -p /var/log/ipp-usb 
mkdir -p /var/lock
mkdir -p /var/dev
mkdir -p /usr/share/ipp-usb/quirks 

# Put config files in place (do not overwrite existing user config)
cp -r /usr/share/ipp-usb/* /etc/ipp-usb/
if [ ! -f /etc/ipp-usb/ipp-usb.conf ]; then
    cp etc/ipp-usb/ipp-usb.conf /etc/ipp-usb/
fi

# Monitor appearing/disappearing of USB devices
udevadm monitor -k -s usb | while read START OP DEV REST; do
    START_IPP_USB=0
    if test "$START" = "KERNEL"; then
        # First lines of "udevadm monitor" output, check for already plugged
        # devices. Consider only IPP-over-USB devices (interface 7/1/4)
        if [ `udevadm trigger -v -n --subsystem-match=usb --property-match=ID_USB_INTERFACES='*:070104:*' | wc -l` -gt 0 ]; then
            # IPP-over-USB device already connected
            START_IPP_USB=1
        fi
    elif test "$OP" = "add"; then
        # New device got added
        if [ -z $DEV ]; then
            # Missing device path
            continue
        else
            # Does the device support IPP-over-USB (interface 7/1/4)?
            # Retry 5 times as sometimes the ID_USB_INTERFACES property is not
            # immediately set
            for i in 1 2 3 4 5; do
                # Give some time for ID_USB_INTERFACES property to appear
                sleep 0.02
                # Check ID_USB_INTERFACE for 7/1/4 interface
                if udevadm info -q property -p $DEV | grep -q ID_USB_INTERFACES=.*:070104:.*; then
                    # IPP-over-USB device got connected now
                    START_IPP_USB=1
                    break
                fi
            done
        fi
    fi
    if [ $START_IPP_USB = 1 ]; then
        # Start ipp-usb-server
        /usr/sbin/ipp-usb udev
    fi
done