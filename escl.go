/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * ESCL service registration
 */

package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
)

// EsclService queries eSCL ScannerCapabilities using provided
// http.Client and decodes received information into the form
// suitable for DNS-SD registration
//
// Discovered services will be added to the services collection
func EsclService(log *LogMessage, services *DNSSdServices,
	port int, usbinfo UsbDeviceInfo, ippinfo *IppPrinterInfo,
	c *http.Client) (err error) {

	uri := fmt.Sprintf("http://localhost:%d/eSCL/ScannerCapabilities", port)

	decoder := newEsclCapsDecoder(ippinfo)
	svc := DNSSdSvcInfo{
		Type: "_uscan._tcp",
		Port: port,
	}

	var xmlData []byte
	var list []string

	// Query ScannerCapabilities
	resp, err := c.Get(uri)
	if err != nil {
		goto ERROR
	}

	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		err = fmt.Errorf("HTTP status: %s", resp.Status)
		goto ERROR
	}

	xmlData, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		goto ERROR
	}

	log.Add(LogTraceESCL, '<', "ESCL Scanner Capabilities:")
	log.LineWriter(LogTraceESCL, '<').WriteClose(xmlData)
	log.Nl(LogTraceESCL)
	log.Flush()

	// Decode the XML
	err = decoder.decode(bytes.NewBuffer(xmlData))
	if err != nil {
		goto ERROR
	}

	if decoder.uuid == "" {
		decoder.uuid = usbinfo.UUID()
	}

	// If we have no data, assume eSCL response was invalud
	// If we miss some essential data, assume eSCL response was invalid
	switch {
	case decoder.version == "":
		err = errors.New("missed pwg:Version")
	case len(decoder.cs) == 0:
		err = errors.New("missed scan:ColorMode")
	case len(decoder.pdl) == 0:
		err = errors.New("missed pwg:DocumentFormat")
	case !(decoder.platen || decoder.adf):
		err = errors.New("missed pwg:DocumentFormat")
	}

	if err != nil {
		goto ERROR
	}

	// Build eSCL DNSSdInfo
	if decoder.duplex {
		svc.Txt.Add("duplex", "T")
	} else {
		svc.Txt.Add("duplex", "F")
	}

	switch {
	case decoder.platen && !decoder.adf:
		svc.Txt.Add("is", "platen")
	case !decoder.platen && decoder.adf:
		svc.Txt.Add("is", "adf")
	case decoder.platen && decoder.adf:
		svc.Txt.Add("is", "platen,adf")
	}

	list = []string{}
	for c := range decoder.cs {
		list = append(list, c)
	}
	sort.Strings(list)
	svc.Txt.IfNotEmpty("cs", strings.Join(list, ","))

	svc.Txt.IfNotEmpty("UUID", decoder.uuid)
	svc.Txt.URLIfNotEmpty("adminurl", decoder.adminurl)
	svc.Txt.URLIfNotEmpty("representation", decoder.representation)

	list = []string{}
	for p := range decoder.pdl {
		list = append(list, p)
	}
	sort.Strings(list)
	svc.Txt.AddPDL("pdl", strings.Join(list, ","))

	svc.Txt.Add("ty", usbinfo.ProductName)
	svc.Txt.Add("rs", "eSCL")
	svc.Txt.IfNotEmpty("vers", decoder.version)
	svc.Txt.IfNotEmpty("txtvers", "1")

	// Add to services
	services.Add(svc)

	return

	// Handle a error
ERROR:
	err = fmt.Errorf("eSCL: %s", err)
	return
}

// esclCapsDecoder represents eSCL ScannerCapabilities decoder
type esclCapsDecoder struct {
	uuid           string              // Device UUID
	adminurl       string              // Admin URL
	representation string              // Icon URL
	version        string              // eSCL Version
	platen, adf    bool                // Has platen/ADF
	duplex         bool                // Has duplex
	pdl, cs        map[string]struct{} // Formats/colors
}

// newesclCapsDecoder creates new esclCapsDecoder
func newEsclCapsDecoder(ippinfo *IppPrinterInfo) *esclCapsDecoder {
	decoder := &esclCapsDecoder{
		pdl: make(map[string]struct{}),
		cs:  make(map[string]struct{}),
	}

	if ippinfo != nil {
		decoder.uuid = ippinfo.UUID
		decoder.adminurl = ippinfo.AdminURL
		decoder.representation = ippinfo.IconURL
	}

	return decoder
}

// Decode scanner capabilities
func (decoder *esclCapsDecoder) decode(in io.Reader) error {
	xmlDecoder := xml.NewDecoder(in)

	var path bytes.Buffer
	var lenStack []int

	for {
		token, err := xmlDecoder.RawToken()
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			lenStack = append(lenStack, path.Len())
			path.WriteByte('/')
			path.WriteString(t.Name.Space)
			path.WriteByte(':')
			path.WriteString(t.Name.Local)
			decoder.element(path.String())

		case xml.EndElement:
			last := len(lenStack) - 1
			path.Truncate(lenStack[last])
			lenStack = lenStack[:last]

		case xml.CharData:
			data := bytes.TrimSpace(t)
			if len(data) > 0 {
				decoder.data(path.String(), string(data))
			}
		}
	}

	return nil
}

const (
	// Relative to root
	esclPlaten          = "/scan:ScannerCapabilities/scan:Platen"
	esclAdf             = "/scan:ScannerCapabilities/scan:Adf"
	esclPlatenInputCaps = esclPlaten + "/scan:PlatenInputCaps"
	esclAdfSimplexCaps  = esclAdf + "/scan:AdfSimplexInputCaps"
	esclAdfDuplexCaps   = esclAdf + "/scan:AdfDuplexInputCaps"

	// Relative to esclPlatenInputCaps, esclAdfSimplexCaps or esclAdfDuplexCaps
	esclSettingProfile    = "/scan:SettingProfiles/scan:SettingProfile"
	esclColorMode         = esclSettingProfile + "/scan:ColorModes/scan:ColorMode"
	esclDocumentFormat    = esclSettingProfile + "/scan:DocumentFormats/pwg:DocumentFormat"
	esclDocumentFormatExt = esclSettingProfile + "/scan:DocumentFormats/scan:DocumentFormatExt"
)

// handle beginning of XML element
func (decoder *esclCapsDecoder) element(path string) {
	switch path {
	case esclPlaten:
		decoder.platen = true
	case esclAdf:
		decoder.adf = true
	case esclAdfDuplexCaps:
		decoder.duplex = true
	}
}

// handle XML element data
func (decoder *esclCapsDecoder) data(path, data string) {
	switch path {
	case "/scan:ScannerCapabilities/scan:UUID":
		uuid := UUIDNormalize(data)
		if uuid != "" && decoder.uuid == "" {
			decoder.uuid = data
		}
	case "/scan:ScannerCapabilities/scan:AdminURI":
		decoder.adminurl = data
	case "/scan:ScannerCapabilities/scan:IconURI":
		decoder.representation = data
	case "/scan:ScannerCapabilities/pwg:Version":
		decoder.version = data

	case esclPlatenInputCaps + esclColorMode,
		esclAdfSimplexCaps + esclColorMode,
		esclAdfDuplexCaps + esclColorMode:

		data = strings.ToLower(data)
		switch {
		case strings.HasPrefix(data, "rgb"):
			decoder.cs["color"] = struct{}{}
		case strings.HasPrefix(data, "grayscale"):
			decoder.cs["grayscale"] = struct{}{}
		case strings.HasPrefix(data, "blackandwhite"):
			decoder.cs["binary"] = struct{}{}
		}

	case esclPlatenInputCaps + esclDocumentFormat,
		esclAdfSimplexCaps + esclDocumentFormat,
		esclAdfDuplexCaps + esclDocumentFormat:

		decoder.pdl[data] = struct{}{}

	case esclPlatenInputCaps + esclDocumentFormatExt,
		esclAdfSimplexCaps + esclDocumentFormatExt,
		esclAdfDuplexCaps + esclDocumentFormatExt:

		decoder.pdl[data] = struct{}{}
	}
}
