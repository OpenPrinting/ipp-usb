#!/bin/sh
set -eux

echo "Creating system users..."

# Check if users exist before attempting to create them
if ! id -u systemd-resolve >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin systemd-resolve
fi

if ! id -u systemd-network >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin systemd-network
fi

echo "Ensuring necessary directories exist..."

# Ensure /run/dbus exists with correct permissions
if [ ! -d /run/dbus ]; then
    mkdir -p /run/dbus
    chmod 755 /run/dbus
    chown root:root /run/dbus
fi

echo "Starting dbus service..."

# Start dbus and verify it's running
service dbus start
if ! pgrep -x "dbus-daemon" >/dev/null; then
    echo "Failed to start dbus-daemon!" >&2
    exit 1
fi

echo "Starting avahi-daemon..."

# Start avahi-daemon and ensure it's running
avahi-daemon --daemonize --no-drop-root
if ! pgrep -x "avahi-daemon" >/dev/null; then
    echo "Failed to start avahi-daemon!" >&2
    exit 1
fi

echo "Services started successfully."

# Keep the container alive using a foreground process
exec sleep infinity
