// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-richdoc/data/csv"
	"github.com/go-richdoc/data/json"
	"github.com/go-richdoc/data/text"
	"github.com/go-richdoc/data/xml"
	"github.com/go-richdoc/markdown"
	"github.com/go-richdoc/richdoc"
)

// writeMarkdown is a variable so a test can reach the path where writing
// fails. It cannot be made to fail from here with a document these readers
// produce — which is exactly why the branch would otherwise be a claim nobody
// has tested.
var writeMarkdown = markdown.Write

func run(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("richdata", flag.ContinueOnError)
	fs.SetOutput(errw)
	format := fs.String("format", "", "csv, json, xml or text; empty means decide from the name")
	title := fs.String("title", "", "a level-1 heading above the document")
	outPath := fs.String("o", "", "where to write; empty means standard output")
	header := fs.String("header", "detect", "CSV first row: detect, always or never")
	delim := fs.String("delimiter", "", "CSV field separator; empty means sniff it")
	verbatim := fs.Bool("verbatim", false, "text: keep the whole file as one code block")
	noTables := fs.Bool("no-tables", false, "json, xml: keep repeated records as lists")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(errw, "richdata: exactly one input file")
		return 2
	}
	in := fs.Arg(0)

	kind := strings.ToLower(*format)
	if kind == "" {
		kind = fromName(in)
	}
	if kind == "" {
		// ⛔ Refused rather than guessed. A file with no extension could be
		// any of the four, and reading JSON as plain text produces a document
		// that looks fine and says nothing.
		fmt.Fprintf(errw, "richdata: cannot tell what %q is; pass -format "+
			"(csv, json, xml or text)\n", filepath.Base(in))
		return 2
	}

	src, err := os.ReadFile(in)
	if err != nil {
		fmt.Fprintln(errw, "richdata:", err)
		return 1
	}

	doc, err := convert(kind, src, options{
		title: *title, header: *header, delim: *delim,
		verbatim: *verbatim, noTables: *noTables,
	})
	if err != nil {
		fmt.Fprintf(errw, "richdata: %s: %v\n", filepath.Base(in), err)
		return 1
	}

	md, err := writeMarkdown(doc)
	if err != nil {
		fmt.Fprintln(errw, "richdata: writing Markdown:", err)
		return 1
	}
	if *outPath == "" {
		out.Write(md)
		return 0
	}
	if err := os.WriteFile(*outPath, md, 0o644); err != nil {
		fmt.Fprintln(errw, "richdata:", err)
		return 1
	}
	fmt.Fprintf(out, "%s: %d blocks, %d bytes\n", *outPath, len(doc.Blocks), len(md))
	return 0
}

type options struct {
	title, header, delim string
	verbatim, noTables   bool
}

func convert(kind string, src []byte, o options) (*richdoc.Document, error) {
	switch kind {
	case "csv":
		mode, err := headerMode(o.header)
		if err != nil {
			return nil, err
		}
		var d rune
		if o.delim != "" {
			r := []rune(o.delim)
			if len(r) != 1 {
				return nil, fmt.Errorf("a delimiter is one character, not %q", o.delim)
			}
			d = r[0]
		}
		return csv.Parse(src, csv.Options{Delimiter: d, Header: mode, Caption: o.title})
	case "json":
		return json.Parse(src, json.Options{Title: o.title, NoTables: o.noTables})
	case "xml":
		return xml.Parse(src, xml.Options{Title: o.title, NoTables: o.noTables})
	case "text", "txt":
		return text.Parse(src, text.Options{Title: o.title, Verbatim: o.verbatim})
	default:
		return nil, fmt.Errorf("unknown format %q: csv, json, xml or text", kind)
	}
}

func headerMode(s string) (csv.HeaderMode, error) {
	switch strings.ToLower(s) {
	case "", "detect":
		return csv.Detect, nil
	case "always", "yes":
		return csv.Always, nil
	case "never", "no":
		return csv.Never, nil
	default:
		return csv.Detect, fmt.Errorf("-header is detect, always or never, not %q", s)
	}
}

// fromName reads the format off a file's extension, and returns "" where it
// cannot — the caller refuses rather than guesses.
func fromName(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv", ".tsv":
		return "csv"
	case ".json", ".ndjson":
		return "json"
	case ".xml":
		return "xml"
	case ".txt", ".text", ".log":
		return "text"
	default:
		return ""
	}
}
