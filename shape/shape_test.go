// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package shape

import (
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

func TestAnEmptyStringIsNoInlinesRatherThanAnEmptyRun(t *testing.T) {
	// ⛔ Writers differ on what they do with a zero-length run: some emit
	// nothing, some emit an empty element a reader then sees as content. An
	// empty cell is a thing a table has a great many of, so the difference is
	// not academic.
	if got := Text(""); got != nil {
		t.Errorf("an empty string gave %#v", got)
	}
	if got := Text("x"); len(got) != 1 {
		t.Fatalf("a string gave %d inlines", len(got))
	}
	if got := Cell("").Inlines; got != nil {
		t.Errorf("an empty cell holds %#v", got)
	}
}

func TestARowKeepsItsOrderAndItsEmptyCells(t *testing.T) {
	r := Row([]string{"a", "", "c"})
	if len(r) != 3 {
		t.Fatalf("%d cells", len(r))
	}
	if r[1].Inlines != nil {
		t.Error("the empty cell in the middle was given content")
	}
	if got := r[2].Inlines[0].(richdoc.Text).Value; got != "c" {
		t.Errorf("the third cell is %q — the row was reordered or shortened", got)
	}
	if got := Row(nil); len(got) != 0 {
		t.Errorf("an empty row gave %d cells", len(got))
	}
}

func TestAHeadingIsAValueAndItsLevelIsClamped(t *testing.T) {
	// ⛔ A VALUE, not a pointer. richdoc's blocks are a closed interface set
	// whose marker method has a value receiver, so *Heading satisfies Block
	// too — it compiles, it type-switches, and it matches no case any consumer
	// wrote. A sibling converter shipped that, and the headings reached the
	// document, the block count was right, and they came out of the PDF as
	// nothing at all.
	var b richdoc.Block = Heading(1, "x")
	if _, ok := b.(richdoc.Heading); !ok {
		t.Fatalf("a heading is a %T", b)
	}

	// Levels outside 1..6 come from a file, not from a person: a JSON tree a
	// hundred deep would otherwise ask for a level-100 heading, which no
	// format has and every writer handles differently.
	for _, c := range []struct{ in, want int }{
		{-5, 1}, {0, 1}, {1, 1}, {3, 3}, {6, 6}, {7, 6}, {1000, 6},
	} {
		if got := Heading(c.in, "x").Level; got != c.want {
			t.Errorf("level %d clamped to %d, want %d", c.in, got, c.want)
		}
	}
}

func TestAKeyIsSetAsCodeRatherThanAsProse(t *testing.T) {
	// ⛔ A key is a literal string out of a file — it may be "0", or "true", or
	// hold a bracket. Setting it in the body face invites it to be read as
	// prose; a monospaced run says "this is what the file says", which is what
	// it is.
	p := KeyValue("total", Text("12"))
	if len(p.Inlines) < 3 {
		t.Fatalf("%d inlines", len(p.Inlines))
	}
	code, ok := p.Inlines[0].(richdoc.Code)
	if !ok {
		t.Fatalf("the key is a %T", p.Inlines[0])
	}
	if code.Value != "total" {
		t.Errorf("the key is %q", code.Value)
	}
	var all strings.Builder
	for _, in := range p.Inlines {
		switch v := in.(type) {
		case richdoc.Text:
			all.WriteString(v.Value)
		case richdoc.Code:
			all.WriteString(v.Value)
		}
	}
	if got := all.String(); got != "total: 12" {
		t.Errorf("the line reads %q", got)
	}
	// A key with no value still reads as a key.
	if got := KeyValue("k", nil); len(got.Inlines) != 2 {
		t.Errorf("a valueless key gave %d inlines", len(got.Inlines))
	}
}

func TestAParagraphHoldsItsString(t *testing.T) {
	if got := Para("hello").Inlines[0].(richdoc.Text).Value; got != "hello" {
		t.Errorf("%q", got)
	}
	if got := Para("").Inlines; got != nil {
		t.Errorf("an empty paragraph holds %#v", got)
	}
}

func TestAnItemWrapsBlocks(t *testing.T) {
	it := Item(Para("a"), Para("b"))
	if len(it.Blocks) != 2 {
		t.Errorf("%d blocks", len(it.Blocks))
	}
	if got := Item(); len(got.Blocks) != 0 {
		t.Errorf("an empty item holds %d blocks", len(got.Blocks))
	}
}

func TestALimitSaysWhichCeilingAndWhatItIs(t *testing.T) {
	// ⛔ The whole value of a refusal is that the person reading it can tell a
	// hostile file from a legitimately enormous one. "too large" cannot.
	err := Over(11, 10, "rows")
	if err == nil {
		t.Fatal("11 passed a ceiling of 10")
	}
	msg := err.Error()
	for _, want := range []string{"rows", "10"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%q does not carry %q", msg, want)
		}
	}
	le, ok := err.(LimitError)
	if !ok {
		t.Fatalf("a %T, which a caller cannot match on", err)
	}
	if le.What != "rows" || le.Limit != 10 {
		t.Errorf("%+v", le)
	}
	// Exactly at the ceiling is allowed — the side a limit gets wrong.
	if err := Over(10, 10, "rows"); err != nil {
		t.Errorf("exactly at the ceiling was refused: %v", err)
	}
	if err := Over(0, 10, "rows"); err != nil {
		t.Errorf("nothing was refused: %v", err)
	}
}

func TestTheCeilingsAreTheNumbersTheDocumentationQuotes(t *testing.T) {
	// ⛔ A change detector, deliberately. These numbers are quoted in this
	// package's doc comments and in the README of every reader that uses them,
	// so moving one silently makes several statements false at once — and the
	// only one anybody notices is the one that stops a legitimate file.
	//
	// Written out rather than derived, so the judge cannot move with the
	// subject.
	for _, c := range []struct {
		name string
		got  int
		want int
	}{
		{"MaxInputBytes", MaxInputBytes, 64 << 20},
		{"MaxRows", MaxRows, 100_000},
		{"MaxColumns", MaxColumns, 1_000},
		{"MaxDepth", MaxDepth, 100},
		{"MaxNodes", MaxNodes, 500_000},
	} {
		if c.got != c.want {
			t.Errorf("%s is %d, not %d. If that was meant, the doc comments and the "+
				"READMEs quoting it have to change too.", c.name, c.got, c.want)
		}
	}
}
