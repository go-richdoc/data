// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package csv reads delimiter-separated text into a [richdoc.Document] holding
// one [richdoc.Table].
//
// # What it decides, and what it refuses to guess
//
// ⛔ The DELIMITER is sniffed, because a file called .csv is as likely to be
// semicolon-separated as comma-separated — that is what a French or German
// spreadsheet writes, and a reader that assumes a comma turns every row into a
// single column without saying so. What it will not guess is whether the first
// row is a HEADER: a file of numbers whose first row is also numbers has no
// tell, and quietly promoting data to a header loses a row. [Options.Header]
// decides, and its zero value is Detect, which promotes only when the first row
// is text and the second is not.
package csv

import (
	"bytes"
	encodingcsv "encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/richdoc"
)

// HeaderMode says what to do with the first row.
type HeaderMode int

const (
	// Detect promotes the first row to a header when it looks like one: every
	// cell non-empty and not a number, and at least one cell of the second row
	// a number. A single-row file is never a header on its own — a header with
	// nothing under it is a table with no data, which is not what a one-row
	// file is.
	Detect HeaderMode = iota
	// Always promotes the first row.
	Always
	// Never keeps every row as data.
	Never
)

// String names the mode, so a test and an error message can say which one is
// meant without repeating the mapping.
func (m HeaderMode) String() string {
	switch m {
	case Always:
		return "always"
	case Never:
		return "never"
	default:
		return "detect"
	}
}

// Options says how to read.
type Options struct {
	// Delimiter is the field separator. Zero means sniff it.
	Delimiter rune

	// Header decides the first row. See [HeaderMode].
	Header HeaderMode

	// Caption is the table's caption, if any.
	Caption string

	// MaxRows and MaxColumns override the ceilings in
	// [github.com/go-richdoc/data/shape]. Zero means those.
	MaxRows    int
	MaxColumns int
}

// candidates are the delimiters worth sniffing, in the order a tie is broken.
//
// ⛔ Comma first, because it is what the format is named after; semicolon
// second, because it is what a spreadsheet writes wherever the comma is a
// decimal point. Tab and pipe follow.
var candidates = []rune{',', ';', '\t', '|'}

// Parse reads delimiter-separated text.
func Parse(src []byte, opt Options) (*richdoc.Document, error) {
	if len(src) > shape.MaxInputBytes {
		return nil, shape.LimitError{What: "bytes", Limit: shape.MaxInputBytes}
	}
	maxRows, maxCols := opt.MaxRows, opt.MaxColumns
	if maxRows <= 0 {
		maxRows = shape.MaxRows
	}
	if maxCols <= 0 {
		maxCols = shape.MaxColumns
	}

	delim := opt.Delimiter
	if delim == 0 {
		delim = Sniff(src)
	}

	r := encodingcsv.NewReader(bytes.NewReader(src))
	r.Comma = delim
	// ⛔ A ragged file is read, not refused. Exports are ragged all the time —
	// a trailing comma, a short last row — and refusing the whole document
	// over one row is a worse answer than a table with a gap in it. The rows
	// are squared off below, so what comes out is still a rectangle.
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	var rows [][]string
	width := 0
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading row %d: %w", len(rows)+1, err)
		}
		if len(rec) > maxCols {
			return nil, shape.LimitError{What: "columns", Limit: maxCols}
		}
		if len(rec) > width {
			width = len(rec)
		}
		rows = append(rows, rec)
		if len(rows) > maxRows {
			return nil, shape.LimitError{What: "rows", Limit: maxRows}
		}
	}
	// ⛔ Said out loud. An empty document and "this file held no rows" are
	// different answers, and a caller handed the first one has no way to tell
	// which happened.
	//
	// ⛔ And a file of BLANK LINES is empty too, which took a test to notice:
	// encoding/csv reads a line of spaces as a record holding one field, so a
	// whitespace-only file came back as a one-by-one table containing nothing,
	// with no error. A blank row BETWEEN full ones is data — a gap the file
	// chose — so only a file that is blank throughout is refused.
	if len(rows) == 0 || allBlank(rows) {
		return nil, errors.New("no rows: an empty table is not a conversion")
	}

	// Square the rows off, so every consumer sees a rectangle.
	for i := range rows {
		for len(rows[i]) < width {
			rows[i] = append(rows[i], "")
		}
	}

	t := richdoc.Table{Align: make([]richdoc.Alignment, width)}
	if opt.Caption != "" {
		t.Caption = shape.Text(opt.Caption)
	}
	body := rows
	if headerWanted(opt.Header, rows) {
		t.Header = shape.Row(rows[0])
		body = rows[1:]
	}
	for _, r := range body {
		t.Rows = append(t.Rows, shape.Row(r))
	}
	return &richdoc.Document{Blocks: []richdoc.Block{t}}, nil
}

// ParseReader is Parse over a reader, bounded at [shape.MaxInputBytes].
func ParseReader(r io.Reader, opt Options) (*richdoc.Document, error) {
	b, err := io.ReadAll(io.LimitReader(r, shape.MaxInputBytes+1))
	if err != nil {
		return nil, err
	}
	return Parse(b, opt)
}

// headerWanted applies [HeaderMode] to the rows actually read.
func headerWanted(mode HeaderMode, rows [][]string) bool {
	switch mode {
	case Always:
		return true
	case Never:
		return false
	}
	// ⛔ Detect, and it is deliberately timid. Promoting a row of data to a
	// header LOSES it, and a reader cannot get it back; leaving a header as
	// data is ugly and recoverable. So it promotes only on evidence: every
	// first-row cell non-empty and not a number, and the second row holding at
	// least one number.
	if len(rows) < 2 {
		return false
	}
	for _, c := range rows[0] {
		if strings.TrimSpace(c) == "" || isNumber(c) {
			return false
		}
	}
	for _, c := range rows[1] {
		if isNumber(c) {
			return true
		}
	}
	return false
}

// allBlank says whether every cell of every row is empty or whitespace.
func allBlank(rows [][]string) bool {
	for _, r := range rows {
		for _, c := range r {
			if strings.TrimSpace(c) != "" {
				return false
			}
		}
	}
	return true
}

func isNumber(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	// A spreadsheet in a comma-decimal locale writes 3,14 — but only where the
	// delimiter is not a comma, so by the time this is asked the field is
	// already one field.
	if _, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64); err == nil {
		return true
	}
	return false
}

// Sniff says which delimiter a file uses.
//
// ⛔ It counts over the first few LINES rather than the whole file, and it
// prefers the delimiter whose count is the same on every line. A file of prose
// containing commas has an unsteady comma count; a two-column semicolon file
// has exactly one semicolon on each line. Steadiness is the signal, not
// frequency — "the most common character" picks the comma out of English text
// every time.
func Sniff(src []byte) rune {
	lines := firstLines(src, 10)
	if len(lines) == 0 {
		return ','
	}
	best, bestScore := ',', -1
	for _, c := range candidates {
		n := countOutsideQuotes(lines[0], c)
		if n == 0 {
			continue
		}
		steady := true
		for _, l := range lines[1:] {
			if countOutsideQuotes(l, c) != n {
				steady = false
				break
			}
		}
		score := n
		if steady {
			score += 1000
		}
		if score > bestScore {
			best, bestScore = c, score
		}
	}
	return best
}

// countOutsideQuotes counts r in a line, ignoring anything inside double
// quotes — a quoted field holding commas is exactly what a sniffer must not
// count.
func countOutsideQuotes(line string, r rune) int {
	n, inQuotes := 0, false
	for _, c := range line {
		switch {
		case c == '"':
			inQuotes = !inQuotes
		case c == r && !inQuotes:
			n++
		}
	}
	return n
}

// firstLines returns up to n non-empty lines, without their endings.
func firstLines(src []byte, n int) []string {
	if !utf8.Valid(src) {
		// Not an error: a CSV may be in any encoding, and the delimiters this
		// sniffs for are all ASCII. Reading it as Latin-1 would still find
		// them, which is all this needs.
		src = []byte(string(bytes.ToValidUTF8(src, nil)))
	}
	var out []string
	for _, l := range strings.Split(string(src), "\n") {
		l = strings.TrimSuffix(l, "\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		out = append(out, l)
		if len(out) == n {
			break
		}
	}
	return out
}
