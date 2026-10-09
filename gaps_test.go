// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package data's own tests: the paths each reader has that nothing else
// reaches, gathered here because they are the same handful of questions asked
// of four readers.
package data_test

import (
	"strings"
	"testing"

	"github.com/go-richdoc/data/csv"
	"github.com/go-richdoc/data/json"
	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/data/text"
	"github.com/go-richdoc/data/xml"
	"github.com/go-richdoc/richdoc"
)

func TestADelimiterThatIsNotOneIsRefused(t *testing.T) {
	// ⛔ encoding/csv refuses a quote, a carriage return, a newline or an
	// invalid rune as a separator — and it refuses them at READ time, not at
	// construction, so the error arrives from inside the loop. It has to carry
	// the row number, because over a directory of exports "invalid field or
	// comment delimiter" alone says nothing about which file or where.
	for _, d := range []rune{'"', '\n', '\r'} {
		_, err := csv.Parse([]byte("a,b\n1,2\n"), csv.Options{Delimiter: d})
		if err == nil {
			t.Errorf("%q was accepted as a delimiter", d)
			continue
		}
		if !strings.Contains(err.Error(), "row") {
			t.Errorf("%q was refused as %q, which does not say where", d, err)
		}
	}
}

func TestACommaDecimalCountsAsANumber(t *testing.T) {
	// ⛔ The detector's second reading of a number. A semicolon-separated file
	// written anywhere the comma is a decimal point has 1,5 in its first data
	// row — and if that does not read as a number, the header above it is not
	// promoted and the document gains a row of column names as data.
	d, err := csv.Parse([]byte("poids;taille\n1,5;2,75\n"), csv.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tb, ok := firstTable(d.Blocks)
	if !ok {
		t.Fatal("no table")
	}
	if tb.Header == nil {
		t.Error("a row of comma decimals did not read as numbers, so the header was lost")
	}
}

// firstTable is the first table anywhere in a document, however deeply a list
// has wrapped it.
func firstTable(bs []richdoc.Block) (richdoc.Table, bool) {
	for _, b := range bs {
		switch v := b.(type) {
		case richdoc.Table:
			return v, true
		case richdoc.List:
			for _, it := range v.Items {
				if tb, ok := firstTable(it.Blocks); ok {
					return tb, true
				}
			}
		}
	}
	return richdoc.Table{}, false
}

func TestAFileThatDoesNotEndWithANewlineKeepsItsLastParagraph(t *testing.T) {
	// ⛔ The last group is only flushed after the loop. A file whose final line
	// has no newline — which is most files written by a program — would
	// otherwise lose its last paragraph entirely, in silence.
	d, err := text.Parse([]byte("first\n\nlast line with no newline"), text.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Blocks) != 2 {
		t.Fatalf("%d blocks: the last paragraph was dropped", len(d.Blocks))
	}
}

func TestJSONStopsCountingAtTheNodeCeiling(t *testing.T) {
	// Three different places count nodes — scalars inside an object, cells
	// inside a table, and values on the way down — and each has to stop.
	for _, src := range []string{
		`{"a":1,"b":2,"c":3,"d":4,"e":5}`,           // object scalars
		`[{"a":1},{"a":2},{"a":3},{"a":4},{"a":5}]`, // table cells
		`{"a":{"b":{"c":{"d":{"e":1}}}}}`,           // values descending
		`[[1],[2],[3],[4],[5]]`,                     // arrays descending
	} {
		if _, err := json.Parse([]byte(src), json.Options{MaxNodes: 3}); err == nil {
			t.Errorf("%s passed a ceiling of three nodes", src)
		} else if le, ok := err.(shape.LimitError); !ok || le.What != "nodes" {
			t.Errorf("%s was refused as %v", src, err)
		}
	}
}

func TestXMLStopsAtTheCeilingWhereverItIsReached(t *testing.T) {
	// The reader bounds depth as it walks; the renderer bounds it again on the
	// way back out, because a tree that was read is not yet a document.
	deep := strings.Repeat("<a>", 30) + "x" + strings.Repeat("</a>", 30)
	if _, err := xml.Parse([]byte(deep), xml.Options{MaxDepth: 5}); err == nil {
		t.Error("a deep document was accepted")
	}

	// A row ceiling reached while BUILDING the table, after the tree is read.
	var many strings.Builder
	many.WriteString("<r>")
	for i := 0; i < 10; i++ {
		many.WriteString("<a><x>1</x></a>")
	}
	many.WriteString("</r>")
	if _, err := xml.Parse([]byte(many.String()), xml.Options{MaxRows: 3}); err == nil {
		t.Error("a table past the row ceiling was built")
	}
	// And a column ceiling.
	wide := "<r><a>" + strings.Repeat("<c>1</c>", 8) + "</a><a>" +
		strings.Repeat("<c>2</c>", 8) + "</a></r>"
	if _, err := xml.Parse([]byte(wide), xml.Options{MaxColumns: 3}); err == nil {
		t.Error("a table past the column ceiling was built")
	}
}

func TestXMLSiblingsWithTheSameColumnCountButDifferentNamesStayAList(t *testing.T) {
	// ⛔ Same NAME, same COUNT, different column names. Comparing only the
	// counts would build a table whose second row holds the wrong values under
	// the wrong headings — a document that is confidently, silently wrong.
	src := `<r><a><x>1</x><y>2</y></a><a><p>3</p><q>4</q></a></r>`
	d, err := xml.Parse([]byte(src), xml.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := firstTable(d.Blocks); ok {
		t.Error("siblings with different column names were put in one table")
	}
}
