#!/bin/sh

set -eux

# Create needed directories (ignore errors)
mkdir -p /etc/ipp-usb || :
mkdir -p /var/log/ipp-usb || :
mkdir -p /var/ipp-usb/lock || :
mkdir -p /var/ipp-usb/dev || :
mkdir -p /usr/share/ipp-usb/quirks || :

# Put config files in place (do not overwrite existing user config)
yes no | cp -i /usr/share/ipp-usb/quirks/* /etc/ipp-usb/quirks >/dev/null 2>&1 || :
if [ ! -f /etc/ipp-usb/ipp-usb.conf ]; then
    cp /usr/share/ipp-usb/ipp-usb.conf /etc/ipp-usb/ >/dev/null 2>&1 || :
fi

# Wait for avahi-daemon to initialize
while true; do
    if [ -f "/var/run/avahi-daemon/pid" ] || [ -f "/run/avahi-daemon/pid" ]; then
        echo "[$(date)] avahi-daemon is active. Starting ipp-usb..."
        break
    fi
    echo "[$(date)] Waiting for avahi-daemon to initialize..."
    sleep 1
done

# Run ipp-usb with logging
echo "[$(date)] Running ipp-usb..."

# Run ipp-usb with the provided command-line arguments
exec /usr/sbin/ipp-usb "$@"
