/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * IPP service registration
 */

package main

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/alexpevzner/goipp"
)

// IppService performs IPP Get-Printer-Attributes query using provided
// http.Client and decodes received information into the form suitable
// for DNS-SD registration
func IppService(c *http.Client) (dnssd_name string, info DnsSdInfo, err error) {
	uri := "http://localhost/ipp/print"

	// Query printer attributes
	msg := goipp.NewRequest(goipp.DefaultVersion, goipp.OpGetPrinterAttributes, 1)
	msg.Operation.Add(goipp.MakeAttribute("attributes-charset",
		goipp.TagCharset, goipp.String("utf-8")))
	msg.Operation.Add(goipp.MakeAttribute("attributes-natural-language",
		goipp.TagLanguage, goipp.String("en-US")))
	msg.Operation.Add(goipp.MakeAttribute("printer-uri",
		goipp.TagURI, goipp.String(uri)))
	msg.Operation.Add(goipp.MakeAttribute("requested-attributes",
		goipp.TagKeyword, goipp.String("all")))

	req, _ := msg.EncodeBytes()
	resp, err := c.Post(uri, goipp.ContentType, bytes.NewBuffer(req))
	if err != nil {
		return
	}

	// Decode IPP response message
	respData, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return
	}

	err = msg.DecodeBytes(respData)
	if err != nil {
		log_debug("! IPP: %s", err)
		log_dump(respData)
		err = nil // FIXME - ignore error for now
		return
	}

	// Decode service info
	attrs := newIppDecoder(msg)
	dnssd_name, info = attrs.Decode()

	return
}

// ippAttrs represents a collection of IPP printer attributes,
// enrolled into a map for convenient access
type ippAttrs map[string]goipp.Values

// Create new ippAttrs
func newIppDecoder(msg *goipp.Message) ippAttrs {
	attrs := make(ippAttrs)

	// Note, we move from the end of list to the beginning, so
	// in a case of duplicated attributes, first occurrence wins
	for i := len(msg.Printer) - 1; i >= 0; i-- {
		attr := msg.Printer[i]
		attrs[attr.Name] = attr.Values
	}

	return attrs
}

// Decode printer attributes and build TXT record for IPP service
//
// This is where information comes from:
//
//   DNS-SD name: "printer-dns-sd-name" with fallback to
//                "printer-info" and "printer-make-and-model"
//
//   TXT fields:
//     air:              hardcoded as "none"
//     mopria-certified: "mopria-certified"
//     rp:               hardcoded as "ipp/print"
//     kind:             "printer-kind"
//     URF:              "urf-supported" with fallback to "printer-device-id"
//     UUID:             "printer-uuid"
//     Color:            "color-supported"
//     Duplex:           search "sides-supported" for strings with
//                       prefix "one" or "two"
//     note:             "printer-location"
//     ty:               "printer-make-and-model"
//     product:          "printer-make-and-model", in round brackets
//     pdl:              "document-format-supported"
//     txtvers:          hardcoded as "1"
//
func (attrs ippAttrs) Decode() (dnssd_name string, info DnsSdInfo) {
	info = DnsSdInfo{Type: "_ipp._tcp"}

	// Obtain dnssd_name
	dnssd_name = attrs.strSingle("printer-dns-sd-name",
		"printer-info", "printer-make-and-model")

	// Obtain and parse IEEE 1284 device ID
	devid := make(map[string]string)
	for _, id := range strings.Split(attrs.strSingle("printer-device-id"), ";") {
		keyval := strings.SplitN(id, ":", 2)
		if len(keyval) == 2 {
			devid[keyval[0]] = keyval[1]
		}
	}

	info.Txt.Add("air", "none")
	info.Txt.AddNotEmpty("mopria-certified", attrs.strSingle("mopria-certified"))
	info.Txt.Add("rp", "ipp/print")
	info.Txt.AddNotEmpty("kind", attrs.strJoined("printer-kind"))
	if !info.Txt.AddNotEmpty("URF", attrs.strJoined("urf-supported")) {
		info.Txt.AddNotEmpty("URF", devid["URF"])
	}
	info.Txt.AddNotEmpty("UUID", strings.TrimPrefix(attrs.strSingle("printer-uuid"), "urn:uuid:"))
	info.Txt.AddNotEmpty("Color", attrs.getBool("color-supported"))
	info.Txt.AddNotEmpty("Duplex", attrs.getDuplex())
	info.Txt.Add("note", attrs.strSingle("printer-location"))
	info.Txt.AddNotEmpty("ty", attrs.strSingle("printer-make-and-model"))
	info.Txt.AddNotEmpty("product", attrs.strBrackets("printer-make-and-model"))
	info.Txt.AddNotEmpty("pdl", attrs.strJoined("document-format-supported"))
	info.Txt.Add("txtvers", "1")

	log_debug("> %q: %s TXT record", dnssd_name, info.Type)
	for _, txt := range info.Txt {
		log_debug("  %s=%s", txt.Key, txt.Value)
	}

	return
}

// getDuplex returns "T" if printer supports two-sided
// printing, "F" if not and "" if it cant' tell
func (attrs ippAttrs) getDuplex() string {
	vals := attrs.getAttr(goipp.TypeString, "sides-supported")
	one, two := false, false
	for _, v := range vals {
		s := string(v.(goipp.String))
		switch {
		case strings.HasPrefix(s, "one"):
			one = true
		case strings.HasPrefix(s, "two"):
			two = true
		}
	}

	if two {
		return "T"
	}

	if one {
		return "F"
	}

	return ""
}

// Get a single-string attribute
func (attrs ippAttrs) strSingle(names ...string) string {
	strs := attrs.getStrings(names...)
	if strs == nil {
		return ""
	}

	return strs[0]
}

// Get a multi-string attribute, represented as a comma-separated list
func (attrs ippAttrs) strJoined(names ...string) string {
	strs := attrs.getStrings(names...)
	return strings.Join(strs, ",")
}

// Get a single string, and put it into brackets
func (attrs ippAttrs) strBrackets(names ...string) string {
	s := attrs.strSingle(names...)
	if s != "" {
		s = "(" + s + ")"
	}
	return s
}

// Get attribute's []string value by attribute name
// Multiple names may be specified, for fallback purposes
func (attrs ippAttrs) getStrings(names ...string) []string {
	vals := attrs.getAttr(goipp.TypeString, names...)
	strs := make([]string, len(vals))
	for i := range vals {
		strs[i] = string(vals[i].(goipp.String))
	}

	return strs
}

// Get boolean attribute. Returns "F" or "T" if attribute is found,
// empty string otherwise.
// Multiple names may be specified, for fallback purposes
func (attrs ippAttrs) getBool(names ...string) string {
	vals := attrs.getAttr(goipp.TypeBoolean, names...)
	if vals == nil {
		return ""
	}
	if vals[0].(goipp.Boolean) {
		return "T"
	}
	return "F"
}

// Get attribute's value by attribute name
// Multiple names may be specified, for fallback purposes
// Value type is checked and enforced
func (attrs ippAttrs) getAttr(t goipp.Type, names ...string) []goipp.Value {

	for _, name := range names {
		v, ok := attrs[name]
		if ok && v[0].V.Type() == t {
			var vals []goipp.Value
			for i := range v {
				vals = append(vals, v[i].V)
			}
			return vals
		}
	}

	return nil
}
