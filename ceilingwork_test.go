// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Does a ceiling stop the WORK, or only the ANSWER?
//
// ⛔ THE CONTROL IS THE WHOLE TEST. Two inputs that are refused by the SAME
// ceiling at the SAME node count should cost the same, however large the file
// is: the reader was supposed to stop at node 500 001 and nothing after that
// byte should matter. If allocation scales with the file instead, the ceiling
// is checked AFTER the work it exists to prevent — which is a refusal that
// arrives too late to be a defence.
//
// The XML reader is the positive control. It bounds nodes inside its token
// walk, so its two figures are expected to agree, and a test where they agree
// for XML and disagree for JSON is a test that can tell the two apart. Without
// the XML row, a disagreement would only say "a bigger file allocates more".
//
// ⛔ AND IT IS A PROVEN WITNESS, not a hopeful one. Run before the pre-scan in
// json/prescan.go existed, the JSON row read: refused at node 500 001 both
// times, 243 001 944 bytes against 1 275 409 040, ratio 5.2 — a failure. With
// the pre-scan it refuses for "values" in 40 bytes, both times. The XML row read
// 1.0 on both runs. A control that has never been seen to fail is not a control.
package data_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/go-richdoc/data/json"
	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/data/xml"
	"github.com/go-richdoc/richdoc"
)

// refusedAt is the ratio of what a large refused file allocates to what a
// small one allocates, when both are refused by the same ceiling.
const (
	smallValues = 1 << 20 // just over the node ceiling
	largeValues = 8 << 20 // eight times as much source, same ceiling, same node
)

func TestACeilingThatStopsTheAnswerButNotTheWork(t *testing.T) {
	if smallValues <= shape.MaxNodes || largeValues <= shape.MaxNodes {
		t.Fatalf("both inputs must pass the node ceiling of %d", shape.MaxNodes)
	}

	t.Run("json", func(t *testing.T) {
		small, large := refusalCost(t, "json", smallValues, largeValues,
			func(n int) []byte { return jsonFlatArray(n) },
			func(b []byte) (*richdoc.Document, error) { return json.Parse(b, json.Options{}) })
		ratio := float64(large) / float64(small)
		t.Logf("same ceiling, 8x the source: %d -> %d bytes allocated, ratio %.1f", small, large, ratio)
		if ratio > 2 {
			t.Errorf("the node ceiling does not bound the work: 8x the source costs %.1fx the allocation", ratio)
		}
		// ⛔ AN ABSOLUTE CEILING AS WELL AS THE RATIO, because the ratio can
		// hold while both arms balloon together — two refusals that each cost a
		// gigabyte are a ratio of 1.0 and a defect. The pre-scan refuses this
		// before encoding/json decodes anything, which is 40 bytes; 1 MiB names
		// a regression of four orders of magnitude the moment it appears.
		mustStayUnder(t, "json", large, 1<<20)
	})

	t.Run("xml", func(t *testing.T) {
		small, large := refusalCost(t, "xml", smallValues, largeValues,
			func(n int) []byte { return xmlLeaves(n) },
			func(b []byte) (*richdoc.Document, error) { return xml.Parse(b, xml.Options{}) })
		ratio := float64(large) / float64(small)
		t.Logf("same ceiling, 8x the source: %d -> %d bytes allocated, ratio %.1f", small, large, ratio)
		if ratio > 2 {
			t.Errorf("the node ceiling does not bound the work: 8x the source costs %.1fx the allocation", ratio)
		}
		// ⛔ The XML reader bounds nodes inside its own token walk, so it stops
		// at node 500 001 and the cost is the ceiling's own — about 159 MB,
		// whatever the file's size. 256 MiB names a regression without
		// pretending that figure is small.
		mustStayUnder(t, "xml", large, 256<<20)
	})
}

// TestSysReflectsWhatTheProcessHolds is the self-comparison the heap cap rests
// on, and it comes before every figure that depends on it.
//
// ⛔ A CAP NOBODY HAS SEEN TRIP IS NOT A CAP. The check inside [allocated] never
// fires while the readers behave, so on its own it asserts that a comparison
// compiles. Here a known amount of live data has to show up in
// runtime.MemStats.Sys — if it does not, the cap is decoration and so is every
// bomb guarded by it.
//
// ⛔ IT ASSERTS TWO ABSOLUTE FACTS, AND THE FIRST DRAFT USED A DELTA — which is
// the very mistake this package documents about Sys, made by me an hour after
// writing it down. `after.Sys - before.Sys` was 164 292 904 − 164 292 904 = 0:
// the earlier bombs had already grown the arena to 164 MB, so 64 MiB of new
// live data fitted inside it and moved Sys not at all. The test passed run
// alone and failed run last. Sys only ever grows, so only its ABSOLUTE value
// means anything.
//
// The two facts the cap rests on, both order-independent:
//
//   - Sys is at least what the process holds live — otherwise comparing it to a
//     cap says nothing about memory;
//   - and it is large enough to pass a small cap, so the comparison can fire at
//     all. go-richdoc/html proves the same thing by capping at 8 MiB and
//     watching a real refusal exceed it; a THIRD copy of this idea belongs in a
//     shared package rather than a third file.
func TestSysReflectsWhatTheProcessHolds(t *testing.T) {
	const (
		live     = 64 << 20
		smallCap = 8 << 20
	)

	kept := make([][]byte, 0, 64)
	for range 64 {
		kept = append(kept, make([]byte, 1<<20))
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	runtime.KeepAlive(kept)

	if m.Sys < live {
		t.Errorf("the process holds %d bytes live and Sys reads %d: Sys does not reflect "+
			"what the process holds, so the cap in allocated() guards nothing", live, m.Sys)
	}
	if m.Sys <= smallCap {
		t.Errorf("Sys reads %d, which no cap of %d would catch: the comparison in "+
			"allocated() cannot fire", m.Sys, smallCap)
	}
	t.Logf("the process holds %d bytes live, Sys reads %d, past a notional %d-byte cap",
		live, m.Sys, smallCap)
}

// mustStayUnder is the assertion that NAMES a regression, as opposed to the
// heap cap, which only keeps one from reaching the machine.
//
// ⛔ It reads a CUMULATIVE figure on purpose. A soft memory limit makes the
// collector work harder rather than refusing an allocation, so a reader whose
// ceiling broke would stay inside the cap and merely take minutes — and a slow
// test reads as a slow machine. Bytes handed out cannot be collected away, so
// this fires at once and says both numbers.
func mustStayUnder(t *testing.T, name string, asked uint64, ceiling uint64) {
	t.Helper()
	if asked > ceiling {
		t.Errorf("%s: refusing asked the allocator for %d bytes, past the %d-byte "+
			"ceiling — the bound is no longer refusing before the work",
			name, asked, ceiling)
	}
}

// refusalCost builds both inputs, checks both are refused by the node ceiling,
// and returns what each one allocated.
//
// ⛔ It FAILS if either input is accepted. A reader that returns a document has
// not exercised the ceiling at all, and the ratio would then be comparing two
// successful conversions — a measurement of nothing, reported as a pass.
func refusalCost(t *testing.T, name string, small, large int,
	build func(int) []byte, parse func([]byte) (*richdoc.Document, error)) (uint64, uint64) {
	t.Helper()

	var firstWhat string
	cost := func(n int) uint64 {
		src := build(n)
		total, _, err := allocated(t, func() (*richdoc.Document, error) { return parse(src) })
		var lim shape.LimitError
		if !errors.As(err, &lim) {
			t.Fatalf("%s with %d values: want a shape.LimitError, got %v", name, n, err)
		}
		// ⛔ Both inputs have to be refused by the SAME ceiling, whichever one
		// that is. Two refusals for two different reasons are two different
		// code paths, and their costs are not comparable.
		if firstWhat == "" {
			firstWhat = lim.What
		} else if lim.What != firstWhat {
			t.Fatalf("%s: the small input was refused for %q and the large one for %q",
				name, firstWhat, lim.What)
		}
		t.Logf("%s with %d values: refused for %q, %d bytes allocated", name, n, lim.What, total)
		return total
	}
	return cost(small), cost(large)
}
