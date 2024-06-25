/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Authentication
 */

package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AuthUIDRule represents a single rule for client authentication
// based on client UID
type AuthUIDRule struct {
	Name    string  // @name means group, * means any
	Allowed AuthOps // Allowed operations
}

// IsUser tells if rule is a user rule
func (rule *AuthUIDRule) IsUser() bool {
	return !rule.IsGroup()
}

// IsGroup tells if rule is a group rule
func (rule *AuthUIDRule) IsGroup() bool {
	return strings.HasPrefix(rule.Name, "@")
}

// MatchUser matches rule against user name
func (rule *AuthUIDRule) MatchUser(name string) AuthOps {
	if rule.IsGroup() {
		return 0
	}

	if rule.Name == "*" || rule.Name == name {
		return rule.Allowed
	}

	return 0
}

// MatchGroup matches rule against group name
func (rule *AuthUIDRule) MatchGroup(name string) AuthOps {
	if !rule.IsGroup() {
		return 0
	}

	ruleName := rule.Name[1:] // Strip leading '@'
	if ruleName == "*" || ruleName == name {
		return rule.Allowed
	}

	return 0
}

// AuthOps is bitmask of allowed operations
type AuthOps int

// AuthOps values
const (
	AuthOpsConfig AuthOps = 1 << iota // Configuration web console
	AuthOpsFax                        // Faxing
	AuthOpsPrint                      // Printing
	AuthOpsScan                       // Scanning

	// All and None of above
	AuthOpsAll = AuthOpsConfig | AuthOpsFax | AuthOpsPrint |
		AuthOpsScan
	AuthOpsNone AuthOps = 0
)

// String returns string representation of AuthOps flags, for debugging.
func (ops AuthOps) String() string {
	if ops == 0 {
		return "none"
	}

	s := []string{}

	if ops&AuthOpsConfig != 0 {
		s = append(s, "config")
	}

	if ops&AuthOpsFax != 0 {
		s = append(s, "fax")
	}

	if ops&AuthOpsPrint != 0 {
		s = append(s, "print")
	}

	if ops&AuthOpsScan != 0 {
		s = append(s, "scan")
	}

	return strings.Join(s, ",")
}

// AuthUIDinfo is the resolved and cached UID info, for matching
type AuthUIDinfo struct {
	UsrNames []string  // User (numerical and symbolic) names
	GrpNames []string  // Group names (numerical and symbolic)
	expires  time.Time // Expiration time, for caching
}

// authUIDinfoCache contains authUIDinfo cache, indexed by UID
var (
	authUIDinfoCache     = make(map[int]*AuthUIDinfo)
	authUIDinfoCacheLock sync.Mutex
)

// authUIDinfoCacheTTL is the expiration timeout for authUIDinfoCache
const authUIDinfoCacheTTL = 2 * time.Second

// AuthUIDinfoLookup performs AuthUIDinfo lookup by UID.
func AuthUIDinfoLookup(uid int) (*AuthUIDinfo, error) {
	// Lookup authUIDinfoCache
	authUIDinfoCacheLock.Lock()
	info := authUIDinfoCache[uid]
	authUIDinfoCacheLock.Unlock()

	if info != nil && info.expires.After(time.Now()) {
		return info, nil
	}

	// Resolve user names for matching
	// Also populates grpIDs with numeric group IDs
	usrNames := []string{strconv.Itoa(uid)}
	grpIDs := []string{}

	usr, err := user.LookupId(usrNames[0])
	if err != nil {
		return nil, err
	}

	usrNames = append(usrNames, usr.Username)
	grpIDs = append(grpIDs, usr.Gid)

	grpids, err := usr.GroupIds()
	if err != nil {
		return nil, err
	}

	grpIDs = append(grpIDs, grpids...)

	// Resolve group IDs to names
	grpNames := append([]string{}, grpIDs...)
	for _, gid := range grpIDs {
		grp, err := user.LookupGroupId(gid)
		if err != nil {
			return nil, err
		}

		grpNames = append(grpNames, grp.Name)
	}

	// Update cache
	info = &AuthUIDinfo{
		UsrNames: usrNames,
		GrpNames: grpNames,
		expires:  time.Now().Add(authUIDinfoCacheTTL),
	}

	authUIDinfoCacheLock.Lock()
	authUIDinfoCache[uid] = info
	authUIDinfoCacheLock.Unlock()

	// Return the answer
	return info, nil
}

// AuthUID returns operations allowed to client with given UID
// uid == -1 indicates that UID is not available (i.e., external
// connection)
func AuthUID(info *AuthUIDinfo) AuthOps {
	// Everything is allowed if authentication is not configured
	if Conf.ConfAuthUID == nil {
		return AuthOpsAll
	}

	// Apply rules
	allowed := AuthOpsNone

	for _, rule := range Conf.ConfAuthUID {
		if rule.IsUser() {
			for _, usr := range info.UsrNames {
				allowed |= rule.MatchUser(usr)
			}
		} else {
			for _, grp := range info.GrpNames {
				allowed |= rule.MatchGroup(grp)
			}
		}
	}

	return allowed
}

// AuthHTTPRequest performs authentication for the incoming
// HTTP request
//
// On success, status is http.StatusOK and err is nil.
// Otherwise, status is appropriate for HTTP error response,
// and err explains the reason
func AuthHTTPRequest(log *Logger,
	client, server *net.TCPAddr,
	rq *http.Request) (status int, err error) {

	// Guess the operation by URL
	post := rq.Method == "POST"
	ops := AuthOpsConfig // The default
	switch {
	case post && strings.HasPrefix(rq.URL.Path, "/ipp/print"):
		ops = AuthOpsPrint
	case post && strings.HasPrefix(rq.URL.Path, "/ipp/faxout"):
		ops = AuthOpsFax
	case strings.HasPrefix(rq.URL.Path, "/eSCL"):
		ops = AuthOpsScan
	}

	log.Debug(' ', "auth: operation requested: %s (HTTP %s %s)",
		ops, rq.Method, rq.URL)

	// Check if client and server addresses are both local
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		err = fmt.Errorf("can't get local IP addresses: %s", err)
		log.Error('!', "auth: %s", err)

		return http.StatusInternalServerError, err
	}

	clientIsLocal := client.IP.IsLoopback()
	serverIsLocal := server.IP.IsLoopback()

	for _, addr := range addrs {
		if clientIsLocal && serverIsLocal {
			// Both addresses known to be local,
			// we don't need to continue
			break
		}

		if ip, ok := addr.(*net.IPNet); ok {
			if client.IP.Equal(ip.IP) {
				clientIsLocal = true
			}

			if server.IP.Equal(ip.IP) {
				serverIsLocal = true
			}
		}
	}

	log.Debug(' ', "auth: address check:")
	log.Debug(' ', "  client-addr %s local=%v", client.IP, clientIsLocal)
	log.Debug(' ', "  server-addr %s local=%v", server.IP, serverIsLocal)

	// Obtain UID
	uid := -1
	if clientIsLocal && serverIsLocal {
		if TCPClientUIDSupported() {
			uid, err = TCPClientUID(client, server)
			if err != nil {
				err = fmt.Errorf("can't get client UID: %s",
					err)
				log.Error('!', "auth: %s", err)
				return http.StatusInternalServerError, err
			}
		}

		log.Debug(' ', "auth: client UID=%d", uid)
	} else {
		log.Debug(' ', "auth: client UID=%d (non-local connection)",
			uid)
	}

	// Lookup UID info
	info, err := AuthUIDinfoLookup(uid)
	if err != nil {
		err = fmt.Errorf("can't resolve UID %d: %s", uid, err)
		log.Error('!', "auth: %s", err)
		return 0, err
	}

	log.Debug(' ', "auth: UID %d resolved:", uid)
	log.Debug(' ', "  user names:  %s", strings.Join(info.UsrNames, ","))
	log.Debug(' ', "  group names: %s", strings.Join(info.GrpNames, ","))

	// Authenticate
	allowed := AuthUID(info)
	log.Debug(' ', "auth: allowed operations: %s", allowed)

	if ops&allowed != AuthOpsNone {
		log.Debug(' ', "auth: access granted")
		return http.StatusOK, nil
	}

	err = errors.New("Operation not allowed. See ipp-usb.conf for details")
	log.Error('!', "auth: %s", err)

	return http.StatusForbidden, err
}
