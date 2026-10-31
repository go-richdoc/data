// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Four readers of untrusted input, one fuzz target each.
//
// ⛔ WHAT THESE ASSERT IS A PROPERTY OF THE OUTPUT, not the absence of a panic.
// "It did not crash" is the weakest thing a fuzz target can check and it is the
// one most targets stop at. Each reader here must also return a document XOR an
// error, build nothing deeper than its own ceiling allows, and give the same
// answer twice for the same bytes — the last one because a document that
// differs between two runs makes every diff of two conversions meaningless, and
// map iteration order in Go is randomised on purpose.
package data_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-richdoc/data/csv"
	"github.com/go-richdoc/data/json"
	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/data/text"
	"github.com/go-richdoc/data/xml"
	"github.com/go-richdoc/richdoc"
)

// check is what every one of the four targets asks.
func check(t *testing.T, format string, src []byte, parse func([]byte) (*richdoc.Document, error)) {
	t.Helper()

	doc, err := parse(src)

	// A document XOR an error.
	if (doc == nil) == (err == nil) {
		t.Fatalf("%s: doc=%v err=%v — one of the two and not both", format, doc != nil, err)
	}
	if err != nil {
		return
	}

	// ⛔ The depth of what was BUILT, against the ceiling that was supposed to
	// bound it. A reader whose guard is in the wrong place returns a document
	// deeper than its own ceiling and no error, which is the failure a panic
	// check cannot see.
	//
	// The slack is for the blocks a reader wraps a level in — a list, an item,
	// a paragraph — which are depth in the document and not depth in the file.
	if d := treeDepth(doc.Blocks); d > 4*shape.MaxDepth+16 {
		t.Fatalf("%s: a document %d blocks deep, past %d", format, d, 4*shape.MaxDepth+16)
	}

	// The same bytes twice.
	again, err2 := parse(src)
	if err2 != nil {
		t.Fatalf("%s: accepted once and refused the second time: %v", format, err2)
	}
	if a, b := fmt.Sprintf("%#v", doc), fmt.Sprintf("%#v", again); a != b {
		t.Fatalf("%s: two runs over the same bytes gave two documents", format)
	}
}

// treeDepth is how far the blocks nest.
func treeDepth(blocks []richdoc.Block) int {
	deepest := 0
	for _, b := range blocks {
		if d := 1 + treeDepth(childBlocks(b)); d > deepest {
			deepest = d
		}
	}
	return deepest
}

// childBlocks is the blocks one block holds, for the two that hold any.
func childBlocks(b richdoc.Block) []richdoc.Block {
	switch t := b.(type) {
	case richdoc.List:
		var out []richdoc.Block
		for _, it := range t.Items {
			out = append(out, it.Blocks...)
		}
		return out
	case richdoc.BlockQuote:
		return t.Blocks
	}
	return nil
}

func FuzzCSV(f *testing.F) {
	f.Add([]byte("a,b\n1,2\n"))
	f.Add([]byte("a;b\n1;2\n"))
	f.Add([]byte("\"a\",\"b\"\n"))
	f.Add([]byte("a\n"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, src []byte) {
		check(t, "csv", src, func(b []byte) (*richdoc.Document, error) {
			return csv.Parse(b, csv.Options{})
		})
	})
}

func FuzzJSON(f *testing.F) {
	f.Add([]byte(`{"a":1}`))
	f.Add([]byte(`[{"a":1},{"a":2}]`))
	f.Add([]byte(`[[[[1]]]]`))
	f.Add([]byte(`"a"`))
	f.Add([]byte(`{"a":{"b":[1,null,true]}}`))
	f.Fuzz(func(t *testing.T, src []byte) {
		check(t, "json", src, func(b []byte) (*richdoc.Document, error) {
			return json.Parse(b, json.Options{})
		})
	})
}

func FuzzXML(f *testing.F) {
	f.Add([]byte(`<r><a>1</a></r>`))
	f.Add([]byte(`<r><a x="1"/><a x="2"/></r>`))
	f.Add([]byte(`<r/>`))
	f.Add([]byte(`<?xml version="1.0"?><r>text</r>`))
	f.Add([]byte(`<!DOCTYPE r><r/>`))
	f.Fuzz(func(t *testing.T, src []byte) {
		check(t, "xml", src, func(b []byte) (*richdoc.Document, error) {
			return xml.Parse(b, xml.Options{})
		})
	})
}

func FuzzText(f *testing.F) {
	f.Add([]byte("one\n\ntwo\n"))
	f.Add([]byte("  indented\n"))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte("no newline"))
	f.Fuzz(func(t *testing.T, src []byte) {
		check(t, "text", src, func(b []byte) (*richdoc.Document, error) {
			return text.Parse(b, text.Options{})
		})
	})
}

// FuzzPrescanAgreesWithTheConverter is the property the pre-scan in
// json/prescan.go has to keep, hunted for rather than argued about.
//
// ⛔ A refusal for "values" must be one the converter would also have made.
// The pre-scan counts over raw bytes and the converter counts over a decoded
// tree, and the proof that the two agree is a derivation in shape.MaxSourceValues
// — a derivation is exactly the kind of claim a fuzzer is good at breaking.
func FuzzPrescanAgreesWithTheConverter(f *testing.F) {
	f.Add([]byte(`[{"a":1},{"a":2}]`))
	f.Add([]byte(`{"a":{"b":{"c":1}}}`))
	f.Add([]byte(`[1,2,3,4,5,6,7,8,9,10]`))
	f.Fuzz(func(t *testing.T, src []byte) {
		// A tiny ceiling, so the question is reachable at fuzz-corpus sizes.
		// ⛔ nodes is the ceiling the derivation pairs with it: the relation in
		// shape.MaxSourceValues is values = 2*nodes + 2, so a source ceiling of
		// 8 is the one belonging to a node ceiling of 3.
		const (
			values = 8
			nodes  = (values - 2) / 2
		)
		_, err := json.Parse(src, json.Options{MaxSourceValues: values})
		var lim shape.LimitError
		if !errors.As(err, &lim) || lim.What != "values" {
			return // not the case under test
		}
		// It was refused before decoding. With the pre-scan out of the way, the
		// converter must refuse it too — for nodes, or for depth, or by failing
		// to decode at all. What it must NOT do is hand back a document.
		doc, err2 := json.Parse(src, json.Options{MaxSourceValues: 1 << 30, MaxNodes: nodes})
		if err2 == nil {
			t.Fatalf("the pre-scan refused %q for values, and the converter accepted it: %v",
				src, doc != nil)
		}
	})
}
