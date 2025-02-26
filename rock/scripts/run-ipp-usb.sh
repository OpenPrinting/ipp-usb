#!/bin/sh

#set -e -x

# Create needed directories (ignore errors)
mkdir -p /etc/ipp-usb || :
mkdir -p /var/log/ipp-usb || :
mkdir -p /var/lock || :
mkdir -p /var/dev || :
mkdir -p /usr/share/ipp-usb/quirks || :

# Put config files in place (do not overwrite existing user config)
yes no | cp -i /usr/share/ipp-usb/quirks/* /etc/ipp-usb/quirks >/dev/null 2>&1 || :
if [ ! -f /etc/ipp-usb/ipp-usb.conf ]; then
    cp /usr/share/ipp-usb/ipp-usb.conf /etc/ipp-usb/ >/dev/null 2>&1 || :
fi

# Run ipp-usb with the provided command-line arguments
exec /usr/sbin/ipp-usb "$@"
