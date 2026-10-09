// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package xml reads an XML document into a [richdoc.Document].
//
// # What it is for, and what it is not
//
// ⛔ This renders XML AS DATA: a tree of elements, their attributes and their
// text, laid out so a person can read it. It is not an XML-to-prose converter
// and it does not know what any particular vocabulary means — an ODT body
// belongs to [github.com/go-odf/odf], a DocBook to a DocBook reader. Handed
// one of those, this will faithfully render the markup rather than the
// document, which is the right answer to the question it was asked and the
// wrong answer to the question somebody probably meant.
//
// # Repeated siblings become a table
//
// The same decision as the JSON reader, for the same reason: a sequence of
// sibling elements with the same name and the same child names is a grid, and
// a sequence of nested bullet lists is unreadable. Both the leaf text and the
// attributes become columns.
package xml

import (
	"bytes"
	encodingxml "encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/richdoc"
)

// Options says how to read.
type Options struct {
	// Title, when set, becomes a level-1 heading above the document.
	Title string

	// NoTables keeps repeated siblings as lists.
	NoTables bool

	// MaxDepth, MaxNodes, MaxRows and MaxColumns override the ceilings in
	// [github.com/go-richdoc/data/shape]. Zero means those.
	MaxDepth, MaxNodes, MaxRows, MaxColumns int
}

// node is one element, kept in document order.
type node struct {
	name  string
	attrs []encodingxml.Attr
	text  string
	kids  []*node
}

// Parse reads an XML document.
//
// ⛔ encoding/xml expands no custom entities and fetches no external ones, so a
// billion-laughs document and an <!ENTITY SYSTEM "file:///etc/passwd"> are both
// refused by the decoder rather than by anything here. Measured in both
// directions on a sibling package rather than assumed.
func Parse(src []byte, opt Options) (*richdoc.Document, error) {
	if len(src) > shape.MaxInputBytes {
		return nil, shape.LimitError{What: "bytes", Limit: shape.MaxInputBytes}
	}
	c := newConv(opt)
	root, err := c.read(src)
	if err != nil {
		return nil, err
	}

	blocks, err := c.element(root, 1)
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
	maxDepth, maxNodes, maxRows, maxCols int
	nodes                                int
	// ⛔ noTables was documented in Options and never read. A test asking for
	// it found a table anyway — which is how an option that does nothing
	// announces itself, and it is worse than no option at all, because the
	// documentation makes a promise nothing keeps.
	noTables bool
}

func newConv(opt Options) *conv {
	c := &conv{maxDepth: opt.MaxDepth, maxNodes: opt.MaxNodes,
		maxRows: opt.MaxRows, maxCols: opt.MaxColumns, noTables: opt.NoTables}
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

// read walks the document into a tree, bounding depth and count AS IT GOES.
//
// ⛔ Bounding after the tree is built would mean building it first, which is
// the allocation the ceiling exists to prevent. The depth here is the
// decoder's own nesting, which is where the memory is — a lesson a sibling
// package learnt by putting its guard where its code recursed instead.
func (c *conv) read(src []byte) (*node, error) {
	dec := encodingxml.NewDecoder(bytes.NewReader(src))
	var stack []*node
	var root *node

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading XML: %w", err)
		}
		switch t := tok.(type) {
		case encodingxml.StartElement:
			if len(stack) >= c.maxDepth {
				return nil, shape.LimitError{What: "depth", Limit: c.maxDepth}
			}
			c.nodes++
			if c.nodes > c.maxNodes {
				return nil, shape.LimitError{What: "nodes", Limit: c.maxNodes}
			}
			n := &node{name: t.Name.Local, attrs: t.Attr}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.kids = append(parent.kids, n)
			} else if root == nil {
				root = n
			} else {
				// A second root. XML forbids it and encoding/xml accepts it;
				// keeping it as a child of the first would be a lie about the
				// document's shape.
				return nil, errors.New("the document has more than one root element")
			}
			stack = append(stack, n)
		case encodingxml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case encodingxml.CharData:
			if len(stack) == 0 {
				continue
			}
			s := strings.TrimSpace(string(t))
			if s == "" {
				continue
			}
			cur := stack[len(stack)-1]
			if cur.text == "" {
				cur.text = s
			} else {
				cur.text += " " + s
			}
		}
	}
	if root == nil {
		return nil, errors.New("no elements: an empty document is not a conversion")
	}
	return root, nil
}

// element turns one element into blocks.
// ⛔ There is NO depth check here, and that is deliberate. read already
// refuses past maxDepth as it walks, so a tree that reaches this function is
// already within the ceiling and a second check could never fire. A first
// draft had one; the mutation removing it survived, which is how an
// unreachable guard announces itself. The error return remains because
// building a TABLE can fail on the row and column ceilings.
func (c *conv) element(n *node, depth int) ([]richdoc.Block, error) {
	// A leaf: its name, its attributes and its text, on one line.
	if len(n.kids) == 0 {
		return []richdoc.Block{shape.KeyValue(n.name, shape.Text(leafValue(n)))}, nil
	}

	if cols, ok := c.uniformKids(n); ok && !c.noTables {
		t, err := c.table(n, cols)
		if err != nil {
			return nil, err
		}
		head := richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Code{Value: n.name}}}
		return []richdoc.Block{head, t}, nil
	}

	list := richdoc.List{Tight: true}
	for _, k := range n.kids {
		sub, err := c.element(k, depth+1)
		if err != nil {
			return nil, err
		}
		list.Items = append(list.Items, shape.Item(sub...))
	}
	head := richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Code{Value: n.name}}}
	if n.text != "" {
		head.Inlines = append(head.Inlines, richdoc.Text{Value: ": " + n.text})
	}
	return []richdoc.Block{head, list}, nil
}

// leafValue is a leaf's text, with its attributes after it where it has any.
func leafValue(n *node) string {
	var b strings.Builder
	b.WriteString(n.text)
	for _, a := range n.attrs {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString("@" + a.Name.Local + "=" + a.Value)
	}
	return b.String()
}

// uniformKids says whether every child is the same element with the same leaf
// children, and returns the column names: the attributes first, then the leaf
// child names.
func (c *conv) uniformKids(n *node) ([]string, bool) {
	if len(n.kids) < 2 {
		return nil, false
	}
	name := n.kids[0].name
	cols := columnsOf(n.kids[0])
	if len(cols) == 0 {
		return nil, false
	}
	for _, k := range n.kids {
		if k.name != name {
			return nil, false
		}
		got := columnsOf(k)
		if len(got) != len(cols) {
			return nil, false
		}
		for i := range cols {
			if got[i] != cols[i] {
				return nil, false
			}
		}
	}
	return cols, true
}

// columnsOf is the attribute names and leaf-child names of one element, or nil
// if any child is not a leaf.
func columnsOf(n *node) []string {
	var cols []string
	for _, a := range n.attrs {
		cols = append(cols, "@"+a.Name.Local)
	}
	for _, k := range n.kids {
		if len(k.kids) > 0 {
			return nil
		}
		cols = append(cols, k.name)
	}
	return cols
}

func (c *conv) table(n *node, cols []string) (richdoc.Block, error) {
	if err := shape.Over(len(n.kids), c.maxRows, "rows"); err != nil {
		return nil, err
	}
	if err := shape.Over(len(cols), c.maxCols, "columns"); err != nil {
		return nil, err
	}
	t := richdoc.Table{Align: make([]richdoc.Alignment, len(cols)), Header: shape.Row(cols)}
	for _, k := range n.kids {
		values := map[string]string{}
		for _, a := range k.attrs {
			values["@"+a.Name.Local] = a.Value
		}
		for _, leaf := range k.kids {
			values[leaf.name] = leafValue(leaf)
		}
		row := make([]richdoc.Cell, len(cols))
		for i, col := range cols {
			row[i] = shape.Cell(values[col])
		}
		t.Rows = append(t.Rows, row)
	}
	return t, nil
}
