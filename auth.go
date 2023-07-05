/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Authentication
 */

package main

import (
	"os/user"
	"strconv"
	"strings"
	"time"
)

// AuthUIDRule represents a single rule for client authentication
// based on client UID
type AuthUIDRule struct {
	Name    string  // @name means group, * means any
	Allowed AuthOps // Allowed operations
}

// IsGroup tells if rule is a user rule
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

	ruleName := rule.Name[:1] // Strip leading '@'
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

// authUIDinfo is the resolved and cached UID info, for matching
type authUIDinfo struct {
	usrNames []string  // User (numerical and symbolic) names
	grpNames []string  // Group names (numerical and symbolic)
	expires  time.Time // Expiration time
}

// authUIDinfoCache contains authUIDinfo cache, indexed by UID
var authUIDinfoCache = make(map[int]*authUIDinfo)

// authUIDinfoCacheTTL is the expiration timeout for authUIDinfoCache
const authUIDinfoCacheTTL = 2 * time.Second

// authUIDinfoLookup performs authUIDinfo lookup by UID
func authUIDinfoLookup(uid int) (*authUIDinfo, error) {
	// Lookup authUIDinfoCache
	info := authUIDinfoCache[uid]
	if info != nil && info.expires.After(time.Now()) {
		return info, nil
	}

	// Resolve user names for matching
	// Also populates grpIDs with numeric group IDs
	usrNames := []string{strconv.Itoa(uid)}
	grpIDs := []string{}
	if usr, err := user.LookupId(usrNames[0]); err != nil {
		return nil, err
	} else {
		usrNames = append(usrNames, usr.Name)
		grpIDs = append(grpIDs, usr.Gid)

		grpids, err := usr.GroupIds()
		if err != nil {
			return nil, err
		}

		grpIDs = append(grpIDs, grpids...)
	}

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
	info = &authUIDinfo{
		usrNames: usrNames,
		grpNames: grpNames,
		expires:  time.Now().Add(authUIDinfoCacheTTL),
	}

	authUIDinfoCache[uid] = info

	// Return the answer
	return info, nil
}

// AuthUID returns operations allowed to client with given UID
func AuthUID(uid int) (AuthOps, error) {
	// Everything is allowed if authentication is not configured
	if Conf.ConfAuthUID == nil {
		return AuthOpsAll, nil
	}

	// Lookup UID info
	info, err := authUIDinfoLookup(uid)
	if err != nil {
		return 0, err
	}

	// Apply rules
	allowed := AuthOpsNone

	for _, rule := range Conf.ConfAuthUID {
		if rule.IsUser() {
			for _, usr := range info.usrNames {
				allowed |= rule.MatchUser(usr)
			}
		} else {
			for _, grp := range info.grpNames {
				allowed |= rule.MatchGroup(grp)
			}
		}
	}

	return allowed, nil
}
