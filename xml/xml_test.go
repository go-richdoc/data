// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package xml

import (
	"strings"
	"testing"

	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/richdoc"
)

func flat(bs []richdoc.Block) string {
	var b strings.Builder
	var blocks func([]richdoc.Block)
	var inlines func([]richdoc.Inline)
	inlines = func(is []richdoc.Inline) {
		for _, i := range is {
			switch t := i.(type) {
			case richdoc.Text:
				b.WriteString(t.Value)
			case richdoc.Code:
				b.WriteString(t.Value)
			}
		}
	}
	blocks = func(bs []richdoc.Block) {
		for _, blk := range bs {
			switch t := blk.(type) {
			case richdoc.Paragraph:
				inlines(t.Inlines)
				b.WriteString("\n")
			case richdoc.Heading:
				inlines(t.Inlines)
				b.WriteString("\n")
			case richdoc.List:
				for _, it := range t.Items {
					blocks(it.Blocks)
				}
			case richdoc.Table:
				for _, c := range t.Header {
					inlines(c.Inlines)
					b.WriteString("|")
				}
				b.WriteString("\n")
				for _, r := range t.Rows {
					for _, c := range r {
						inlines(c.Inlines)
						b.WriteString("|")
					}
					b.WriteString("\n")
				}
			}
		}
	}
	blocks(bs)
	return b.String()
}

func tableOf(d *richdoc.Document) (richdoc.Table, bool) {
	var find func([]richdoc.Block) (richdoc.Table, bool)
	find = func(bs []richdoc.Block) (richdoc.Table, bool) {
		for _, b := range bs {
			switch v := b.(type) {
			case richdoc.Table:
				return v, true
			case richdoc.List:
				for _, it := range v.Items {
					if tb, ok := find(it.Blocks); ok {
						return tb, true
					}
				}
			}
		}
		return richdoc.Table{}, false
	}
	return find(d.Blocks)
}

func TestRepeatedSiblingsBecomeATable(t *testing.T) {
	src := `<catalogue>
	  <book isbn="1"><title>One</title><year>1990</year></book>
	  <book isbn="2"><title>Two</title><year>1991</year></book>
	</catalogue>`
	d, err := Parse([]byte(src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	tb, ok := tableOf(d)
	if !ok {
		t.Fatalf("no table:\n%s", flat(d.Blocks))
	}
	if len(tb.Rows) != 2 {
		t.Errorf("%d rows", len(tb.Rows))
	}
	// ⛔ The attribute is a COLUMN. An isbn is the identifying fact of a book,
	// and a reader that kept only the child elements would produce a table of
	// titles and years with nothing to join them on.
	var header strings.Builder
	for _, c := range tb.Header {
		for _, in := range c.Inlines {
			if tx, ok := in.(richdoc.Text); ok {
				header.WriteString(tx.Value + " ")
			}
		}
	}
	if !strings.Contains(header.String(), "@isbn") {
		t.Errorf("the attribute is not a column: %q", header.String())
	}
}

func TestSiblingsThatDisagreeStayAList(t *testing.T) {
	for _, src := range []string{
		`<r><a><x>1</x></a><b><x>2</x></b></r>`,             // different names
		`<r><a><x>1</x></a><a><y>2</y></a></r>`,             // different children
		`<r><a><x><deep/></x></a><a><x><deep/></x></a></r>`, // a child that is not a leaf
		`<r><a/></r>`, // only one
	} {
		d, err := Parse([]byte(src), Options{})
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if _, ok := tableOf(d); ok {
			t.Errorf("%s became a table:\n%s", src, flat(d.Blocks))
		}
	}
}

func TestTextAndAttributesOfALeafBothSurvive(t *testing.T) {
	d, err := Parse([]byte(`<r><item kind="x">value</item><other/></r>`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := flat(d.Blocks)
	for _, want := range []string{"value", "@kind=x"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is missing from:\n%s", want, got)
		}
	}
}

func TestBillionLaughsIsRefusedByTheDecoder(t *testing.T) {
	// ⛔ Measured rather than assumed: encoding/xml expands no custom
	// entities, so this is refused before anything here is reached. The test
	// exists so that a future Go which started expanding them turns it red
	// rather than this package quietly gaining a hole.
	src := `<?xml version="1.0"?>
<!DOCTYPE lolz [
 <!ENTITY l0 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa">
 <!ENTITY l1 "&l0;&l0;&l0;&l0;&l0;&l0;&l0;&l0;&l0;&l0;">
 <!ENTITY l2 "&l1;&l1;&l1;&l1;&l1;&l1;&l1;&l1;&l1;&l1;">
 <!ENTITY l3 "&l2;&l2;&l2;&l2;&l2;&l2;&l2;&l2;&l2;&l2;">
 <!ENTITY l4 "&l3;&l3;&l3;&l3;&l3;&l3;&l3;&l3;&l3;&l3;">
]>
<r>&l4;</r>`
	if _, err := Parse([]byte(src), Options{}); err == nil {
		t.Error("a document whose entities expand a million-fold was accepted")
	}
}

func TestAnExternalEntityIsNotFetched(t *testing.T) {
	src := `<?xml version="1.0"?>
<!DOCTYPE r [ <!ENTITY x SYSTEM "file:///etc/passwd"> ]>
<r>&x;</r>`
	d, err := Parse([]byte(src), Options{})
	if err == nil && strings.Contains(flat(d.Blocks), "root:") {
		t.Fatal("⛔ THE FILE'S CONTENTS CAME BACK IN THE DOCUMENT")
	}
}

func TestTheCeilingsAreEnforced(t *testing.T) {
	deep := strings.Repeat("<a>", 300) + strings.Repeat("</a>", 300)
	if _, err := Parse([]byte(deep), Options{MaxDepth: 10}); err == nil {
		t.Error("a deeply nested document was accepted")
	} else if le, ok := err.(shape.LimitError); !ok || le.What != "depth" {
		t.Errorf("refused as %v", err)
	}

	var many strings.Builder
	many.WriteString("<r>")
	for i := 0; i < 60; i++ {
		many.WriteString("<a><x>1</x></a>")
	}
	many.WriteString("</r>")
	if _, err := Parse([]byte(many.String()), Options{MaxRows: 5}); err == nil {
		t.Error("a document past the row ceiling became a table")
	}
	if _, err := Parse([]byte(many.String()), Options{MaxNodes: 4}); err == nil {
		t.Error("a document past the node ceiling was built")
	}
	if _, err := Parse(make([]byte, shape.MaxInputBytes+1), Options{}); err == nil {
		t.Error("a file past the input ceiling was read")
	}
}

func TestAnEmptyOrMalformedDocumentIsRefused(t *testing.T) {
	// ⛔ "No elements" and "an empty document" are different answers, and a
	// caller handed the second has no way to tell which happened.
	for _, src := range []string{"", "   ", "<r>", "not xml at all <<<"} {
		if _, err := Parse([]byte(src), Options{}); err == nil {
			t.Errorf("%q was accepted", src)
		}
	}
}

func TestASecondRootIsRefusedRatherThanAdopted(t *testing.T) {
	// ⛔ XML forbids it, encoding/xml accepts it, and keeping the second as a
	// child of the first would be a lie about the document's shape.
	if _, err := Parse([]byte("<a/><b/>"), Options{}); err == nil {
		t.Error("a document with two roots was accepted")
	}
}

func TestATitleBecomesAHeading(t *testing.T) {
	d, err := Parse([]byte("<r><a/></r>"), Options{Title: "Catalogue"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Blocks[0].(richdoc.Heading); !ok {
		t.Fatalf("first block is %T", d.Blocks[0])
	}
}

func TestTablesCanBeTurnedOff(t *testing.T) {
	src := `<r><a><x>1</x></a><a><x>2</x></a></r>`
	d, err := Parse([]byte(src), Options{NoTables: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tableOf(d); ok {
		t.Error("NoTables still produced a table")
	}
}

func TestMixedContentKeepsItsText(t *testing.T) {
	// An element with both text and children: the text must not vanish
	// because the children got the attention.
	d, err := Parse([]byte("<r>leading<a/>trailing</r>"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := flat(d.Blocks)
	if !strings.Contains(got, "leading") || !strings.Contains(got, "trailing") {
		t.Errorf("mixed content lost its text:\n%s", got)
	}
}

func TestAReaderIsAccepted(t *testing.T) {
	if _, err := ParseReader(strings.NewReader("<r><a/></r>"), Options{}); err != nil {
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
