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
	// ⛔ Neither decoder can overflow a stack here, but NOT because either is
	// iterative — encoding/json's decode of a value into an any does recurse,
	// one frame per level. It cannot run away because the scanner refuses past
	// a hard constant: maxNestingDepth = 10000, encoding/json/scanner.go:148,
	// and the same constant again at encoding/json/jsontext/state.go:53 for the
	// v2 path, so GOEXPERIMENT=jsonv2 does not change the answer. encoding/xml
	// hands out tokens and keeps no tree, so its nesting is whatever the caller
	// stacks — ours, bounded here.
	//
	// What this ceiling is actually for is the DOCUMENT: a deep tree becomes
	// nested lists all the way down, and every consumer of richdoc walks those
	// recursively with no constant of its own. The ceiling protects them.
	MaxDepth = 100

	// MaxNodes is how many values a tree may hold in total. Depth and breadth
	// are different ways to be large, and bounding only one leaves the other.
	MaxNodes = 500_000

	// MaxSourceValues is how many JSON values the SOURCE may declare, counted
	// in a pass over the bytes before anything is decoded.
	//
	// ⛔ IT IS A DIFFERENT QUANTITY FROM MaxNodes, on purpose. MaxNodes bounds
	// what is BUILT and is checked while building; it stopped the answer and
	// not the work, because encoding/json decodes the whole value into an any
	// BEFORE the converter sees its first node. Measured: two flat arrays both
	// refused at node 500 001, the larger one eight times the source, cost 5.2
	// times the allocation — 243 MB against 1.275 GB. The XML reader, which
	// counts inside its own token walk, gave 1.0 on the same test.
	//
	// ⛔ THE VALUE OF THE CONSTANT IS DERIVED, NOT CHOSEN. It must never refuse
	// a document the converter would have accepted, so it needs a floor on the
	// nodes the converter will count given v values in the source. The thinnest
	// case is the TABLE path: an array of n objects of k keys is 1+n+nk values
	// in the source and 1+nk nodes in the converter, because table() visits the
	// cells and not the objects. That ratio is worst at k=1, where the
	// converter counts (v-1)/2. So v > 2*MaxNodes+2 implies more than MaxNodes
	// nodes, and a refusal here is one the converter was going to make anyway.
	// TestTheSourceCeilingNeverRefusesWhatTheConverterWouldAccept keeps it true.
	MaxSourceValues = 2*MaxNodes + 2

	// MaxCells is how many cells a table may hold.
	//
	// ⛔ MaxRows and MaxColumns MULTIPLY, and nothing bounded the product: one
	// hundred thousand rows of a thousand columns is a hundred million cells,
	// each a richdoc.Cell holding an inline run — measured at 120 bytes
	// allocated per cell, so 12 GB from a file the input ceiling allows.
	//
	// ⛔ The value is the tree readers' MaxNodes, because it is the same
	// quantity under another name: a cell is a value a document holds. The
	// defect it closes is an INCONSISTENCY, not a taste — the same export of
	// 100 000 rows by 20 fields was already refused as JSON, at 2 000 000
	// nodes, and accepted as CSV at 2 000 000 cells. Two formats of one
	// repository answering differently about one quantity of data is the thing
	// package shape exists to prevent. A caller who wants the old ceiling says
	// so with Options.MaxCells.
	MaxCells = MaxNodes
)

// A LimitError says which ceiling was reached and what it is, because the whole
// value of a refusal is that the person reading it can tell a hostile file from
// a legitimately enormous one.
type LimitError struct {
	What  string // "rows", "columns", "cells", "depth", "nodes", "values", "bytes"
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
