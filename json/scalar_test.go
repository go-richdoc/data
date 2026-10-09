// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package json

import (
	"strings"
	"testing"
)

func TestAWideArrayIsStoppedByTheColumnCeiling(t *testing.T) {
	// Rows and columns are different ways for a grid to be too large, and an
	// array of a few objects with a thousand keys each reaches only this one.
	var b strings.Builder
	b.WriteString(`[{`)
	for i := 0; i < 30; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"k`)
		b.WriteString(string(rune('a' + i%26)))
		b.WriteString(string(rune('a' + i/26)))
		b.WriteString(`":1`)
	}
	b.WriteString(`}]`)
	if _, err := Parse([]byte(b.String()), Options{MaxColumns: 5}); err == nil {
		t.Error("an array past the column ceiling became a table")
	}
}

func TestScalarIsWhatMakesUseNumberLoadBearing(t *testing.T) {
	// ⛔ Nothing the decoder produces reaches scalar's last line, because it
	// runs with UseNumber and every number arrives as a json.Number. This
	// drives it directly with a float64 — the value the decoder WOULD hand
	// over if UseNumber were ever dropped — so what happens then is written
	// down rather than discovered.
	//
	// It is also the test that says why UseNumber is there at all: a float64
	// cannot hold 2^53+1, so this is what an identifier would come back as.
	const tooBigForAFloat = 9007199254740993.0
	got := scalar(tooBigForAFloat)
	if got == "9007199254740993" {
		t.Error("a float64 held an integer it cannot hold, which would mean this " +
			"machine's float64 is not an IEEE double")
	}
	if got == "" {
		t.Error("a value scalar does not recognise came back as nothing, which a " +
			"document would show as an empty cell rather than as a problem")
	}

	// And the cases the decoder really does produce.
	for _, c := range []struct {
		in   any
		want string
	}{
		{nil, "null"},
		{"text", "text"},
		{true, "true"},
		{false, "false"},
	} {
		if got := scalar(c.in); got != c.want {
			t.Errorf("%#v gave %q, want %q", c.in, got, c.want)
		}
	}
}
