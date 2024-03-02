/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Device-specific quirks
 */

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Quirks represents device-specific quirks
type Quirks struct {
	Origin           string            // file:line of definition
	Model            string            // Device model name
	Blacklist        bool              // Blacklist the device
	HTTPHeaders      map[string]string // HTTP header override
	UsbMaxInterfaces uint              // Max number of USB interfaces
	DisableFax       bool              // Disable fax for device
	ResetMethod      QuirksResetMethod // Device reset method
	InitDelay        time.Duration     // Delay before 1st IPP-USB request
	RequestDelay     time.Duration     // Delay between IPP-USB requests
	IgnoreIppStatus  bool              // Ignore IPP status
	BuggyIppRsp      QuirksBuggyIppRsp // Handling of buggy IPP responses
	Index            int               // Incremented in order of loading
}

// QuirksResetMethod represents how to reset a device
// during initialization
type QuirksResetMethod int

// QuirksResetUnset - reset method not specified
// QuirksResetNone  - don't reset device at all
// QuirksResetSoft  - use class-specific soft reset
// QuirksResetHard  - use USB hard reset
const (
	QuirksResetUnset QuirksResetMethod = iota
	QuirksResetNone
	QuirksResetSoft
	QuirksResetHard
)

// String returns textual representation of QuirksResetMethod
func (m QuirksResetMethod) String() string {
	switch m {
	case QuirksResetUnset:
		return "unset"
	case QuirksResetNone:
		return "none"
	case QuirksResetSoft:
		return "soft"
	case QuirksResetHard:
		return "hard"
	}

	return fmt.Sprintf("unknown (%d)", int(m))
}

// QuirksBuggyIppRsp defines, how to handle buggy IPP responses
type QuirksBuggyIppRsp int

// QuirksBuggyIppRspUnset    - handling of bad IPP responses is not specified
// QuirksBuggyIppRspAllow    - ipp-usb will allow bad IPP responses
// QuirksBuggyIppRspReject   - ipp-usb will reject bad IPP responses
// QuirksBuggyIppRspSanitize - bad ipp responses will be sanitized (fixed)
const (
	QuirksBuggyIppRspUnset QuirksBuggyIppRsp = iota
	QuirksBuggyIppRspAllow
	QuirksBuggyIppRspReject
	QuirksBuggyIppRspSanitize
)

// String returns textual representation of QuirksBuggyIppRsp
func (m QuirksBuggyIppRsp) String() string {
	switch m {
	case QuirksBuggyIppRspUnset:
		return "unset"
	case QuirksBuggyIppRspAllow:
		return "allow"
	case QuirksBuggyIppRspReject:
		return "reject"
	case QuirksBuggyIppRspSanitize:
		return "sanitize"
	}

	return fmt.Sprintf("unknown (%d)", int(m))
}

// empty returns true, if Quirks are actually empty
func (q *Quirks) empty() bool {
	return !q.Blacklist &&
		len(q.HTTPHeaders) == 0 &&
		q.UsbMaxInterfaces == 0 &&
		!q.DisableFax &&
		q.ResetMethod == QuirksResetUnset &&
		q.InitDelay == 0 &&
		q.RequestDelay == 0 &&
		!q.IgnoreIppStatus &&
		q.BuggyIppRsp == QuirksBuggyIppRspUnset
}

// QuirksSet represents collection of quirks
type QuirksSet []*Quirks

// LoadQuirksSet creates new QuirksSet and loads its content from a directory
func LoadQuirksSet(paths ...string) (QuirksSet, error) {
	qset := QuirksSet{}

	for _, path := range paths {
		err := qset.readDir(path)
		if err != nil {
			return nil, err
		}
	}

	return qset, nil
}

// readDir loads all Quirks from a directory
func (qset *QuirksSet) readDir(path string) error {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return err
	}

	for _, file := range files {
		if file.Mode().IsRegular() &&
			strings.HasSuffix(file.Name(), ".conf") {
			err = qset.readFile(filepath.Join(path, file.Name()))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// readFile reads all Quirks from a file
func (qset *QuirksSet) readFile(file string) error {
	// Open quirks file
	ini, err := OpenIniFileWithRecType(file)
	if err != nil {
		return err
	}

	defer ini.Close()

	// Load all quirks
	var q *Quirks
	for err == nil {
		var rec *IniRecord
		rec, err = ini.Next()
		if err != nil {
			break
		}

		// Get Quirks structure
		if rec.Type == IniRecordSection {
			q = &Quirks{
				Origin:      fmt.Sprintf("%s:%d", rec.File, rec.Line),
				Model:       rec.Section,
				HTTPHeaders: make(map[string]string),
				Index:       len(*qset),
			}
			qset.Add(q)

			continue
		} else if q == nil {
			err = fmt.Errorf("%s:%d: %q = %q out of any section",
				rec.File, rec.Line, rec.Key, rec.Value)
			break
		}

		// Update Quirks data
		if strings.HasPrefix(rec.Key, "http-") {
			key := http.CanonicalHeaderKey(rec.Key[5:])
			q.HTTPHeaders[key] = rec.Value
			continue
		}

		switch rec.Key {
		case "blacklist":
			err = rec.LoadBool(&q.Blacklist)

		case "buggy-ipp-responses":
			err = rec.LoadQuirksBuggyIppRsp(&q.BuggyIppRsp)

		case "disable-fax":
			err = rec.LoadBool(&q.DisableFax)

		case "ignore-ipp-status":
			err = rec.LoadBool(&q.IgnoreIppStatus)

		case "init-delay":
			err = rec.LoadDuration(&q.InitDelay)

		case "init-reset":
			err = rec.LoadQuirksResetMethod(&q.ResetMethod)

		case "request-delay":
			err = rec.LoadDuration(&q.RequestDelay)

		case "usb-max-interfaces":
			err = rec.LoadUintRange(&q.UsbMaxInterfaces,
				1, math.MaxUint32)
		}
	}

	if err == io.EOF {
		err = nil
	}

	return err
}

// Add appends Quirks to QuirksSet
func (qset *QuirksSet) Add(q *Quirks) {
	*qset = append(*qset, q)
}

// ByModelName returns a subset of quirks, applicable for
// specific device, matched by model name
//
// In a case of multiple match, quirks are returned in
// the from most prioritized to least prioritized order
//
// Duplicates are removed: if some parameter is set by
// more prioritized entry, it is removed from the less
// prioritized entries. Entries, that in result become
// empty, are removed at all
func (qset QuirksSet) ByModelName(model string) QuirksSet {
	type item struct {
		q        *Quirks
		matchlen int
	}
	var list []item

	// Get list of matching quirks
	for _, q := range qset {
		matchlen := GlobMatch(model, q.Model)
		if matchlen >= 0 {
			list = append(list, item{q, matchlen})
		}
	}

	// Sort the list by matchlen, in decreasing order
	sort.Slice(list, func(i, j int) bool {
		if list[i].matchlen != list[j].matchlen {
			return list[i].matchlen > list[j].matchlen
		}
		return list[i].q.Index < list[j].q.Index
	})

	// Rebuild it into the slice of *Quirks
	quirks := make(QuirksSet, len(list))
	for i := range list {
		quirks[i] = list[i].q
	}

	// Remove duplicates and empty entries
	httpHeaderSeen := make(map[string]struct{})
	out := 0
	for in, q := range quirks {
		// Note, here we avoid modification of the HTTPHeaders
		// map in the original Quirks structure
		//
		// Unfortunately, Golang misses immutable types,
		// so we must be very careful here
		q2 := &Quirks{}
		*q2 = *q
		q2.HTTPHeaders = make(map[string]string)

		for name, value := range quirks[in].HTTPHeaders {
			if _, seen := httpHeaderSeen[name]; !seen {
				httpHeaderSeen[name] = struct{}{}
				q2.HTTPHeaders[name] = value
			}
		}

		if !q2.empty() {
			quirks[out] = q2
			out++
		}
	}

	quirks = quirks[:out]

	return quirks
}

// GetBlacklist returns effective Blacklist parameter,
// taking the whole set into consideration
func (qset QuirksSet) GetBlacklist() bool {
	for _, q := range qset {
		if q.Blacklist {
			return true
		}
	}

	return false
}

// GetUsbMaxInterfaces returns effective UsbMaxInterfaces parameter,
// taking the whole set into consideration
func (qset QuirksSet) GetUsbMaxInterfaces() uint {
	for _, q := range qset {
		if q.UsbMaxInterfaces != 0 {
			return q.UsbMaxInterfaces
		}
	}

	return 0
}

// GetDisableFax returns effective DisableFax parameter,
// taking the whole set into consideration
func (qset QuirksSet) GetDisableFax() bool {
	for _, q := range qset {
		if q.DisableFax {
			return true
		}
	}

	return false
}

// GetResetMethod returns effective ResetMethod parameter
func (qset QuirksSet) GetResetMethod() QuirksResetMethod {
	for _, q := range qset {
		if q.ResetMethod != QuirksResetUnset {
			return q.ResetMethod
		}
	}

	return QuirksResetNone
}

// GetBuggyIppRsp returns effective BuggyIppRsp parameter
func (qset QuirksSet) GetBuggyIppRsp() QuirksBuggyIppRsp {
	for _, q := range qset {
		if q.BuggyIppRsp != QuirksBuggyIppRspUnset {
			return q.BuggyIppRsp
		}
	}

	return QuirksBuggyIppRspUnset
}

// GetInitDelay returns effective InitDelay parameter
func (qset QuirksSet) GetInitDelay() time.Duration {
	for _, q := range qset {
		if q.InitDelay != 0 {
			return q.InitDelay
		}
	}

	return 0
}

// GetRequestDelay returns effective RequestDelay parameter
func (qset QuirksSet) GetRequestDelay() time.Duration {
	for _, q := range qset {
		if q.RequestDelay != 0 {
			return q.RequestDelay
		}
	}

	return 0
}

// GetIgnoreIppStatus returns effective IgnoreIppStatus parameter,
// taking the whole set into consideration
func (qset QuirksSet) GetIgnoreIppStatus() bool {
	for _, q := range qset {
		if q.IgnoreIppStatus {
			return true
		}
	}

	return false
}
