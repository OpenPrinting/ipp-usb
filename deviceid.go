/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * IEEE-1284 Device ID
 */

package main

import "strings"

// DeviceID contains the parsed IEEE 1284 device id.
//
// Essentially, the Device ID is the case-insensitive string of ASCII
// characters defining peripheral characteristics and/or capabilities.
//
// Example:
//
//	"MFG:Hewlett-Packard;CMD:PJL,POSTSCRIPT;MDL:HP LaserJet 4000;CLS:PRINTER;"
//
// DeviceID consist of the series of the key:value1,value2,...,valueN
// tuples, separated by the semicolon characters.
//
// Standard keys:
//
//	Name          Abbreviations     Description
//
//	MANUFACTURER  MFG               Device manufacturer
//	MODEL         MDL               Model name
//	COMMAND SET   CMD               Supported document formats
//	SERIALNUMBER  SERN, SN          Device serial number
//
// See IEEE Std 1284-2000, 7.6 Device ID for details.
type DeviceID struct {
	RawData string           // DeviceID raw string
	Records []DeviceIDRecord // Parsed records
}

// DeviceIDRecord represents a single DeviceID key:value1,value2,...,valueN
// record.
type DeviceIDRecord struct {
	Key     string   // Key name
	RawData string   // Record value, not split
	Values  []string // Record values (split RawData)
}

// DeviceIDParse parses the IEEE 1284 device id string.
func DeviceIDParse(s string) *DeviceID {
	// Split device id into fields
	fields := strings.Split(s, ";")

	// Parse each field
	devid := &DeviceID{
		RawData: s,
		Records: make([]DeviceIDRecord, 0, 4),
	}

	for _, fld := range fields {
		// Strip white space before and after the record.
		// Ignore empty fields
		fld = strings.TrimSpace(fld)
		if fld == "" {
			continue
		}

		// Split field into key and value
		key, value := fld, ""
		if idx := strings.IndexByte(fld, ':'); idx >= 0 {
			key = strings.TrimSpace(fld[:idx])
			value = strings.TrimSpace(fld[idx+1:])
		}

		// Split values
		values := strings.Split(value, ",")
		for i := range values {
			values[i] = strings.TrimSpace(values[i])
		}

		rec := DeviceIDRecord{
			Key:     key,
			RawData: value,
			Values:  values,
		}

		devid.Records = append(devid.Records, rec)
	}

	return devid
}

// Get returns DeviceIDRecord by name, or nil if record is not found.
func (devid *DeviceID) Get(key string) *DeviceIDRecord {
	key = strings.ToUpper(key)

	for _, rec := range devid.Records {
		if strings.ToUpper(rec.Key) == key {
			return &rec
		}
	}
	return nil
}

// GetString returns DeviceID record as a whole string.
// If record is not found, "" is returned.
func (devid *DeviceID) GetString(key string) string {
	if rec := devid.Get(key); rec != nil {
		return rec.RawData
	}

	return ""
}

// Manufacturer returns the manufacturer name.
func (devid *DeviceID) Manufacturer() string {
	return devid.find("MANUFACTURER", "MFG")
}

// Model returns the model name.
func (devid *DeviceID) Model() string {
	return devid.find("MODEL", "MDL")
}

// CommandSet returns the device command set
// (list of supported document formats).
func (devid *DeviceID) CommandSet() []string {
	keys := []string{"COMMAND SET", "CMD"}
	for _, key := range keys {
		if rec := devid.Get(key); rec != nil {
			return rec.Values
		}
	}

	return nil
}

// SerialNumber returns the device serial number.
func (devid *DeviceID) SerialNumber() string {
	return devid.find("SERIALNUMBER", "SERN", "SN")
}

func (devid *DeviceID) find(keys ...string) string {
	for _, key := range keys {
		if rec := devid.Get(key); rec != nil {
			return rec.RawData
		}
	}

	return ""
}
