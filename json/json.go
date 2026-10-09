// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package json reads a JSON value into a [richdoc.Document].
//
// # The one decision worth describing
//
// ⛔ An array of objects that share their keys becomes a TABLE. Everything
// else becomes nested lists.
//
// That is the shape almost every JSON export has — an API page, a database
// dump, a log — and rendering it as six hundred nested bullet lists produces
// something nobody can read, from data that was a grid all along. Rendering a
// genuine tree as a table, on the other hand, would require inventing columns.
// So the test is strict: every element an object, and every object the same
// keys in the same order.
//
// A heterogeneous array falls back to a list, which is never wrong, only
// verbose.
package json

import (
	"bytes"
	encodingjson "encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"

	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/richdoc"
)

// Options says how to read.
type Options struct {
	// Title, when set, becomes a level-1 heading above the document.
	Title string

	// NoTables keeps arrays of objects as lists. The table shape is almost
	// always what a reader wants; this is for when the array's order or its
	// nesting matters more than its grid.
	NoTables bool

	// MaxDepth, MaxNodes, MaxRows and MaxColumns override the ceilings in
	// [github.com/go-richdoc/data/shape]. Zero means those.
	MaxDepth, MaxNodes, MaxRows, MaxColumns int
}

// Parse reads one JSON value.
func Parse(src []byte, opt Options) (*richdoc.Document, error) {
	if len(src) > shape.MaxInputBytes {
		return nil, shape.LimitError{What: "bytes", Limit: shape.MaxInputBytes}
	}
	c := newConv(opt)

	// ⛔ A decoder with UseNumber, so 64-bit integers survive. encoding/json's
	// default turns every number into a float64, and an identifier of more
	// than fifteen digits — a Twitter id, a Snowflake, a bank reference —
	// comes back with its last digits replaced by zeros. Nothing errors; the
	// document is simply wrong in a way that looks plausible.
	dec := encodingjson.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("reading JSON: %w", err)
	}

	blocks, err := c.value(v, 1)
	if err != nil {
		return nil, err
	}
	if opt.Title != "" {
		blocks = append([]richdoc.Block{shape.Heading(1, opt.Title)}, blocks...)
	}
	return &richdoc.Document{Blocks: blocks}, nil
}

// ParseReader is Parse over a reader, bounded at [shape.MaxInputBytes].
func ParseReader(r io.Reader, opt Options) (*richdoc.Document, error) {
	b, err := io.ReadAll(io.LimitReader(r, shape.MaxInputBytes+1))
	if err != nil {
		return nil, err
	}
	return Parse(b, opt)
}

type conv struct {
	opt                                  Options
	nodes                                int
	maxDepth, maxNodes, maxRows, maxCols int
}

func newConv(opt Options) *conv {
	c := &conv{opt: opt,
		maxDepth: opt.MaxDepth, maxNodes: opt.MaxNodes,
		maxRows: opt.MaxRows, maxCols: opt.MaxColumns}
	if c.maxDepth <= 0 {
		c.maxDepth = shape.MaxDepth
	}
	if c.maxNodes <= 0 {
		c.maxNodes = shape.MaxNodes
	}
	if c.maxRows <= 0 {
		c.maxRows = shape.MaxRows
	}
	if c.maxCols <= 0 {
		c.maxCols = shape.MaxColumns
	}
	return c
}

// value turns one JSON value into blocks.
func (c *conv) value(v any, depth int) ([]richdoc.Block, error) {
	if depth > c.maxDepth {
		return nil, shape.LimitError{What: "depth", Limit: c.maxDepth}
	}
	c.nodes++
	if c.nodes > c.maxNodes {
		return nil, shape.LimitError{What: "nodes", Limit: c.maxNodes}
	}

	switch t := v.(type) {
	case map[string]any:
		return c.object(t, depth)
	case []any:
		return c.array(t, depth)
	default:
		return []richdoc.Block{shape.Para(scalar(v))}, nil
	}
}

// object becomes a list of key: value items, in the file's own key order where
// one is recoverable and alphabetical otherwise.
//
// ⛔ encoding/json's map loses key order, and Go randomises map iteration on
// purpose. Sorting is the only reproducible answer available: two runs over
// the same file must produce the same document, or no diff of two conversions
// means anything.
func (c *conv) object(m map[string]any, depth int) ([]richdoc.Block, error) {
	keys := sortedKeys(m)
	list := richdoc.List{Tight: true}
	for _, k := range keys {
		child := m[k]
		switch child.(type) {
		case map[string]any, []any:
			sub, err := c.value(child, depth+1)
			if err != nil {
				return nil, err
			}
			head := richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Code{Value: k}}}
			list.Items = append(list.Items, shape.Item(append([]richdoc.Block{head}, sub...)...))
		default:
			c.nodes++
			if c.nodes > c.maxNodes {
				return nil, shape.LimitError{What: "nodes", Limit: c.maxNodes}
			}
			list.Items = append(list.Items,
				shape.Item(shape.KeyValue(k, shape.Text(scalar(child)))))
		}
	}
	if len(list.Items) == 0 {
		// An object with no keys is not nothing: it is a value the file chose
		// to put there, and a document that drops it silently has lost a fact.
		return []richdoc.Block{shape.Para("{}")}, nil
	}
	return []richdoc.Block{list}, nil
}

func (c *conv) array(a []any, depth int) ([]richdoc.Block, error) {
	if len(a) == 0 {
		return []richdoc.Block{shape.Para("[]")}, nil
	}
	if !c.opt.NoTables {
		if cols, ok := uniformObjects(a); ok {
			return c.table(a, cols)
		}
	}
	list := richdoc.List{Ordered: true, Start: 1, Tight: true}
	for _, e := range a {
		sub, err := c.value(e, depth+1)
		if err != nil {
			return nil, err
		}
		list.Items = append(list.Items, shape.Item(sub...))
	}
	return []richdoc.Block{list}, nil
}

// table renders an array of uniform objects as a grid.
func (c *conv) table(a []any, cols []string) ([]richdoc.Block, error) {
	if err := shape.Over(len(a), c.maxRows, "rows"); err != nil {
		return nil, err
	}
	if err := shape.Over(len(cols), c.maxCols, "columns"); err != nil {
		return nil, err
	}
	t := richdoc.Table{
		Align:  make([]richdoc.Alignment, len(cols)),
		Header: shape.Row(cols),
	}
	for _, e := range a {
		m := e.(map[string]any)
		row := make([]richdoc.Cell, len(cols))
		for i, k := range cols {
			c.nodes++
			if c.nodes > c.maxNodes {
				return nil, shape.LimitError{What: "nodes", Limit: c.maxNodes}
			}
			row[i] = shape.Cell(scalar(m[k]))
		}
		t.Rows = append(t.Rows, row)
	}
	return []richdoc.Block{t}, nil
}

// uniformObjects says whether every element is an object with the same keys,
// and returns those keys.
//
// ⛔ Strict on purpose. A table built from "most of them agree" silently drops
// whatever the odd element held, and the element that disagrees is exactly the
// interesting one. Nested values disqualify the array too: a cell holding a
// whole sub-document is a cell nobody can read.
func uniformObjects(a []any) ([]string, bool) {
	first, ok := a[0].(map[string]any)
	if !ok || len(first) == 0 {
		return nil, false
	}
	keys := sortedKeys(first)
	for _, e := range a {
		m, ok := e.(map[string]any)
		if !ok || len(m) != len(keys) {
			return nil, false
		}
		for _, k := range keys {
			v, present := m[k]
			if !present {
				return nil, false
			}
			switch v.(type) {
			case map[string]any, []any:
				return nil, false
			}
		}
	}
	return keys, true
}

// scalar renders a JSON scalar the way the file wrote it.
func scalar(v any) string {
	switch t := v.(type) {
	case nil:
		// ⛔ "null", not "". JSON distinguishes a key whose value is null from
		// a key whose value is the empty string, and a document that renders
		// both as nothing has merged two different facts.
		return "null"
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case encodingjson.Number:
		return t.String()
	}
	// ⛔ There is no float64 case, and no default beyond this. The decoder runs
	// with UseNumber, so every number arrives as a json.Number and nothing else
	// can reach here — a first draft handled float64 and fmt.Sprint anyway, and
	// both were unreachable. An unreachable branch is not defence in depth; it
	// is a claim nobody has tested, and it hides the fact that UseNumber is
	// load-bearing rather than a nicety.
	return fmt.Sprintf("%v", v)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
