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
	"net/http"

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
	err = msg.Decode(resp.Body)
	resp.Body.Close()

	if err != nil {
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

// Decode printer attributes
func (attrs ippAttrs) Decode() (dnssd_name string, info DnsSdInfo) {
	var ok bool
	dnssd_name, ok = attrs.getString("printer-dns-sd-name", "printer-info")

	_ = ok

	return
}

// Get attribute's string value by attribute name
// Multiple names may be specified, for fallback purposes
func (attrs ippAttrs) getString(names ...string) (string, bool) {
	vals, ok := attrs.getAttr(goipp.TypeString, names...)
	if !ok {
		return "", ok
	}

	return string(vals[0].(goipp.String)), true
}

// Get attribute's []string value by attribute name
// Multiple names may be specified, for fallback purposes
func (attrs ippAttrs) getStrings(names ...string) ([]string, bool) {
	vals, ok := attrs.getAttr(goipp.TypeString, names...)
	if !ok {
		return nil, ok
	}

	strings := make([]string, len(vals))
	for i := range vals {
		strings[i] = string(vals[i].(goipp.String))
	}

	return strings, true
}

// Get attribute's value by attribute name
// Multiple names may be specified, for fallback purposes
// Value type is checked and enforced
func (attrs ippAttrs) getAttr(t goipp.Type, names ...string) ([]goipp.Value, bool) {

	for _, name := range names {
		v, ok := attrs[name]
		if ok && v[0].V.Type() == t {
			var vals []goipp.Value
			for i := range v {
				vals = append(vals, v[i].V)
			}
			return vals, true
		}
	}

	return nil, false
}
