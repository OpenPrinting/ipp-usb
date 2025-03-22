#!/bin/sh
set -eux

# Create necessary directories
mkdir -p /etc/ipp-usb || :
mkdir -p /etc/ipp-usb/quirks || :
mkdir -p /var/ipp-usb/lock || :
mkdir -p /var/ipp-usb/dev || :
mkdir -p /var/log/ipp-usb || :

# Copy quirks files if not present
cp -rn /usr/share/ipp-usb/quirks/* /etc/ipp-usb/quirks/ >/dev/null 2>&1 || :
# Put config files in place (do not overwrite existing user config)
if [ ! -f /etc/ipp-usb/ipp-usb.conf ]; then
    cp /etc/ipp-usb.conf /etc/ipp-usb/ >/dev/null 2>&1
fi

# Wait for avahi-daemon
while true; do
    if [ -f "/var/run/avahi-daemon/pid" ] || [ -f "/run/avahi-daemon/pid" ]; then
        echo "[$(date)] avahi-daemon is active. Starting ipp-usb..."
        break
    fi
    echo "[$(date)] Waiting for avahi-daemon to initialize..."
    sleep 1
done

# Start ipp-usb 
exec /usr/sbin/ipp-usb 
