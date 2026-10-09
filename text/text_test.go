// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package text

import (
	"strings"
	"testing"

	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/richdoc"
)

func kinds(bs []richdoc.Block) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		switch b.(type) {
		case richdoc.Paragraph:
			out[i] = "p"
		case richdoc.CodeBlock:
			out[i] = "code"
		case richdoc.Heading:
			out[i] = "h"
		default:
			out[i] = "?"
		}
	}
	return out
}

func first(bs []richdoc.Block, kind string) string {
	for _, b := range bs {
		switch t := b.(type) {
		case richdoc.Paragraph:
			if kind != "p" {
				continue
			}
			var s strings.Builder
			for _, in := range t.Inlines {
				if tx, ok := in.(richdoc.Text); ok {
					s.WriteString(tx.Value)
				}
			}
			return s.String()
		case richdoc.CodeBlock:
			if kind == "code" {
				return t.Text
			}
		}
	}
	return ""
}

func TestABlankLineEndsAParagraph(t *testing.T) {
	d, err := Parse([]byte("one\ntwo\n\nthree\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(d.Blocks); strings.Join(got, ",") != "p,p" {
		t.Fatalf("blocks came back as %v", got)
	}
	// Wrapped lines are flowed: that is what a paragraph is.
	if got := first(d.Blocks, "p"); got != "one two" {
		t.Errorf("the first paragraph is %q", got)
	}
}

func TestTextLaidOutOnPurposeIsKept(t *testing.T) {
	// ⛔ What this reader exists to get right. A table drawn with spaces, a
	// log, a column of figures — every naive reader flows them into a
	// paragraph and the file becomes nonsense. Nothing errors, and the
	// document looks fine until somebody who knows the data reads it.
	src := "A sentence first.\n\n" +
		"  name      count\n" +
		"  apples        3\n" +
		"  pears        12\n"
	d, err := Parse([]byte(src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(d.Blocks); strings.Join(got, ",") != "p,code" {
		t.Fatalf("blocks came back as %v", got)
	}
	code := first(d.Blocks, "code")
	if !strings.Contains(code, "  apples        3") {
		t.Errorf("the alignment was lost:\n%s", code)
	}
}

func TestAnIndentedRunIsKeptEvenWithNoInnerSpacing(t *testing.T) {
	// ⛔ The indent signal ALONE. Every earlier fixture was both indented and
	// internally spaced, so either test alone satisfied them and a mutation
	// removing the indent check survived. Here the lines are single words:
	// only the left margin says they were placed.
	src := "Before.\n\n    alpha\n    beta\n    gamma\n"
	d, err := Parse([]byte(src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(d.Blocks); strings.Join(got, ",") != "p,code" {
		t.Fatalf("blocks came back as %v — an indented run was flowed into a paragraph", got)
	}
}

func TestColumnsHeldApartBySpacesAreKeptEvenWithNoIndent(t *testing.T) {
	// ⛔ The second signal, and it had no witness: every other fixture here is
	// INDENTED, so the leading-space test caught them all and a mutation
	// removing the inner-spaces test survived. A table written flush to the
	// left margin is still a table.
	src := "Before.\n\n" +
		"name      count\n" +
		"apples        3\n" +
		"pears        12\n"
	d, err := Parse([]byte(src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(d.Blocks); strings.Join(got, ",") != "p,code" {
		t.Fatalf("blocks came back as %v — the columns were flowed into a paragraph", got)
	}
	if code := first(d.Blocks, "code"); !strings.Contains(code, "apples        3") {
		t.Errorf("the alignment was lost:\n%s", code)
	}
}

func TestOneIndentedLineIsNotAListing(t *testing.T) {
	// ⛔ The other direction, and the one over-eager detection gets wrong: a
	// single indented line in flowing prose is a wrapped sentence, not a
	// listing. Both signals are about the paragraph as a WHOLE.
	src := "This sentence carries on\n  and wraps once, indented.\n"
	d, err := Parse([]byte(src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(d.Blocks); strings.Join(got, ",") != "p" {
		t.Errorf("a wrapped sentence came back as %v", got)
	}
}

func TestASingleLineIsNeverALayout(t *testing.T) {
	d, err := Parse([]byte("  just  one  indented  line\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(d.Blocks); strings.Join(got, ",") != "p" {
		t.Errorf("one line came back as %v", got)
	}
}

func TestMarkdownIsNotGuessedAt(t *testing.T) {
	// ⛔ A line beginning with a dash, or an underlined line, are MARKDOWN's
	// rules. Applying them here would silently turn a file that meant none of
	// it into a document with headings and lists that are not in it — and a
	// file that did mean them should be read by the Markdown reader.
	src := "Title\n=====\n\n- not a list\n- also not\n"
	d, err := Parse([]byte(src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range d.Blocks {
		if _, ok := b.(richdoc.Heading); ok {
			t.Error("an underlined line became a heading")
		}
		if _, ok := b.(richdoc.List); ok {
			t.Error("a dashed line became a list")
		}
	}
}

func TestVerbatimKeepsEverythingExactly(t *testing.T) {
	src := "one\n\n   two\n\n\nthree\n"
	d, err := Parse([]byte(src), Options{Verbatim: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Blocks) != 1 {
		t.Fatalf("%d blocks", len(d.Blocks))
	}
	cb, ok := d.Blocks[0].(richdoc.CodeBlock)
	if !ok {
		t.Fatalf("block is %T", d.Blocks[0])
	}
	if cb.Text != src {
		t.Errorf("verbatim changed the text:\n%q\nvs\n%q", cb.Text, src)
	}
}

func TestLineEndingsAreNormalised(t *testing.T) {
	// A file from Windows must not come back with a stray carriage return in
	// every paragraph.
	d, err := Parse([]byte("one\r\ntwo\r\n\r\nthree\r\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := first(d.Blocks, "p"); strings.ContainsRune(got, '\r') {
		t.Errorf("a carriage return survived: %q", got)
	}
	if got := kinds(d.Blocks); strings.Join(got, ",") != "p,p" {
		t.Errorf("CRLF paragraphs came back as %v", got)
	}
}

func TestInvalidUTF8IsRepairedRatherThanRefused(t *testing.T) {
	// ⛔ Half the world's .txt is Latin-1. Refusing it would be correct and
	// useless; a run of replacement characters is visible, which is what a
	// person needs in order to notice and re-save the file.
	d, err := Parse([]byte("caf\xe9 au lait\n"), Options{})
	if err != nil {
		t.Fatalf("a Latin-1 file was refused: %v", err)
	}
	got := first(d.Blocks, "p")
	if !strings.Contains(got, "�") {
		t.Errorf("the bad byte vanished silently: %q", got)
	}
	if !strings.Contains(got, "au lait") {
		t.Errorf("the rest of the line was lost: %q", got)
	}
}

func TestAnEmptyFileIsAnEmptyDocument(t *testing.T) {
	// ⛔ Unlike the other three readers, an empty TEXT file is a legitimate
	// answer rather than a failure: a file of no bytes genuinely holds no
	// paragraphs, and there is nothing a caller could have meant instead.
	d, err := Parse(nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Blocks) != 0 {
		t.Errorf("%d blocks from nothing", len(d.Blocks))
	}
}

func TestTheCeilingsAreEnforced(t *testing.T) {
	var many strings.Builder
	for i := 0; i < 50; i++ {
		many.WriteString("para\n\n")
	}
	if _, err := Parse([]byte(many.String()), Options{MaxBlocks: 5}); err == nil {
		t.Error("a file past the block ceiling was accepted")
	} else if le, ok := err.(shape.LimitError); !ok || le.What != "blocks" {
		t.Errorf("refused as %v", err)
	}
	if _, err := Parse(make([]byte, shape.MaxInputBytes+1), Options{}); err == nil {
		t.Error("a file past the input ceiling was read")
	}
}

func TestATitleBecomesAHeading(t *testing.T) {
	d, err := Parse([]byte("body\n"), Options{Title: "Notes"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Blocks[0].(richdoc.Heading); !ok {
		t.Fatalf("first block is %T", d.Blocks[0])
	}
	// And with Verbatim, so the two options compose rather than one winning.
	d, err = Parse([]byte("body\n"), Options{Title: "Notes", Verbatim: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(d.Blocks); strings.Join(got, ",") != "h,code" {
		t.Errorf("title+verbatim came back as %v", got)
	}
}

func TestAReaderIsAccepted(t *testing.T) {
	if _, err := ParseReader(strings.NewReader("hello\n"), Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseReader(failingReader{}, Options{}); err == nil {
		t.Error("a reader that refuses everything was accepted")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errFail{} }

type errFail struct{}

func (errFail) Error() string { return "no" }
