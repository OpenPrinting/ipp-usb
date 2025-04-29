/* Go IPP - IPP core protocol implementation in pure Go
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Package documentation
 */

/*
Package goipp implements IPP core protocol, as defined by RFC 8010

It doesn't implement high-level operations, such as "print a document",
"cancel print job" and so on. It's scope is limited to proper generation
and parsing of IPP requests and responses.

	IPP protocol uses the following simple model:
	1. Send a request
	2. Receive a response

Request and response both has a similar format, represented here
by type Message, with the only difference, that Code field of
that Message is the Operation code in request and Status code
in response. So most of operations are common for request and
response messages

# Example (Get-Printer-Attributes):

	package main

	import (
		"bytes"
		"net/http"
		"os"

		"github.com/OpenPrinting/goipp"
	)

	const uri = "http://192.168.1.102:631"

	// Build IPP OpGetPrinterAttributes request
	func makeRequest() ([]byte, error) {
		m := goipp.NewRequest(goipp.DefaultVersion, goipp.OpGetPrinterAttributes, 1)
		m.Operation.Add(goipp.MakeAttribute("attributes-charset",
			goipp.TagCharset, goipp.String("utf-8")))
		m.Operation.Add(goipp.MakeAttribute("attributes-natural-language",
			goipp.TagLanguage, goipp.String("en-US")))
		m.Operation.Add(goipp.MakeAttribute("printer-uri",
			goipp.TagURI, goipp.String(uri)))
		m.Operation.Add(goipp.MakeAttribute("requested-attributes",
			goipp.TagKeyword, goipp.String("all")))

		return m.EncodeBytes()
	}

	// Check that there is no error
	func check(err error) {
		if err != nil {
			panic(err)
		}
	}

	func main() {
		request, err := makeRequest()
		check(err)

		resp, err := http.Post(uri, goipp.ContentType, bytes.NewBuffer(request))
		check(err)

		var respMsg goipp.Message

		err = respMsg.Decode(resp.Body)
		check(err)

		respMsg.Print(os.Stdout, false)
	}

# Example (Print PDF file):

	package main

	import (
		"bytes"
		"errors"
		"fmt"
		"io"
		"net/http"
		"os"

		"github.com/OpenPrinting/goipp"
	)

	const (
		PrinterURL = "ipp://192.168.1.102:631/ipp/print"
		TestPage   = "onepage-a4.pdf"
	)

	// checkErr checks for an error. If err != nil, it prints error
	// message and exits
	func checkErr(err error, format string, args ...interface{}) {
		if err != nil {
			msg := fmt.Sprintf(format, args...)
			fmt.Fprintf(os.Stderr, "%s: %s\n", msg, err)
			os.Exit(1)
		}
	}

	// ExamplePrintPDF demo
	func main() {
		// Build and encode IPP request
		req := goipp.NewRequest(goipp.DefaultVersion, goipp.OpPrintJob, 1)
		req.Operation.Add(goipp.MakeAttribute("attributes-charset",
			goipp.TagCharset, goipp.String("utf-8")))
		req.Operation.Add(goipp.MakeAttribute("attributes-natural-language",
			goipp.TagLanguage, goipp.String("en-US")))
		req.Operation.Add(goipp.MakeAttribute("printer-uri",
			goipp.TagURI, goipp.String(PrinterURL)))
		req.Operation.Add(goipp.MakeAttribute("requesting-user-name",
			goipp.TagName, goipp.String("John Doe")))
		req.Operation.Add(goipp.MakeAttribute("job-name",
			goipp.TagName, goipp.String("job name")))
		req.Operation.Add(goipp.MakeAttribute("document-format",
			goipp.TagMimeType, goipp.String("application/pdf")))

		payload, err := req.EncodeBytes()
		checkErr(err, "IPP encode")

		// Open document file
		file, err := os.Open(TestPage)
		checkErr(err, "Open document file")

		defer file.Close()

		// Build HTTP request
		body := io.MultiReader(bytes.NewBuffer(payload), file)

		httpReq, err := http.NewRequest(http.MethodPost, PrinterURL, body)
		checkErr(err, "HTTP")

		httpReq.Header.Set("content-type", goipp.ContentType)
		httpReq.Header.Set("accept", goipp.ContentType)

		// Execute HTTP request
		httpRsp, err := http.DefaultClient.Do(httpReq)
		if httpRsp != nil {
			defer httpRsp.Body.Close()
		}

		checkErr(err, "HTTP")

		if httpRsp.StatusCode/100 != 2 {
			checkErr(errors.New(httpRsp.Status), "HTTP")
		}

		// Decode IPP response
		rsp := &goipp.Message{}
		err = rsp.Decode(httpRsp.Body)
		checkErr(err, "IPP decode")

		if goipp.Status(rsp.Code) != goipp.StatusOk {
			err = errors.New(goipp.Status(rsp.Code).String())
			checkErr(err, "IPP")
		}
	}
*/
package goipp
