all:
	-gotags -R . > tags
	go build -ldflags "-s -w"

install:
	install -s -D -t $(PREFIX)/sbin ipp-usb
	install -D -t $(PREFIX)/lib/udev/rules.d systemd-udev/*.rules
	install -D -t $(PREFIX)/lib/systemd/system systemd-udev/*.service
	install -D -t $(PREFIX)/etc/ipp-usb ipp-usb.conf
