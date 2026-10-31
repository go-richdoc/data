// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package csv

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-richdoc/data/shape"
)

// grid is rows of cols one-character cells.
func grid(rows, cols int) []byte {
	row := strings.Repeat("a,", cols-1) + "a\n"
	return []byte(strings.Repeat(row, rows))
}

// TestTheProductIsBoundedAndTheFactorsAreNot is the defect: a file inside both
// MaxRows and MaxColumns can still be a hundred million cells.
//
// ⛔ The assertion is on WHICH ceiling refuses it. A test that only asked for
// an error would pass on a file refused for rows, which is the case that
// already worked and says nothing about the product.
func TestTheProductIsBoundedAndTheFactorsAreNot(t *testing.T) {
	// 1 000 rows of 1 000 columns: a thousandth of MaxRows, exactly MaxColumns,
	// and a million cells — twice [shape.MaxCells].
	src := grid(1_000, 1_000)
	_, err := Parse(src, Options{})

	var lim shape.LimitError
	if !errors.As(err, &lim) {
		t.Fatalf("a million cells was accepted: %v", err)
	}
	if lim.What != "cells" {
		t.Fatalf("refused for %q; the rows and the columns were both inside their ceilings, "+
			"so only the product can explain it", lim.What)
	}
	if lim.Limit != shape.MaxCells {
		t.Errorf("the error names %d as the ceiling and shape.MaxCells is %d", lim.Limit, shape.MaxCells)
	}
}

// TestTheCellCeilingIsCallerChosen, in both directions: a caller who wants the
// old hundred-million ceiling can have it, and a caller who wants less can too.
func TestTheCellCeilingIsCallerChosen(t *testing.T) {
	src := grid(100, 100) // ten thousand cells
	if _, err := Parse(src, Options{MaxCells: 10_000}); err != nil {
		t.Errorf("ten thousand cells against a ceiling of ten thousand: %v", err)
	}
	_, err := Parse(src, Options{MaxCells: 9_999})
	var lim shape.LimitError
	if !errors.As(err, &lim) || lim.What != "cells" {
		t.Fatalf("ten thousand cells against a ceiling of 9 999: want a cells LimitError, got %v", err)
	}
}

// TestAnOrdinaryExportStillParses: the ceiling has to leave room for the files
// people actually have.
func TestAnOrdinaryExportStillParses(t *testing.T) {
	// 20 000 rows of 20 fields: 400 000 cells, a large but unremarkable export.
	if _, err := Parse(grid(20_000, 20), Options{}); err != nil {
		t.Fatalf("a 20 000-row export of 20 fields was refused: %v", err)
	}
}

// TestARaggedFileCountsTheWidthItWillBeSquaredTo.
//
// ⛔ The rows are squared off to the widest one, so the cells BUILT are
// rows times width and not the sum of the row lengths. A count of what was
// read would undercount exactly the file that exploits it: one wide row and a
// million narrow ones.
func TestARaggedFileCountsTheWidthItWillBeSquaredTo(t *testing.T) {
	var b strings.Builder
	b.WriteString(strings.Repeat("a,", 999) + "a\n") // one row of 1 000
	for range 600 {
		b.WriteString("a\n") // six hundred rows of one
	}
	// 601 rows squared to width 1 000 is 601 000 cells, past the ceiling,
	// although only 1 600 cells were read.
	_, err := Parse([]byte(b.String()), Options{})
	var lim shape.LimitError
	if !errors.As(err, &lim) || lim.What != "cells" {
		t.Fatalf("601 000 cells after squaring: want a cells LimitError, got %v", err)
	}
}
