// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package csv

import (
	"strings"
	"testing"

	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/richdoc"
)

// table is the one table a document is expected to hold.
func table(t *testing.T, d *richdoc.Document) richdoc.Table {
	t.Helper()
	for _, b := range d.Blocks {
		if tb, ok := b.(richdoc.Table); ok {
			return tb
		}
	}
	t.Fatalf("no table in %d blocks", len(d.Blocks))
	return richdoc.Table{}
}

// cells is a row as plain strings.
func cells(cs []richdoc.Cell) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		var b strings.Builder
		for _, in := range c.Inlines {
			if t, ok := in.(richdoc.Text); ok {
				b.WriteString(t.Value)
			}
		}
		out[i] = b.String()
	}
	return out
}

func TestASemicolonFileIsNotOneColumn(t *testing.T) {
	// ⛔ What the sniffer exists for. A spreadsheet anywhere the comma is a
	// decimal point writes semicolons, and a reader that assumes a comma turns
	// every row into a single column — with no error, so the document looks
	// like a one-column table of long strings and nobody questions it.
	d, err := Parse([]byte("produit;quantité;prix\npain;3;2,50\nvin;1;12,00\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	tb := table(t, d)
	if got := cells(tb.Header); len(got) != 3 {
		t.Fatalf("header came back as %v", got)
	}
	if got := cells(tb.Rows[0]); got[2] != "2,50" {
		t.Errorf("the comma decimal was split: %v", got)
	}
}

func TestACommaInsideQuotesIsNotADelimiter(t *testing.T) {
	// ⛔ The sniffer counts OUTSIDE quotes. A two-column file whose second
	// column is prose full of commas would otherwise sniff as a six-column
	// file, and every row would be ragged.
	src := `name,note
ada,"first, and also second, in a sense"
alan,"short"
`
	if got := Sniff([]byte(src)); got != ',' {
		t.Errorf("sniffed %q", got)
	}
	// Header: Always, because the detector declines a file with no numbers in
	// it — that is its documented timidity, and this test is about the
	// sniffer, not about the detector.
	d, err := Parse([]byte(src), Options{Header: Always})
	if err != nil {
		t.Fatal(err)
	}
	tb := table(t, d)
	if n := len(tb.Header); n != 2 {
		t.Fatalf("%d columns: %v", n, cells(tb.Header))
	}
	if got := cells(tb.Rows[0])[1]; got != "first, and also second, in a sense" {
		t.Errorf("the quoted field came back as %q", got)
	}
}

func TestProseIsNotMistakenForACSV(t *testing.T) {
	// ⛔ Steadiness is the signal, not frequency. English prose is full of
	// commas, and "the most common character" picks the comma every time —
	// but an unsteady count is the giveaway, so the file falls back to the
	// default rather than being shredded into columns.
	src := "Once, long ago, there was a reader.\nIt sniffed, and it guessed.\nIt was wrong.\n"
	d, err := Parse([]byte(src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	tb := table(t, d)
	if tb.Header != nil {
		t.Errorf("prose was given a header row: %v", cells(tb.Header))
	}
}

func TestSteadinessBeatsFrequency(t *testing.T) {
	// ⛔ The two readings diverge here, and only here. Both delimiters appear
	// once on the first line, so a sniffer that only counted would take the
	// first it tried — the comma — and be wrong. The semicolon is the one
	// whose count is the SAME on every line, which is what a column separator
	// looks like and what a comma inside prose never does.
	//
	// A mutation removing the steadiness bonus survived every other test in
	// this file, because in all of them frequency and steadiness happened to
	// agree.
	// ⛔ The comma has to appear on the FIRST line, or the sniffer drops it
	// before steadiness is ever consulted — which is why an earlier version of
	// this fixture passed with and without the bonus and proved nothing.
	src := []byte("nom;note, détaillée\nada;bonne, et longue\nalan;courte, nette, sèche\n")
	if got := Sniff(src); got != ';' {
		t.Errorf("sniffed %q: the comma is as frequent on the first line and ragged "+
			"after it, which is exactly what steadiness is for", got)
	}
	d, err := Parse(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(table(t, d).Rows[0]); n != 2 {
		t.Errorf("%d columns — the notes were split on their commas", n)
	}
}

func TestQuotedTextDoesNotMakeADelimiterLookSteady(t *testing.T) {
	// ⛔ The other half of the same decision. Counting inside quotes makes the
	// real delimiter look RAGGED — a quoted field holding commas moves its
	// count from line to line — and hands the choice to whatever else happens
	// to be regular. Here that is the semicolon, which is inside the data.
	src := []byte(`a,b;c
"x,y",z;w
"p,q",r;v
`)
	if got := Sniff(src); got != ',' {
		t.Errorf("sniffed %q: the commas inside the quoted fields were counted, so the "+
			"comma looked ragged and the semicolon won", got)
	}
}

func TestAHeaderIsPromotedOnlyOnEvidence(t *testing.T) {
	// ⛔ Deliberately timid. Promoting a row of data to a header LOSES it and
	// a reader cannot get it back; leaving a header as data is ugly and
	// recoverable. So the default promotes only when the first row is all text
	// and the second holds a number.
	for _, c := range []struct {
		name string
		src  string
		want bool
	}{
		{"text then numbers", "a,b\n1,2\n", true},
		{"numbers throughout", "1,2\n3,4\n", false},
		{"text throughout", "a,b\nc,d\n", false},
		{"an empty first cell", ",b\n1,2\n", false},
		{"one row only", "a,b\n", false},
	} {
		d, err := Parse([]byte(c.src), Options{})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := table(t, d).Header != nil
		if got != c.want {
			t.Errorf("%s: header=%v, want %v", c.name, got, c.want)
		}
	}
}

func TestTheHeaderCanBeForcedEitherWay(t *testing.T) {
	src := []byte("1,2\n3,4\n")
	d, err := Parse(src, Options{Header: Always})
	if err != nil {
		t.Fatal(err)
	}
	if tb := table(t, d); tb.Header == nil || len(tb.Rows) != 1 {
		t.Errorf("Always gave header=%v, %d rows", tb.Header != nil, len(tb.Rows))
	}
	d, err = Parse([]byte("a,b\n1,2\n"), Options{Header: Never})
	if err != nil {
		t.Fatal(err)
	}
	if tb := table(t, d); tb.Header != nil || len(tb.Rows) != 2 {
		t.Errorf("Never gave header=%v, %d rows", tb.Header != nil, len(tb.Rows))
	}
}

func TestARaggedFileIsSquaredOffRatherThanRefused(t *testing.T) {
	// ⛔ Exports are ragged all the time — a trailing comma, a short last row.
	// Refusing the whole document over one row is a worse answer than a table
	// with a gap in it, and every consumer of richdoc expects a rectangle.
	d, err := Parse([]byte("a,b,c\n1,2\n3,4,5,6\n"), Options{Header: Never})
	if err != nil {
		t.Fatalf("a ragged file was refused: %v", err)
	}
	tb := table(t, d)
	for i, r := range tb.Rows {
		if len(r) != 4 {
			t.Errorf("row %d has %d cells: %v", i, len(r), cells(r))
		}
	}
}

func TestAnEmptyFileIsRefusedRatherThanReturnedAsAnEmptyTable(t *testing.T) {
	// ⛔ An empty document and "this file held no rows" are different answers,
	// and a caller handed the first has no way to tell which happened.
	if _, err := Parse(nil, Options{}); err == nil {
		t.Error("an empty file came back as a document")
	}
	if _, err := Parse([]byte("\n\n  \n"), Options{}); err == nil {
		t.Error("a file of blank lines came back as a document")
	}
}

func TestTheCeilingsAreTheOnesSaidOutLoud(t *testing.T) {
	wide := strings.Repeat("x,", 20) + "x\n"
	if _, err := Parse([]byte(wide), Options{MaxColumns: 5}); err == nil {
		t.Error("a row past the column ceiling was accepted")
	} else if le, ok := err.(shape.LimitError); !ok || le.What != "columns" {
		t.Errorf("refused as %v", err)
	}

	var tall strings.Builder
	for i := 0; i < 20; i++ {
		tall.WriteString("a,b\n")
	}
	if _, err := Parse([]byte(tall.String()), Options{MaxRows: 5, Header: Never}); err == nil {
		t.Error("a file past the row ceiling was accepted")
	}
	if _, err := Parse(make([]byte, shape.MaxInputBytes+1), Options{}); err == nil {
		t.Error("a file past the input ceiling was read")
	}
}

func TestTabAndPipeAreSniffedToo(t *testing.T) {
	if got := Sniff([]byte("a\tb\tc\n1\t2\t3\n")); got != '\t' {
		t.Errorf("tab sniffed as %q", got)
	}
	if got := Sniff([]byte("a|b|c\n1|2|3\n")); got != '|' {
		t.Errorf("pipe sniffed as %q", got)
	}
	if got := Sniff(nil); got != ',' {
		t.Errorf("an empty file sniffed as %q, and the default is a comma", got)
	}
	if got := Sniff([]byte("nodelimitershere\n")); got != ',' {
		t.Errorf("a file with no delimiter sniffed as %q", got)
	}
}

func TestAnExplicitDelimiterIsNotSniffedOver(t *testing.T) {
	// The file looks like semicolons; the caller says commas and means it.
	d, err := Parse([]byte("a;b\nc;d\n"), Options{Delimiter: ',', Header: Never})
	if err != nil {
		t.Fatal(err)
	}
	if got := cells(table(t, d).Rows[0]); len(got) != 1 || got[0] != "a;b" {
		t.Errorf("an explicit delimiter was overridden: %v", got)
	}
}

func TestACaptionTravels(t *testing.T) {
	d, err := Parse([]byte("a,b\n1,2\n"), Options{Caption: "Sales"})
	if err != nil {
		t.Fatal(err)
	}
	tb := table(t, d)
	if len(tb.Caption) == 0 {
		t.Fatal("the caption was dropped")
	}
	if got := tb.Caption[0].(richdoc.Text).Value; got != "Sales" {
		t.Errorf("caption %q", got)
	}
}

func TestInvalidUTF8DoesNotStopTheSniffer(t *testing.T) {
	// A CSV may be in any encoding, and the delimiters are all ASCII.
	src := []byte("a;b\n\xff\xfe;d\n")
	if got := Sniff(src); got != ';' {
		t.Errorf("Latin-1 bytes stopped the sniffer: %q", got)
	}
}

func TestAReaderIsAccepted(t *testing.T) {
	d, err := ParseReader(strings.NewReader("a,b\n1,2\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if table(t, d).Header == nil {
		t.Error("the header was lost through a reader")
	}
	if _, err := ParseReader(failingReader{}, Options{}); err == nil {
		t.Error("a reader that refuses everything was accepted")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errFail }

var errFail = &myErr{}

type myErr struct{}

func (*myErr) Error() string { return "no" }

func TestAnUnreadableRowNamesItsNumber(t *testing.T) {
	// encoding/csv is lenient here by configuration, so what remains is the
	// shape it will not read at all; the message has to say WHICH row.
	_, err := Parse([]byte("a,b\n\"unterminated\n"), Options{Header: Never, Delimiter: ','})
	if err == nil {
		t.Skip("encoding/csv accepted it under LazyQuotes, which is its own answer")
	}
	if !strings.Contains(err.Error(), "row") {
		t.Errorf("the refusal is %q and does not say which row", err)
	}
}
