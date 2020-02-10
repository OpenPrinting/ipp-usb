MANDIR = /usr/share/man/
MANPAGE = ipp-usb.1

# Merge DESTDIR and PREFIX
PREFIX := $(abspath $(DESTDIR)/$(PREFIX))
ifeq ($(PREFIX),/)
        PREFIX :=
endif

all:
	-gotags -R . > tags
	go build -ldflags "-s -w"

man:	ipp-usb.1

$(MANPAGE): $(MANPAGE).md
	ronn --roff --manual=$@ $<

install:
	install -s -D -t $(PREFIX)/sbin ipp-usb
	install -D -t $(PREFIX)/lib/udev/rules.d systemd-udev/*.rules
	install -D -t $(PREFIX)/lib/systemd/system systemd-udev/*.service
	install -D -t $(PREFIX)/etc/ipp-usb ipp-usb.conf
	mkdir -p $(PREFIX)/$(MANDIR)/man1
	gzip <$(MANPAGE) > $(PREFIX)$(MANDIR)/man1/$(MANPAGE).gz

