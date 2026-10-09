// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package json

import (
	"strings"
	"testing"

	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/richdoc"
)

// flat renders a document as plain text, so a test can say what came out
// without walking a tree by hand.
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
			case richdoc.Emph:
				inlines(t.Inlines)
			case richdoc.Strong:
				inlines(t.Inlines)
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

func tableOf(t *testing.T, d *richdoc.Document) (richdoc.Table, bool) {
	t.Helper()
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

func TestAnArrayOfUniformObjectsBecomesATable(t *testing.T) {
	// ⛔ The decision this package is for. Almost every JSON export is this
	// shape — an API page, a database dump, a log — and rendering it as
	// hundreds of nested bullet lists produces something nobody can read, out
	// of data that was a grid all along.
	d, err := Parse([]byte(`[{"id":1,"name":"ada"},{"id":2,"name":"alan"}]`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	tb, ok := tableOf(t, d)
	if !ok {
		t.Fatalf("no table:\n%s", flat(d.Blocks))
	}
	if len(tb.Header) != 2 || len(tb.Rows) != 2 {
		t.Errorf("%d columns, %d rows", len(tb.Header), len(tb.Rows))
	}
}

func TestAnArrayThatIsNotUniformStaysAList(t *testing.T) {
	// ⛔ Strict on purpose. A table built from "most of them agree" silently
	// drops whatever the odd element held — and the element that disagrees is
	// exactly the interesting one.
	for _, src := range []string{
		`[{"a":1},{"b":2}]`,                     // different keys
		`[{"a":1},{"a":1,"b":2}]`,               // different counts
		`[{"a":1},"not an object"]`,             // not all objects
		`[{"a":{"nested":true}},{"a":{"n":1}}]`, // a nested value
		`[{},{}]`,                               // no keys at all
	} {
		d, err := Parse([]byte(src), Options{})
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if _, ok := tableOf(t, d); ok {
			t.Errorf("%s became a table", src)
		}
	}
}

func TestTheTableCanBeTurnedOff(t *testing.T) {
	d, err := Parse([]byte(`[{"id":1},{"id":2}]`), Options{NoTables: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tableOf(t, d); ok {
		t.Error("NoTables still produced a table")
	}
}

func TestALargeIntegerKeepsAllItsDigits(t *testing.T) {
	// ⛔ encoding/json's default turns every number into a float64, so an
	// identifier of more than fifteen digits — a Snowflake, a bank reference —
	// comes back with its last digits replaced by zeros. Nothing errors; the
	// document is simply wrong in a way that looks entirely plausible.
	const id = "9007199254740993" // 2^53 + 1, the first integer a float64 cannot hold
	d, err := Parse([]byte(`{"id":`+id+`}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := flat(d.Blocks); !strings.Contains(got, id) {
		t.Errorf("the identifier came back as %q, and it was %s", strings.TrimSpace(got), id)
	}
}

func TestNullIsNotTheEmptyString(t *testing.T) {
	// ⛔ JSON distinguishes a key whose value is null from a key whose value is
	// "", and a document that renders both as nothing has merged two facts.
	d, err := Parse([]byte(`{"a":null,"b":""}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := flat(d.Blocks)
	if !strings.Contains(got, "a: null") {
		t.Errorf("null came back as %q", got)
	}
}

func TestAnEmptyObjectOrArrayIsNotDroppedInSilence(t *testing.T) {
	// It is a value the file chose to put there, and a document that loses it
	// has lost a fact.
	for src, want := range map[string]string{
		`{"a":{}}`: "{}",
		`{"a":[]}`: "[]",
	} {
		d, err := Parse([]byte(src), Options{})
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if got := flat(d.Blocks); !strings.Contains(got, want) {
			t.Errorf("%s came back as %q", src, strings.TrimSpace(got))
		}
	}
}

func TestTwoRunsOverTheSameFileAgree(t *testing.T) {
	// ⛔ Go randomises map iteration ON PURPOSE, so without an ordering two
	// conversions of the same file differ and no diff of two documents means
	// anything.
	src := []byte(`{"z":1,"a":2,"m":3,"b":4,"y":5,"c":6}`)
	first := ""
	for i := 0; i < 20; i++ {
		d, err := Parse(src, Options{})
		if err != nil {
			t.Fatal(err)
		}
		got := flat(d.Blocks)
		if i == 0 {
			first = got
			continue
		}
		if got != first {
			t.Fatalf("run %d differs:\n%q\nvs\n%q", i, got, first)
		}
	}
	if !strings.HasPrefix(strings.TrimSpace(first), "a:") {
		t.Errorf("keys are not in a stable order: %q", first)
	}
}

func TestTheCeilingsAreEnforced(t *testing.T) {
	deep := strings.Repeat(`{"a":`, 200) + `1` + strings.Repeat(`}`, 200)
	if _, err := Parse([]byte(deep), Options{MaxDepth: 10}); err == nil {
		t.Error("a deeply nested value was accepted")
	} else if le, ok := err.(shape.LimitError); !ok || le.What != "depth" {
		t.Errorf("refused as %v", err)
	}

	var wide strings.Builder
	wide.WriteString("[")
	for i := 0; i < 50; i++ {
		if i > 0 {
			wide.WriteString(",")
		}
		wide.WriteString(`{"a":1}`)
	}
	wide.WriteString("]")
	if _, err := Parse([]byte(wide.String()), Options{MaxRows: 5}); err == nil {
		t.Error("an array past the row ceiling became a table")
	}
	if _, err := Parse([]byte(wide.String()), Options{MaxNodes: 3}); err == nil {
		t.Error("a value past the node ceiling was built")
	}
	if _, err := Parse(make([]byte, shape.MaxInputBytes+1), Options{}); err == nil {
		t.Error("a file past the input ceiling was read")
	}
}

func TestATitleBecomesAHeading(t *testing.T) {
	d, err := Parse([]byte(`{"a":1}`), Options{Title: "Orders"})
	if err != nil {
		t.Fatal(err)
	}
	h, ok := d.Blocks[0].(richdoc.Heading)
	if !ok {
		// ⛔ A VALUE, not a pointer: *Heading satisfies Block, compiles, and
		// matches no consumer's case. A sibling converter shipped that once.
		if _, isPtr := d.Blocks[0].(*richdoc.Heading); isPtr {
			t.Fatal("the heading is a *Heading, which no consumer matches")
		}
		t.Fatalf("first block is %T", d.Blocks[0])
	}
	if h.Level != 1 {
		t.Errorf("level %d", h.Level)
	}
}

func TestSomethingThatIsNotJSONIsRefused(t *testing.T) {
	if _, err := Parse([]byte("{not json"), Options{}); err == nil {
		t.Error("it was accepted")
	}
}

func TestAReaderIsAccepted(t *testing.T) {
	if _, err := ParseReader(strings.NewReader(`{"a":1}`), Options{}); err != nil {
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

func TestAScalarAtTheTopLevelIsADocument(t *testing.T) {
	for _, src := range []string{`42`, `"hello"`, `true`, `null`} {
		d, err := Parse([]byte(src), Options{})
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if len(d.Blocks) == 0 {
			t.Errorf("%s came back as nothing", src)
		}
	}
}
