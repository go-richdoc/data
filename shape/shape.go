// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package shape holds what the four readers in this repository have in common:
// the ceilings they all need, and the handful of decisions about when a tree
// becomes a table and when it becomes a list.
//
// ⛔ It exists so those decisions are made ONCE. Four copies of "an array of
// objects with the same keys is a table" is four places for it to drift, and a
// drift between two converters is invisible until somebody compares their
// output.
package shape

import (
	"fmt"

	"github.com/go-richdoc/richdoc"
)

// None of these formats IS a document, and every one of them lets a small file
// ask for a large one. A CSV row can declare a million columns; a JSON array
// can nest ten thousand deep; an XML element can do the same.
//
// ⛔ These are ceilings on what is BUILT, not on what is read. The input is
// bounded separately by [MaxInputBytes], because a reader that refuses a
// gigabyte of JSON only after parsing it has already held the gigabyte.
const (
	// MaxInputBytes is how much source any reader here will take.
	MaxInputBytes = 64 << 20

	// MaxRows and MaxColumns bound a table. Past these the document stops
	// being something a person reads and starts being something that fills
	// memory, and the formats that feed it have no limits of their own.
	MaxRows    = 100_000
	MaxColumns = 1_000

	// MaxDepth is how far a tree may nest. JSON and XML are both recursive,
	// and the depth is chosen by the file.
	//
	// ⛔ encoding/json and encoding/xml are iterative enough not to overflow a
	// stack here, but the DOCUMENT built from a deep tree is nested lists all
	// the way down, and every consumer of richdoc walks it recursively. The
	// ceiling protects them, not us.
	MaxDepth = 100

	// MaxNodes is how many values a tree may hold in total. Depth and breadth
	// are different ways to be large, and bounding only one leaves the other.
	MaxNodes = 500_000
)

// A LimitError says which ceiling was reached and what it is, because the whole
// value of a refusal is that the person reading it can tell a hostile file from
// a legitimately enormous one.
type LimitError struct {
	What  string // "rows", "columns", "depth", "nodes", "bytes"
	Limit int
}

func (e LimitError) Error() string {
	return fmt.Sprintf("more %s than the %d allowed", e.What, e.Limit)
}

// Over returns a LimitError when n is past limit, and nil otherwise.
func Over(n, limit int, what string) error {
	if n > limit {
		return LimitError{What: what, Limit: limit}
	}
	return nil
}

// Text is a cell or an inline run holding exactly this string.
func Text(s string) []richdoc.Inline {
	if s == "" {
		// ⛔ A cell of no inlines, rather than one holding an empty Text.
		// Writers differ on what they do with a zero-length run — some emit
		// nothing, some emit an empty element that a reader then sees as
		// content — and an empty cell is a thing a table has a lot of.
		return nil
	}
	return []richdoc.Inline{richdoc.Text{Value: s}}
}

// Cell is a table cell holding one string.
func Cell(s string) richdoc.Cell { return richdoc.Cell{Inlines: Text(s)} }

// Row turns a slice of strings into a row of cells.
func Row(ss []string) []richdoc.Cell {
	out := make([]richdoc.Cell, len(ss))
	for i, s := range ss {
		out[i] = Cell(s)
	}
	return out
}

// Para is a paragraph holding one string.
func Para(s string) richdoc.Paragraph { return richdoc.Paragraph{Inlines: Text(s)} }

// Heading is a heading holding one string.
//
// ⛔ A VALUE, not a pointer. richdoc's blocks are a closed interface set whose
// marker method has a value receiver, so *Heading satisfies Block too — it
// compiles, it type-switches, and it matches no case any consumer wrote. A
// sibling converter returned one and the headings reached the document, the
// block count was right, and they came out of the PDF as nothing at all.
func Heading(level int, s string) richdoc.Heading {
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}
	return richdoc.Heading{Level: level, Inlines: Text(s)}
}

// KeyValue is one entry of a mapping: the key in bold, then its value.
//
// ⛔ Code rather than Strong for the key. A key is a literal string from a
// file — it may be "0", or "true", or contain a bracket — and setting it in
// the body face invites it to be read as prose. A monospaced run says "this is
// what the file says", which is exactly what it is.
func KeyValue(key string, value []richdoc.Inline) richdoc.Paragraph {
	ins := []richdoc.Inline{richdoc.Code{Value: key}, richdoc.Text{Value: ": "}}
	return richdoc.Paragraph{Inlines: append(ins, value...)}
}

// Item wraps blocks as one list item.
func Item(blocks ...richdoc.Block) richdoc.ListItem {
	return richdoc.ListItem{Blocks: blocks}
}
