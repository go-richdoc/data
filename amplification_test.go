// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// What a small file can make these four readers ALLOCATE.
//
// ⛔ The ceilings in [shape] bound what is BUILT — rows, columns, depth, nodes
// — and the input is bounded separately at [shape.MaxInputBytes]. Neither of
// those is a bound on MEMORY, and the gap between them is a number nobody had
// measured: how many bytes of heap does one byte of source buy?
//
// ⛔ WHY THESE RUN UNDER LOAD AND THE TIMING ONES DO NOT. Every figure here is
// a count of BYTES ALLOCATED, read from [runtime.MemStats]. It is exact and it
// is independent of what else the machine is doing — the same input allocates
// the same bytes on an idle machine and on one at load 55. A ratio of
// DURATIONS is not, which is why there is no timing claim in this file. If you
// want one, measure it on an idle machine and put it somewhere that says so.
package data_test

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/go-richdoc/data/csv"
	"github.com/go-richdoc/data/json"
	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/data/text"
	"github.com/go-richdoc/data/xml"
	"github.com/go-richdoc/richdoc"
)

// allocated is how many bytes one call allocates in total, and how many the
// document it returns still holds afterwards.
//
// ⛔ TotalAlloc, not HeapAlloc, for the first figure: a reader that allocates a
// gigabyte and frees all but a megabyte has still asked the allocator for a
// gigabyte, and on a machine serving many such calls that is the number that
// matters. HeapAlloc with the document still reachable is the second figure,
// and the two answer different questions.
// ⛔ retained is SIGNED, and that is not a detail. HeapAlloc can be LOWER after
// the call than before it — a refusal frees the decoder's working set and the
// GC may return more than the call ever held. As a uint64 subtraction that came
// out as 18446744073642448112, which is the sort of number that survives a
// review because nobody reads twenty digits as "negative four hundred
// kilobytes". Found by running the report, not by reading it.
func allocated(t *testing.T, f func() (*richdoc.Document, error)) (total uint64, retained int64, err error) {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	doc, err := f()

	runtime.ReadMemStats(&after)
	total = after.TotalAlloc - before.TotalAlloc
	runtime.GC()
	var held runtime.MemStats
	runtime.ReadMemStats(&held)
	retained = int64(held.HeapAlloc) - int64(before.HeapAlloc)
	// ⛔ The document has to be reachable PAST the second reading, or the GC is
	// entitled to collect the very thing being weighed and "retained" comes
	// back as noise near zero.
	runtime.KeepAlive(doc)
	return total, retained, err
}

// --- the adversarial inputs, each one the cheapest shape of its format ---

// csvCells is maxCols columns of one character, over as many rows as fit.
func csvCells(rows, cols int) []byte {
	var b strings.Builder
	row := strings.Repeat("a,", cols-1) + "a\n"
	b.Grow(len(row) * rows)
	for range rows {
		b.WriteString(row)
	}
	return []byte(b.String())
}

// jsonFlatArray is [1,1,1,...]: two bytes per value, the cheapest JSON node.
func jsonFlatArray(n int) []byte {
	var b strings.Builder
	b.Grow(2*n + 2)
	b.WriteByte('[')
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('1')
	}
	b.WriteByte(']')
	return []byte(b.String())
}

// xmlLeaves is <r><a/><a/>...</r>: four bytes per element.
func xmlLeaves(n int) []byte {
	var b strings.Builder
	b.Grow(4*n + 8)
	b.WriteString("<r>")
	for range n {
		b.WriteString("<a/>")
	}
	b.WriteString("</r>")
	return []byte(b.String())
}

// textLines is n one-character lines: two bytes each.
func textLines(n int) []byte {
	return []byte(strings.Repeat("a\n", n))
}

// TestWhatOneByteOfSourceBuys reports the amplification of each reader at its
// cheapest shape, and fails if any of them passes the ceiling below.
//
// ⛔ The ceiling is not a round number anybody liked: it is the measured ratio
// with room above it, so that a change which makes a reader allocate twice as
// much per byte is a test failure rather than a thing somebody notices in
// production. Raising it is allowed; raising it SILENTLY is what this prevents.
func TestWhatOneByteOfSourceBuys(t *testing.T) {
	const ceiling = 400 // bytes allocated per byte of source

	cases := []struct {
		name string
		src  []byte
		f    func([]byte) (*richdoc.Document, error)
	}{
		{"csv 100 rows x 1000 columns", csvCells(100, 1000),
			func(b []byte) (*richdoc.Document, error) { return csv.Parse(b, csv.Options{}) }},
		{"json flat array of 100 000", jsonFlatArray(100_000),
			func(b []byte) (*richdoc.Document, error) { return json.Parse(b, json.Options{}) }},
		{"xml 100 000 leaves", xmlLeaves(100_000),
			func(b []byte) (*richdoc.Document, error) { return xml.Parse(b, xml.Options{}) }},
		{"text 100 000 lines", textLines(100_000),
			func(b []byte) (*richdoc.Document, error) { return text.Parse(b, text.Options{}) }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			total, retained, err := allocated(t, func() (*richdoc.Document, error) { return c.f(c.src) })
			if err != nil {
				t.Fatalf("%d bytes of source: %v", len(c.src), err)
			}
			perByte := float64(total) / float64(len(c.src))
			t.Logf("%d bytes of source -> %d allocated (%.1f per byte), %d retained",
				len(c.src), total, perByte, retained)
			if perByte > ceiling {
				t.Errorf("allocates %.1f bytes per byte of source, past the ceiling of %d",
					perByte, ceiling)
			}
		})
	}
}

// TestTheInputCeilingIsNotAMemoryCeiling is the measurement that matters, and
// it is deliberately a REPORT rather than an assertion: it says what a file of
// exactly [shape.MaxInputBytes] can ask for at each reader's cheapest shape, so
// that a person choosing the ceiling for their own deployment has the number.
//
// ⛔ Run it with -run TestTheInputCeilingIsNotAMemoryCeiling and nothing else.
// It allocates by design and it is skipped in short mode, because a test that
// asks for several gigabytes has no business running in CI next to the others.
func TestTheInputCeilingIsNotAMemoryCeiling(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates by design; -short skips it")
	}
	if os.Getenv("DATA_CEILING_REPORT") == "" {
		t.Skip("set DATA_CEILING_REPORT=1 to run the ceiling report")
	}

	const atTheCeiling = shape.MaxInputBytes // 64 MiB

	// ⛔ EVERY INPUT IS CHECKED AGAINST THE CEILING BEFORE IT IS PARSED. The
	// first version of this report divided the ceiling by a rough bytes-per-item
	// figure and built sources of 67 108 865 and 67 108 871 bytes — one and
	// seven bytes OVER. Both were refused for "bytes", both logged "0
	// allocated", and the report answered a question nobody asked while looking
	// exactly like the one that was. The sizes below are the builders' own
	// formulas, and the guard says so when one of them changes.
	report := func(t *testing.T, name string, src []byte, items int,
		parse func([]byte) (*richdoc.Document, error)) {
		t.Helper()
		if len(src) > atTheCeiling {
			t.Fatalf("%s: the input is %d bytes, past the %d-byte ceiling, so this "+
				"would measure the input check and not the reader", name, len(src), atTheCeiling)
		}
		total, retained, err := allocated(t, func() (*richdoc.Document, error) { return parse(src) })
		t.Logf("%s: %d bytes (%d items) -> %d allocated, %d retained, err=%v",
			name, len(src), items, total, retained, err)
	}

	t.Run("csv", func(t *testing.T) {
		// A 1 000-column row of one-character cells is exactly 2 000 bytes.
		rows := atTheCeiling / 2000
		report(t, "csv", csvCells(rows, 1000), rows*1000,
			func(b []byte) (*richdoc.Document, error) { return csv.Parse(b, csv.Options{}) })
	})
	t.Run("json", func(t *testing.T) {
		// jsonFlatArray(n) is 2n+1 bytes: two brackets, n digits, n-1 commas.
		n := (atTheCeiling - 1) / 2
		report(t, "json", jsonFlatArray(n), n,
			func(b []byte) (*richdoc.Document, error) { return json.Parse(b, json.Options{}) })
	})
	t.Run("xml", func(t *testing.T) {
		// xmlLeaves(n) is 4n+7 bytes: <r>, n times <a/>, </r>.
		n := (atTheCeiling - 7) / 4
		report(t, "xml", xmlLeaves(n), n,
			func(b []byte) (*richdoc.Document, error) { return xml.Parse(b, xml.Options{}) })
	})
}
