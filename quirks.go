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
	"io/ioutil"
	"net/http"
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
type QuirksSet map[string]*Quirks

// LoadQuirksSet creates new QuirksSet and loads its content from a directory
func LoadQuirksSet(path string) (QuirksSet, error) {
	qset := make(QuirksSet)
	return qset, qset.readDir(path)
}

// readDir loads all Quirks from a directory
func (qset QuirksSet) readDir(path string) error {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.Mode().IsRegular() && strings.HasSuffix(file.Name(), ".conf") {
			err = qset.readFile(file.Name())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// readFile reads all Quirks from a file
func (qset QuirksSet) readFile(file string) error {
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
			return err
		}

		// Get Quirks structure
		if rec.Type == IniRecordSection {
			q = qset[rec.Section]
			if q != nil {
				return fmt.Errorf("%s:%d: section %q already defined at %s",
					rec.File, rec.Line, rec.Section, q.Origin)
			}

			q = &Quirks{
				Origin:      fmt.Sprintf("%s:%d", rec.File, rec.Line),
				Model:       rec.Section,
				HttpHeaders: make(map[string]string),
				Index:       len(qset),
			}
			qset[rec.Section] = q
			continue
		} else if q == nil {
			return fmt.Errorf("%s:%d: %q = %q out of any section",
				rec.File, rec.Line, rec.Key, rec.Value)
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

		if err != nil {
			return err
		}
	}

	return nil
}

// Get quirks by model name
//
// In a case of multiple match, quirks are returned in
// the from most prioritized to least prioritized order
func (qset QuirksSet) Get(model string) []*Quirks {
	type item struct {
		q        *Quirks
		matchlen int
	}
	var list []item

	// Get list of matching quirks
	for _, q := range qset {
		matchlen := qset.matchModelName(model, q.Model, 0)
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
	quirks := make([]*Quirks, len(list))
	for i := range list {
		quirks[i] = list[i].q
	}

	// If at least one Quirks contains Blacklist == true,
	// it overrides everything else.
	//
	// Note, we check it after building and sorting the entire
	// list for more accurate logging
	for _, q := range quirks {
		if q.Blacklist {
			return []*Quirks{q}
		}
	}

	return quirks
}

// matchModelName matches model name against pattern. Pattern
// may contain wildcards and has a following syntax:
//   *   - matches any sequence of characters
//   ?   - matches exactly one character
//   \ C - matches character C
//   C   - matches character C (C is not *, ? or \)
//
// It return a counter of matched non-wildcard characters, -1 if no match
// Implemented as QuirksSet method, to avoid global namespace pollution
func (qset QuirksSet) matchModelName(model, pattern string, count int) int {
	for model != "" && pattern != "" {
		p := pattern[0]
		pattern = pattern[1:]

		switch p {
		case '*':
			for pattern != "" && pattern[0] == '*' {
				pattern = pattern[1:]
			}

			if pattern == "" {
				return count
			}

			for i := 0; i < len(model); i++ {
				c2 := qset.matchModelName(model[i:],
					pattern, count)
				if c2 >= 0 {
					return c2
				}
			}

		case '?':
			model = model[1:]

		case '\\':
			if pattern == "" {
				return -1
			}
			p, pattern = pattern[0], pattern[1:]
			fallthrough

		default:
			if model[0] != p {
				return -1
			}
			model = model[1:]
			count++

		}
	}

	for pattern != "" && pattern[0] == '*' {
		pattern = pattern[1:]
	}

	if model == "" && pattern == "" {
		return count
	}

	return -1
}
