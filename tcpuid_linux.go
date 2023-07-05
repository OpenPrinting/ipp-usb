/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * UID discovery for TCP connection over loopback -- Linux version
 */

package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
	"unsafe"
)

// #include <linux/inet_diag.h>
// #include <linux/in.h>
// #include <linux/netlink.h>
// #include <linux/sock_diag.h>
// #include <netinet/tcp.h>
// #include <stdint.h>
// #include <sys/socket.h>
//
// typedef struct inet_diag_req_v2 inet_diag_req_v2_struct;
// typedef struct inet_diag_sockid inet_diag_sockid_struct;
// typedef struct nlmsgerr         nlmsgerr_struct;
// typedef struct inet_diag_msg    inet_diag_msg_struct;
//
// typedef struct {
//     struct nlmsghdr         hdr;
//     struct inet_diag_req_v2 data;
// } sock_diag_request;
//
import "C"

// TCPClientUIDSupported tells if TCPClientUID supported on this platform
func TCPClientUIDSupported() bool {
	return true
}

// TCPClientUID obtains UID of client process that created
// TCP connection over the loopback interface
func TCPClientUID(client, server *net.TCPAddr) (int, error) {
	// Open NETLINK_SOCK_DIAG socket
	sock, err := sockDiagOpen()
	if err != nil {
		return -1, err
	}

	defer syscall.Close(sock)

	// Prepare request
	rq := C.sock_diag_request{}

	rq.hdr.nlmsg_len = C.uint32_t(unsafe.Sizeof(rq))
	rq.hdr.nlmsg_type = C.uint16_t(C.SOCK_DIAG_BY_FAMILY)
	rq.hdr.nlmsg_flags = C.uint16_t(C.NLM_F_REQUEST)

	rq.data.sdiag_family = C.AF_INET6
	rq.data.sdiag_protocol = C.IPPROTO_TCP
	rq.data.idiag_states = 1 << C.TCP_ESTABLISHED
	rq.data.id.idiag_sport = C.uint16_t(toBE16((uint16(client.Port))))
	rq.data.id.idiag_dport = C.uint16_t(toBE16((uint16(server.Port))))
	rq.data.id.idiag_cookie[0] = C.INET_DIAG_NOCOOKIE
	rq.data.id.idiag_cookie[1] = C.INET_DIAG_NOCOOKIE

	copy((*[16]byte)(unsafe.Pointer(&rq.data.id.idiag_src))[:],
		client.IP.To16())
	copy((*[16]byte)(unsafe.Pointer(&rq.data.id.idiag_dst))[:],
		server.IP.To16())

	// Send request
	rqData := (*[unsafe.Sizeof(rq)]byte)(unsafe.Pointer(&rq))
	rqAddr := &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}
	err = syscall.Sendto(sock, rqData[:], 0, rqAddr)
	if err != nil {
		return -1, fmt.Errorf("sock_diag: sendto(): %s", err)
	}

	// Receive responses
	buf := make([]byte, syscall.Getpagesize())
	for {
		num, _, err := syscall.Recvfrom(sock, buf, 0)
		if err != nil {
			return -1, fmt.Errorf("sock_diag: recvfrom(): %s", err)
		}

		msgs, err := syscall.ParseNetlinkMessage(buf[:num])
		if err != nil {
			return -1, fmt.Errorf("sock_diag: can't parse response")
		}

		for _, msg := range msgs {
			data := unsafe.Pointer(&msg.Data[0])
			switch msg.Header.Type {
			case syscall.NLMSG_ERROR:
				rsp := (*C.nlmsgerr_struct)(data)
				err = syscall.Errno(-rsp.error)
				return -1, err

			case uint16(C.SOCK_DIAG_BY_FAMILY):
				rsp := (*C.inet_diag_msg_struct)(data)
				return int(rsp.idiag_uid), nil
			}
		}
	}
}

// sockDiagOpen opens NETLINK_SOCK_DIAG socket
func sockDiagOpen() (int, error) {
	const stype = syscall.SOCK_DGRAM | syscall.SOCK_CLOEXEC
	const proto = int(C.NETLINK_SOCK_DIAG)

	sock, err := syscall.Socket(syscall.AF_NETLINK, stype, proto)
	if err != nil {
		return -1, fmt.Errorf("sock_diag: socket(): %s", err)
	}

	sa := &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}
	err = syscall.Bind(sock, sa)
	if err != nil {
		syscall.Close(sock)
		return -1, fmt.Errorf("sock_diag: bind(): %s", err)
	}

	return sock, nil
}

// toBE16 converts uint16 to big endian
func toBE16(in uint16) uint16 {
	var out uint16
	p := (*[2]byte)(unsafe.Pointer(&out))
	binary.BigEndian.PutUint16(p[:], in)
	return out
}
