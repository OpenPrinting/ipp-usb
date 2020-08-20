MANDIR = /usr/share/man/
MANPAGE = ipp-usb.8

# Merge DESTDIR and PREFIX
PREFIX := $(abspath $(DESTDIR)/$(PREFIX))
ifeq ($(PREFIX),/)
        PREFIX :=
endif

all:
	-gotags -R . > tags
	go build -ldflags "-s -w" -tags nethttpomithttp2

man:	$(MANPAGE)

$(MANPAGE): $(MANPAGE).md
	ronn --roff --manual=$@ $<

install:
	install -s -D -t $(PREFIX)/sbin ipp-usb
	install -m 644 -D -t $(PREFIX)/lib/udev/rules.d systemd-udev/*.rules
	install -m 644 -D -t $(PREFIX)/lib/systemd/system systemd-udev/*.service
	install -m 644 -D -t $(PREFIX)/etc/ipp-usb ipp-usb.conf
	mkdir -p $(PREFIX)/$(MANDIR)/man8
	gzip <$(MANPAGE) > $(PREFIX)$(MANDIR)/man8/$(MANPAGE).gz

