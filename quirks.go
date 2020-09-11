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
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Quirks represents device-specific quirks
type Quirks struct {
	Origin      string            // file:line of definition
	Model       string            // Device model name
	Blacklist   bool              // Blacklist the device
	HttpHeaders map[string]string // HTTP header override
	Index       int               // Incremented in order of loading
}

// QuirksSet represents collection of quirks, indexed by model name
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
				HttpHeaders: make(map[string]string),
				Index:       len(*qset),
			}
			*qset = append(*qset, q)

			continue
		} else if q == nil {
			err = fmt.Errorf("%s:%d: %q = %q out of any section",
				rec.File, rec.Line, rec.Key, rec.Value)
			break
		}

		// Update Quirks data
		if strings.HasPrefix(rec.Key, "http-") {
			key := http.CanonicalHeaderKey(rec.Key[5:])
			q.HttpHeaders[key] = rec.Value
			continue
		}

		switch rec.Key {
		case "blacklist":
			err = confLoadBinaryKey(&q.Blacklist, rec,
				"false", "true")
		}
	}

	if err == io.EOF {
		err = nil
	}

	return nil
}

// Get quirks by model name
//
// In a case of multiple match, quirks are returned in
// the from most prioritized to least prioritized order
//
// Duplicates are removed: if some parameter is set by
// more prioritized entry, it is removed from the less
// prioritized entries. Entries, that in result become
// empty, are removed at all
func (qset QuirksSet) Get(model string) []Quirks {
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
	quirks := make([]Quirks, len(list))
	for i := range list {
		quirks[i] = *list[i].q
	}

	// If at least one Quirks contains Blacklist == true,
	// it overrides everything else.
	//
	// Note, we check it after building and sorting the entire
	// list for more accurate logging
	for _, q := range quirks {
		if q.Blacklist {
			return []Quirks{q}
		}
	}

	// Remove duplicates and empty entries
	httpHeaderSeen := make(map[string]struct{})
	out := 0
	for in, q := range quirks {
		q.HttpHeaders = make(map[string]string)

		for name, value := range quirks[in].HttpHeaders {
			if _, seen := httpHeaderSeen[name]; !seen {
				httpHeaderSeen[name] = struct{}{}
				q.HttpHeaders[name] = value
			}
		}

		if len(q.HttpHeaders) != 0 {
			quirks[out] = q
			out++
		}
	}

	quirks = quirks[:out]

	return quirks
}
