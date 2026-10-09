// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// richdata turns CSV, JSON, XML or plain text into a richdoc document, and
// writes it as Markdown.
//
//	richdata sales.csv
//	richdata -format json -title "Orders" orders.json
//	richdata -o out.md data.xml
//
// Markdown rather than PDF: this repository holds the READERS, and linking a
// six-megabyte TeX implementation into them so a command-line tool can exist
// would make every consumer of `data` pay for it. Pipe the Markdown through
// github.com/go-richdoc/markdown's own tooling, or call these packages and
// hand the document to github.com/go-richdoc/latex/pdf.
package main

import "os"

// osExit is a variable so the tests can reach the exit path without ending the
// test binary.
var osExit = os.Exit

func main() { osExit(run(os.Args[1:], os.Stdout, os.Stderr)) }
