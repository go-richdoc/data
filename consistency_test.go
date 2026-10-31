// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// One quantity of data, four formats, one answer.
//
// ⛔ [github.com/go-richdoc/data/shape] exists so the four readers decide the
// same things the same way. Ceilings are one of those things, and the drift was
// invisible until somebody wrote the same table down twice: an export of
// 100 000 rows by 20 fields was refused as JSON, at 2 000 000 nodes against a
// ceiling of 500 000, and accepted as CSV at 2 000 000 cells, because nothing
// bounded rows times columns. Same data, same size, two answers.
package data_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-richdoc/data/csv"
	"github.com/go-richdoc/data/json"
	"github.com/go-richdoc/data/shape"
)

// TestTheSameTableGetsTheSameAnswerInEveryFormat.
func TestTheSameTableGetsTheSameAnswerInEveryFormat(t *testing.T) {
	const (
		rows   = 100_000
		fields = 20
	)

	t.Run("csv", func(t *testing.T) {
		var b strings.Builder
		row := strings.Repeat("a,", fields-1) + "a\n"
		b.Grow(len(row) * rows)
		for range rows {
			b.WriteString(row)
		}
		assertRefusedByACeiling(t, func() error {
			_, err := csv.Parse([]byte(b.String()), csv.Options{})
			return err
		})
	})

	t.Run("json", func(t *testing.T) {
		var b strings.Builder
		b.WriteByte('[')
		obj := `{"a":1,"b":1,"c":1,"d":1,"e":1,"f":1,"g":1,"h":1,"i":1,"j":1,` +
			`"k":1,"l":1,"m":1,"n":1,"o":1,"p":1,"q":1,"r":1,"s":1,"t":1}`
		for i := range rows {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(obj)
		}
		b.WriteByte(']')
		assertRefusedByACeiling(t, func() error {
			_, err := json.Parse([]byte(b.String()), json.Options{})
			return err
		})
	})
}

// assertRefusedByACeiling says only that a ceiling refused it, not which one:
// the formats reach the same quantity by different routes, and demanding the
// same ceiling NAME would be demanding they count alike rather than answer
// alike.
func assertRefusedByACeiling(t *testing.T, f func() error) {
	t.Helper()
	err := f()
	var lim shape.LimitError
	if !errors.As(err, &lim) {
		t.Fatalf("two million values was accepted: %v", err)
	}
	t.Logf("refused for %q at %d", lim.What, lim.Limit)
}

// TestTheCeilingsAreOneQuantityUnderTwoNames keeps the constants from drifting
// apart again, since that drift is what the test above was written for.
func TestTheCeilingsAreOneQuantityUnderTwoNames(t *testing.T) {
	if shape.MaxCells != shape.MaxNodes {
		t.Errorf("MaxCells is %d and MaxNodes is %d; a cell and a node are the same thing "+
			"— a value a document holds — and two numbers for it is the drift shape exists to prevent",
			shape.MaxCells, shape.MaxNodes)
	}
}
