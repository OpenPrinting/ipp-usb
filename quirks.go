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
	"strconv"
	"strings"
	"time"
)

// Quirk represents a single quirk
type Quirk struct {
	Origin    string       // file:line of definition
	Match     string       // Match pattern
	MatchHWID *HWIDPattern // HWID match pattern or nil
	Name      string       // Quirk name
	RawValue  string       // Quirk raw (not parsed) value
	Parsed    interface{}  // Parsed Value
	LoadOrder int          // Incremented in order of loading
}

// Quirk names. Use these constants instead of literal strings,
// so compiler will catch a mistake:
const (
	QuirkNmBlacklist             = "blacklist"
	QuirkNmBuggyIppResponses     = "buggy-ipp-responses"
	QuirkNmDisableFax            = "disable-fax"
	QuirkNmIgnoreIppStatus       = "ignore-ipp-status"
	QuirkNmInitDelay             = "init-delay"
	QuirkNmInitReset             = "init-reset"
	QuirkNmInitRetryPartial      = "init-retry-partial"
	QuirkNmInitTimeout           = "init-timeout"
	QuirkNmMfg                   = "mfg"
	QuirkNmModel                 = "model"
	QuirkNmRequestDelay          = "request-delay"
	QuirkNmUsbMaxInterfaces      = "usb-max-interfaces"
	QuirkNmUsbSendDelayThreshold = "usb-send-delay-threshold"
	QuirkNmUsbSendDelay          = "usb-send-delay"
	QuirkNmZlpRecvHack           = "zlp-recv-hack"
	QuirkNmZlpSend               = "zlp-send"
)

// quirkParse maps quirk names into appropriate parsing methods,
// which defines value syntax and resulting type.
var quirkParse = map[string]func(*Quirk) error{
	QuirkNmBlacklist:             (*Quirk).parseBool,
	QuirkNmBuggyIppResponses:     (*Quirk).parseQuirkBuggyIppRsp,
	QuirkNmDisableFax:            (*Quirk).parseBool,
	QuirkNmIgnoreIppStatus:       (*Quirk).parseBool,
	QuirkNmInitDelay:             (*Quirk).parseDuration,
	QuirkNmInitReset:             (*Quirk).parseQuirkResetMethod,
	QuirkNmInitRetryPartial:      (*Quirk).parseBool,
	QuirkNmInitTimeout:           (*Quirk).parseDuration,
	QuirkNmMfg:                   (*Quirk).parseString,
	QuirkNmModel:                 (*Quirk).parseString,
	QuirkNmRequestDelay:          (*Quirk).parseDuration,
	QuirkNmUsbMaxInterfaces:      (*Quirk).parseUint,
	QuirkNmUsbSendDelay:          (*Quirk).parseDuration,
	QuirkNmUsbSendDelayThreshold: (*Quirk).parseUint,
	QuirkNmZlpRecvHack:           (*Quirk).parseBool,
	QuirkNmZlpSend:               (*Quirk).parseBool,
}

// quirkDefaultStrings contains default values for quirks, in
// a string form.
var quirkDefaultStrings = map[string]string{
	QuirkNmBlacklist:             "false",
	QuirkNmBuggyIppResponses:     "reject",
	QuirkNmDisableFax:            "false",
	QuirkNmIgnoreIppStatus:       "false",
	QuirkNmInitDelay:             "0",
	QuirkNmInitReset:             "none",
	QuirkNmInitRetryPartial:      "false",
	QuirkNmInitTimeout:           DevInitTimeout.String(),
	QuirkNmMfg:                   "",
	QuirkNmModel:                 "",
	QuirkNmRequestDelay:          "0",
	QuirkNmUsbMaxInterfaces:      "0",
	QuirkNmUsbSendDelay:          "0",
	QuirkNmUsbSendDelayThreshold: "0",
	QuirkNmZlpRecvHack:           "false",
	QuirkNmZlpSend:               "false",
}

// quirkDefault contains default values for quirks, precompiled.
var quirkDefault = make(map[string]*Quirk)

// init populates quirkDefault using quirk values from quirkDefaultStrings.
func init() {
	for name, value := range quirkDefaultStrings {
		q := &Quirk{
			Origin:    "default",
			Match:     "*",
			Name:      name,
			RawValue:  value,
			LoadOrder: math.MaxInt32,
		}

		parse := quirkParse[name]
		err := parse(q)
		if err != nil {
			panic(err)
		}

		quirkDefault[name] = q
	}
}

// isHTTP reports if Quirk is the HTTP header quirk
func (q *Quirk) isHTTP() bool {
	return strings.HasPrefix(q.Name, "http-")
}

// isHTTP reports if Quirk is matched by HWID
func (q *Quirk) isHWID() bool {
	return q.MatchHWID != nil
}

// parseString parses and saves [Quirk.RawValue] as string.
func (q *Quirk) parseString() error {
	q.Parsed = q.RawValue
	return nil
}

// parseBool parses and saves [Quirk.RawValue] as bool.
func (q *Quirk) parseBool() error {
	switch q.RawValue {
	case "true":
		q.Parsed = true
	case "false":
		q.Parsed = false
	default:
		return fmt.Errorf("%q: must be true or false", q.RawValue)
	}

	return nil
}

// parseUint parses [Quirk.RawValue] as unsigned int.
func (q *Quirk) parseUint() error {
	v, err := strconv.ParseUint(q.RawValue, 10, 32)
	if err != nil {
		return fmt.Errorf("%q: invalid unsigned integer", q.RawValue)
	}

	q.Parsed = uint(v)
	return nil
}

// parseDuration parses [Quirk.RawValue] as time.Duration.
func (q *Quirk) parseDuration() error {
	// Try to parse as uint. If OK, interpret it
	// as a millisecond time.
	ms, err := strconv.ParseUint(q.RawValue, 10, 32)
	if err == nil {
		q.Parsed = time.Millisecond * time.Duration(ms)
		return nil
	}

	// Try to use time.ParseDuration.
	//
	if strings.HasPrefix(q.RawValue, "+") ||
		strings.HasPrefix(q.RawValue, "-") {
		// Note, time.ParseDuration allows signed duration,
		// but we don't.
		return fmt.Errorf("%q: invalid duration", q.RawValue)
	}

	v, err := time.ParseDuration(q.RawValue)
	if err == nil && v >= 0 {
		q.Parsed = v
		return nil
	}

	return fmt.Errorf("%q: invalid duration", q.RawValue)
}

// parseQuirkBuggyIppRsp parses [Quirk.RawValue] as QuirkBuggyIppRsp.
func (q *Quirk) parseQuirkBuggyIppRsp() error {
	switch q.RawValue {
	case "allow":
		q.Parsed = QuirkBuggyIppRspAllow
	case "reject":
		q.Parsed = QuirkBuggyIppRspReject
	case "sanitize":
		q.Parsed = QuirkBuggyIppRspSanitize
	default:
		s := q.RawValue
		return fmt.Errorf("%q: must be allow, reject or sanitize", s)
	}

	return nil
}

// parseQuirkResetMethod parses [Quirk.RawValue] as QuirkResetMethod.
func (q *Quirk) parseQuirkResetMethod() error {
	switch q.RawValue {
	case "none":
		q.Parsed = QuirkResetNone
	case "soft":
		if q.isHWID() {
			return fmt.Errorf("%s = %s not available in HWID mode",
				q.Name, q.RawValue)
		}

		q.Parsed = QuirkResetSoft
	case "hard":
		q.Parsed = QuirkResetHard
	default:
		return fmt.Errorf("%q: must be none, soft or hard", q.RawValue)
	}

	return nil
}

// QuirkResetMethod represents how to reset a device
// during initialization
type QuirkResetMethod int

// QuirkResetUnset - reset method not specified
// QuirkResetNone  - don't reset device at all
// QuirkResetSoft  - use class-specific soft reset
// QuirkResetHard  - use USB hard reset
const (
	QuirkResetNone QuirkResetMethod = iota
	QuirkResetSoft
	QuirkResetHard
)

// String returns textual representation of QuirkResetMethod
func (m QuirkResetMethod) String() string {
	switch m {
	case QuirkResetNone:
		return "none"
	case QuirkResetSoft:
		return "soft"
	case QuirkResetHard:
		return "hard"
	}

	return fmt.Sprintf("unknown (%d)", int(m))
}

// QuirkBuggyIppRsp defines, how to handle buggy IPP responses
type QuirkBuggyIppRsp int

// QuirkBuggyIppRspReject   - ipp-usb will reject bad IPP responses
// QuirkBuggyIppRspAllow    - ipp-usb will allow bad IPP responses
// QuirkBuggyIppRspSanitize - bad ipp responses will be sanitized (fixed)
const (
	QuirkBuggyIppRspReject QuirkBuggyIppRsp = iota
	QuirkBuggyIppRspAllow
	QuirkBuggyIppRspSanitize
)

// String returns textual representation of QuirkBuggyIppRsp
func (m QuirkBuggyIppRsp) String() string {
	switch m {
	case QuirkBuggyIppRspReject:
		return "reject"
	case QuirkBuggyIppRspAllow:
		return "allow"
	case QuirkBuggyIppRspSanitize:
		return "sanitize"
	}

	return fmt.Sprintf("unknown (%d)", int(m))
}

// Quirks is the collection of Quirk, indexed by Quirk.Name.
// All quirks in the collection have a unique name.
//
// It is used for two purposes:
//   - to represent a section in the quirks file
//   - to represent set of quirks, applied to the particular device.
type Quirks struct {
	byName      map[string]*Quirk // Quirks by name
	weights     map[string]int    // Matching weights
	HTTPHeaders map[string]string // HTTP header override
}

// newQuirks returns a new Quirks structure
func newQuirks() *Quirks {
	return &Quirks{
		byName:      make(map[string]*Quirk),
		weights:     make(map[string]int),
		HTTPHeaders: make(map[string]string),
	}
}

// put adds Quirk to Quirks, or replaces existing one, if any.
func (quirks *Quirks) put(q *Quirk) {
	quirks.byName[q.Name] = q

	if q.isHTTP() {
		// Canonicalize and save HTTP header name
		hdr := http.CanonicalHeaderKey(q.Name[5:])
		quirks.HTTPHeaders[hdr] = q.RawValue
	}
}

// prioritizeAndSave puts Quirk to Quirks, if it is either not in the set yet
// or has higher priority that existing one
func (quirks *Quirks) prioritizeAndSave(q *Quirk, weight int) {
	prev := quirks.byName[q.Name]
	prevWeight := quirks.weights[q.Name]

	save := false

	switch {
	// Always save, if the Quirk is not yet in the set
	case prev == nil:
		save = true
	// Choose by matching weight (more specific match wins)
	case weight > prevWeight:
		save = true
	case weight < prevWeight:

	// Choose by load order (first loaded wins)
	case q.LoadOrder > prev.LoadOrder:
		save = true
	}

	if save {
		quirks.put(q)
		quirks.weights[q.Name] = weight
	}
}

// WriteLog writes Quirks to log.
func (quirks *Quirks) WriteLog(title string, log *Logger) {
	if quirks.IsEmpty() {
		log.Debug(' ', "%s: EMPTY", title)
		return
	}

	log.Debug(' ', "%s:", title)

	prevMatch := ""
	for _, q := range quirks.All() {
		val := q.RawValue
		if _, isStr := q.Parsed.(string); isStr {
			val = strconv.Quote(val)
		}

		if q.Match != prevMatch {
			prevMatch = q.Match
			log.Debug(' ', "  [%s]", q.Match)
		}

		log.Debug(' ', "    ; (%s)", q.Origin)
		log.Debug(' ', "    %s = %s", q.Name, val)
	}
}

// IsEmpty reports if Quirks are empty
func (quirks *Quirks) IsEmpty() bool {
	return len(quirks.byName) == 0
}

// Get returns quirk by name.
func (quirks *Quirks) Get(name string) *Quirk {
	q := quirks.byName[name]
	if q == nil {
		q = quirkDefault[name]
	}

	return q
}

// All returns all quirks in the collection. This method is
// intended mostly for diagnostic purposes (logging, dumping,
// testing and so on).
func (quirks *Quirks) All() []*Quirk {
	qq := make([]*Quirk, 0, len(quirks.byName))
	for _, q := range quirks.byName {
		qq = append(qq, q)
	}

	sort.Slice(qq, func(i, j int) bool {
		return qq[i].Name < qq[j].Name
	})

	return qq
}

// GetBlacklist returns effective "blacklist" parameter,
// taking the whole set into consideration.
func (quirks *Quirks) GetBlacklist() bool {
	return quirks.Get(QuirkNmBlacklist).Parsed.(bool)
}

// GetBuggyIppRsp returns effective "buggy-ipp-responses" parameter
// taking the whole set into consideration.
func (quirks *Quirks) GetBuggyIppRsp() QuirkBuggyIppRsp {
	return quirks.Get(QuirkNmBuggyIppResponses).Parsed.(QuirkBuggyIppRsp)
}

// GetDisableFax returns effective "disable-fax" parameter,
// taking the whole set into consideration.
func (quirks *Quirks) GetDisableFax() bool {
	return quirks.Get(QuirkNmDisableFax).Parsed.(bool)
}

// GetIgnoreIppStatus returns effective "ignore-ipp-status" parameter,
// taking the whole set into consideration.
func (quirks *Quirks) GetIgnoreIppStatus() bool {
	return quirks.Get(QuirkNmIgnoreIppStatus).Parsed.(bool)
}

// GetInitDelay returns effective "init-delay" parameter
// taking the whole set into consideration.
func (quirks *Quirks) GetInitDelay() time.Duration {
	return quirks.Get(QuirkNmInitDelay).Parsed.(time.Duration)
}

// GetInitRetryPartial returns effective "init-retry-partial" parameter
// taking the whole set into consideration.
func (quirks *Quirks) GetInitRetryPartial() bool {
	return quirks.Get(QuirkNmInitRetryPartial).Parsed.(bool)
}

// GetInitReset returns effective "init-reset" parameter
// taking the whole set into consideration.
func (quirks *Quirks) GetInitReset() QuirkResetMethod {
	return quirks.Get(QuirkNmInitReset).Parsed.(QuirkResetMethod)
}

// GetInitTimeout returns effective "init-timeout" parameter
// taking the whole set into consideration.
func (quirks *Quirks) GetInitTimeout() time.Duration {
	return quirks.Get(QuirkNmInitTimeout).Parsed.(time.Duration)
}

// GetMfg returns effective "mfg" parameter
// taking the whole set into consideration.
func (quirks *Quirks) GetMfg() string {
	return quirks.Get(QuirkNmMfg).Parsed.(string)
}

// GetModel returns effective "model" parameter
// taking the whole set into consideration.
func (quirks *Quirks) GetModel() string {
	return quirks.Get(QuirkNmModel).Parsed.(string)
}

// GetRequestDelay returns effective "request-delay" parameter
// taking the whole set into consideration.
func (quirks *Quirks) GetRequestDelay() time.Duration {
	return quirks.Get(QuirkNmRequestDelay).Parsed.(time.Duration)
}

// GetUsbMaxInterfaces returns effective "usb-max-interfaces" parameter,
// taking the whole set into consideration.
func (quirks *Quirks) GetUsbMaxInterfaces() uint {
	return quirks.Get(QuirkNmUsbMaxInterfaces).Parsed.(uint)
}

// GetUsbSendDelayThreshold returns effective "usb-send-delay-threshold"
// parameter taking the whole set into consideration.
func (quirks *Quirks) GetUsbSendDelayThreshold() uint {
	return quirks.Get(QuirkNmUsbSendDelay).Parsed.(uint)
}

// GetUsbSendDelay returns effective "usb-send-delay" parameter
// taking the whole set into consideration.
func (quirks *Quirks) GetUsbSendDelay() time.Duration {
	return quirks.Get(QuirkNmUsbSendDelay).Parsed.(time.Duration)
}

// GetZlpRecvHack returns effective "zlp-send" parameter,
// taking the whole set into consideration.
func (quirks *Quirks) GetZlpRecvHack() bool {
	return quirks.Get(QuirkNmZlpRecvHack).Parsed.(bool)
}

// GetZlpSend returns effective "zlp-send" parameter,
// taking the whole set into consideration.
func (quirks *Quirks) GetZlpSend() bool {
	return quirks.Get(QuirkNmZlpSend).Parsed.(bool)
}

// QuirksDb represents in-memory data base of Quirks, as loaded
// from the disk files.
type QuirksDb []*Quirks

// LoadQuirksSet creates new QuirksDb and loads its content from a directory
func LoadQuirksSet(paths ...string) (QuirksDb, error) {
	qdb := QuirksDb{}

	for _, path := range paths {
		err := qdb.readDir(path)
		if err != nil {
			return nil, err
		}
	}

	return qdb, nil
}

// readDir loads all Quirks from a directory
func (qdb *QuirksDb) readDir(path string) error {
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
			err = qdb.readFile(filepath.Join(path, file.Name()))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// readFile reads all Quirks from a file
func (qdb *QuirksDb) readFile(file string) error {
	// Open quirks file
	ini, err := OpenIniFileWithRecType(file)
	if err != nil {
		return err
	}

	defer ini.Close()

	// Load all quirks
	var quirks *Quirks
	var matchHWID *HWIDPattern
	var loadOrder int

	for err == nil {
		var rec *IniRecord
		rec, err = ini.Next()
		if err != nil {
			break
		}

		origin := fmt.Sprintf("%s:%d", rec.File, rec.Line)

		// Get Quirks structure
		if rec.Type == IniRecordSection {
			matchHWID = ParseHWIDPattern(rec.Section)
			quirks = newQuirks()
			qdb.Add(quirks)

			continue
		} else if quirks == nil {
			err = fmt.Errorf("%s: %q = %q out of any section",
				origin, rec.Key, rec.Value)
			break
		}

		if found := quirks.byName[rec.Key]; found != nil {
			err = fmt.Errorf("%s: %q already defined at %s",
				origin, rec.Key, found.Origin)
			return err
		}

		q := &Quirk{
			Origin:    origin,
			Match:     rec.Section,
			MatchHWID: matchHWID,
			Name:      rec.Key,
			RawValue:  rec.Value,
			LoadOrder: loadOrder,
		}

		loadOrder++

		if q.isHTTP() {
			q.Name = strings.ToLower(q.Name)
			q.Parsed = q.RawValue
		} else {
			parse := quirkParse[q.Name]
			if parse == nil {
				// Ignore unknown keys, it may be due to
				// downgrade of the ipp-usb
				continue
			}

			err := parse(q)
			if err != nil {
				err = fmt.Errorf("%s: %s", origin, err)
				return err
			}
		}

		quirks.put(q)
	}

	if err == io.EOF {
		err = nil
	}

	return err
}

// Add appends Quirks to QuirksDb
func (qdb *QuirksDb) Add(q *Quirks) {
	*qdb = append(*qdb, q)
}

// MatchByHWID returns collection of quirks, applicable for the
// specific device, matched by HWID
func (qdb QuirksDb) MatchByHWID(vid, pid uint16) *Quirks {
	ret := newQuirks()

	for _, quirks := range qdb {
		for _, q := range quirks.byName {
			if q.isHWID() {
				weight := q.MatchHWID.Match(vid, pid)
				if weight >= 0 {
					ret.prioritizeAndSave(q, weight)
				}
			}
		}
	}

	return ret
}

// MatchByModelName returns collection of quirks, applicable for
// the specific device, matched by model name.
func (qdb QuirksDb) MatchByModelName(model string) *Quirks {
	ret := newQuirks()

	for _, quirks := range qdb {
		for _, q := range quirks.byName {
			if !q.isHWID() {
				// Note, by multiplying GlobMatch by 2,
				// we have the following:
				//   - Exact HWID match is the must
				//     weightful. Its weight is math.MaxUint32
				//   - The default (all-wildcard) match is
				//     the least weightful. Its weight is 0.
				//   - Any non-default model-name match is
				//     more weightful, that the wildcard
				//     HWID match, which weight is 1
				//   - Weight of any non-default model-name
				//     match is proportional to the length of
				//     the non-wildcard matched part and
				//     it is between the wildcard and exact
				//     HWID match.
				weight := 2 * GlobMatch(model, q.Match)
				if weight >= 0 {
					ret.prioritizeAndSave(q, weight)
				}
			}
		}
	}

	return ret
}
