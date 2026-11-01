// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
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
	"github.com/spf13/cobra"
)

// writeMarkdown is a variable so a test can reach the path where writing
// fails. It cannot be made to fail from here with a document these readers
// produce — which is exactly why the branch would otherwise be a claim nobody
// has tested.
var writeMarkdown = markdown.Write

// usageError is a refusal about the COMMAND LINE rather than about the file, so
// that run can answer 2 for one and 1 for the other.
//
// ⛔ The distinction is the whole reason this type exists. A script that cannot
// tell "you called me wrongly" from "the file is bad" will retry the second
// case forever and give up on the first — and cobra reports both as a plain
// error, so the classification has to be carried in the error's type.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

func usagef(format string, a ...any) error {
	return usageError{fmt.Errorf(format, a...)}
}

// RootCmd is the command, built fresh so that two tests cannot share one
// command's parsed flags.
func RootCmd() *cobra.Command {
	var (
		format, title, outPath string
		header, delim          string
		verbatim, noTables     bool
	)

	cmd := &cobra.Command{
		Use:   "richdata FILE",
		Short: "Convert CSV, JSON, XML or plain text into a Markdown document",
		Long: "Convert CSV, JSON, XML or plain text into a Markdown document.\n\n" +
			"The format is read from the file's extension unless --format says otherwise.\n" +
			"A file with no extension is REFUSED rather than guessed: reading JSON as\n" +
			"plain text produces a document that looks fine and says nothing.",
		// ⛔ A validator of its own rather than cobra.ExactArgs(1), for two
		// reasons. It keeps this command's own wording — cobra says "accepts 1
		// arg(s), received 0", which names its own API rather than the thing
		// the caller got wrong — and it makes the refusal a usageError, so the
		// exit status is decided by a TYPE instead of by matching cobra's text.
		// Every case moved out of the text matching below is one fewer thing an
		// upgrade can reword underneath us.
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usagef("exactly one input file, and %d given", len(args))
			}
			return nil
		},
		// ⛔ SilenceUsage, because a refusal here is about a file or a value and
		// forty lines of usage under it buries the one line that matters.
		// SilenceErrors too: cobra would print "Error: ..." and every message
		// this command writes is prefixed "richdata:", which run does below.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := args[0]

			kind := strings.ToLower(format)
			if kind == "" {
				kind = fromName(in)
			}
			if kind == "" {
				// ⛔ Refused rather than guessed. A file with no extension could
				// be any of the four.
				return usagef("cannot tell what %q is; pass --format "+
					"(csv, json, xml or text)", filepath.Base(in))
			}

			src, err := os.ReadFile(in)
			if err != nil {
				return err
			}

			doc, err := convert(kind, src, options{
				title: title, header: header, delim: delim,
				verbatim: verbatim, noTables: noTables,
			})
			if err != nil {
				return fmt.Errorf("%s: %w", filepath.Base(in), err)
			}

			md, err := writeMarkdown(doc)
			if err != nil {
				return fmt.Errorf("writing Markdown: %w", err)
			}
			if outPath == "" {
				cmd.OutOrStdout().Write(md)
				return nil
			}
			if err := os.WriteFile(outPath, md, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %d blocks, %d bytes\n",
				outPath, len(doc.Blocks), len(md))
			return nil
		},
	}

	// ⛔ POSIX long flags with shorthands, which is what the rest of this
	// fleet's commands take. The previous version used the standard flag
	// package, so these were spelled -format and -no-tables; under pflag a
	// single dash introduces SHORTHANDS, and -format would be read as -f -o -r
	// -m -a -t. That is a breaking change to the command line and the reason
	// this release is a minor bump rather than a patch.
	f := cmd.Flags()
	f.StringVarP(&format, "format", "f", "", "csv, json, xml or text; empty means decide from the name")
	f.StringVarP(&title, "title", "t", "", "a level-1 heading above the document")
	f.StringVarP(&outPath, "output", "o", "", "where to write; empty means standard output")
	f.StringVar(&header, "header", "detect", "CSV first row: detect, always or never")
	f.StringVarP(&delim, "delimiter", "d", "", "CSV field separator; empty means sniff it")
	f.BoolVar(&verbatim, "verbatim", false, "text: keep the whole file as one code block")
	f.BoolVar(&noTables, "no-tables", false, "json, xml: keep repeated records as lists")
	return cmd
}

// run is the command's exit status, which is what main needs and what the tests
// drive.
//
// ⛔ TWO FAILING STATUSES, NOT ONE. 2 says the command line was wrong — a bad
// flag, the wrong number of arguments, a file whose format cannot be told — and
// 1 says the work failed: the file could not be read, parsed or written. A
// caller that cannot tell them apart cannot decide whether retrying is
// sensible.
func run(args []string, out, errw io.Writer) int {
	cmd := RootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(out)
	cmd.SetErr(errw)

	err := cmd.Execute()
	if err == nil {
		return 0
	}
	fmt.Fprintln(errw, "richdata:", err)

	var ue usageError
	if errors.As(err, &ue) {
		return 2
	}
	// ⛔ What is left is pflag refusing a FLAG — an unknown one, or one whose
	// value is missing. Those are command-line errors and answer 2. pflag
	// offers no type or sentinel for them, so they are recognised by their
	// wording, which is why the argument count above was given a validator of
	// its own: the less this has to match, the less an upgrade can break.
	if isCommandLineError(err) {
		return 2
	}
	return 1
}

// isCommandLineError says whether pflag refused a flag before the command's
// own work began.
//
// ⛔ BY TEXT, because pflag offers no type and no sentinel for it — checked
// against pflag v1.0.9, where these strings are built with fmt.Errorf as it
// parses. Every one of them is pinned by a test, so an upgrade that rewords one
// fails there rather than silently turning a usage error into an exit status of
// 1. Anything this command refuses itself is a usageError and never reaches
// here.
func isCommandLineError(err error) bool {
	s := err.Error()
	for _, marker := range []string{
		"unknown flag",
		"unknown shorthand flag",
		"flag needs an argument",
		"invalid argument",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
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
		return csv.Detect, fmt.Errorf("--header is detect, always or never, not %q", s)
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
