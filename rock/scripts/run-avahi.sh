#!/bin/sh

set -eux

# Start dbus-daemon in the background
/usr/bin/dbus-daemon --system --nofork &

# Wait for the D-Bus system bus to be ready
while [ ! -e /var/run/dbus/system_bus_socket ]; do
  echo "Waiting for dbus-daemon to initialize..."
  sleep 1
done

# Start avahi-daemon after dbus-daemon is ready
/usr/sbin/avahi-daemon -f /etc/avahi/avahi-daemon.conf --no-drop-root --debug

# Keep the container running
exec tail -f /dev/null