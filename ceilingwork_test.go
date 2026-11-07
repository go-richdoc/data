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
	"testing"

	"github.com/go-richdoc/data/json"
	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/data/xml"
	"github.com/go-richdoc/richdoc"
	"github.com/go-richdoc/richdoc/bombtest"
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
		// ⛔ AN ABSOLUTE CEILING AS WELL AS THE RATIO, because the ratio can
		// hold while both arms balloon together — two refusals that each cost a
		// gigabyte are a ratio of 1.0 and a defect. The pre-scan refuses this
		// before encoding/json decodes anything, which is 40 bytes; 1 MiB names
		// a regression of four orders of magnitude the moment it appears.
		bound := bombtest.Bound{
			Why:        "the JSON pre-scan refuses before encoding/json decodes anything",
			AsksAtMost: 1 << 20,
			HeldAtMost: heapCap,
		}
		small, large := refusalCost(t, "json", bound, smallValues, largeValues,
			func(n int) []byte { return jsonFlatArray(n) },
			func(b []byte) (*richdoc.Document, error) { return json.Parse(b, json.Options{}) })
		ratio := float64(large) / float64(small)
		t.Logf("same ceiling, 8x the source: %d -> %d bytes allocated, ratio %.1f", small, large, ratio)
		if ratio > 2 {
			t.Errorf("the node ceiling does not bound the work: 8x the source costs %.1fx the allocation", ratio)
		}
	})

	t.Run("xml", func(t *testing.T) {
		// ⛔ The XML reader bounds nodes inside its own token walk, so it stops
		// at node 500 001 and the cost is the ceiling's own — about 159 MB,
		// whatever the file's size. 256 MiB names a regression without
		// pretending that figure is small.
		bound := bombtest.Bound{
			Why:        "the XML node ceiling refuses inside encoding/xml's token walk",
			AsksAtMost: 256 << 20,
			HeldAtMost: heapCap,
		}
		small, large := refusalCost(t, "xml", bound, smallValues, largeValues,
			func(n int) []byte { return xmlLeaves(n) },
			func(b []byte) (*richdoc.Document, error) { return xml.Parse(b, xml.Options{}) })
		ratio := float64(large) / float64(small)
		t.Logf("same ceiling, 8x the source: %d -> %d bytes allocated, ratio %.1f", small, large, ratio)
		if ratio > 2 {
			t.Errorf("the node ceiling does not bound the work: 8x the source costs %.1fx the allocation", ratio)
		}
	})
}

// refusalCost builds both inputs and returns what each one asked the allocator
// for, with the bomb run through [bombtest.Run].
//
// ⛔ THE GUARDS ARE NOT HERE ANY MORE. bombtest.Run asserts the refusal, holds
// the process to Bound.HeldAtMost and names a regression past Bound.AsksAtMost
// — and refuses, before the bomb allocates, a Bound that left any of those
// unanswered. The copy that used to live here is how the mistakes got made: a
// Sys delta that was order-dependent, a cap nobody had seen trip. What stays is
// this package's own question, which the shared gate cannot answer: whether the
// two inputs were refused by the SAME ceiling.
func refusalCost(t *testing.T, name string, bound bombtest.Bound, small, large int,
	build func(int) []byte, parse func([]byte) (*richdoc.Document, error)) (uint64, uint64) {
	t.Helper()

	var firstWhat string
	cost := func(n int) uint64 {
		src := build(n)
		var parsed error
		r := bombtest.Run(t, bound, func() error {
			_, err := parse(src)
			parsed = err
			return err
		})
		var lim shape.LimitError
		if !errors.As(parsed, &lim) {
			t.Fatalf("%s with %d values: want a shape.LimitError, got %v", name, n, parsed)
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
		t.Logf("%s with %d values: refused for %q, %d bytes allocated", name, n, lim.What, r.Asked)
		return r.Asked
	}
	return cost(small), cost(large)
}
