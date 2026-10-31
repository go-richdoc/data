// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package json

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-richdoc/data/shape"
)

// asLimit is errors.As for a shape.LimitError, named so the tests below read as
// the questions they are asking.
func asLimit(err error, lim *shape.LimitError) bool { return errors.As(err, lim) }

// TestTheCountIsOfValuesAndNotTokens is the distinction the whole state machine
// exists for: a string in an object's key position is not a value.
func TestTheCountIsOfValuesAndNotTokens(t *testing.T) {
	cases := []struct {
		src  string
		want int
	}{
		{`1`, 1},
		{`"a"`, 1},
		{`null`, 1},
		{`true`, 1},
		{`-1.5e10`, 1},
		{`[]`, 1},
		{`{}`, 1},
		{`[1,2,3]`, 4},                  // the array and three numbers
		{`{"a":1}`, 2},                  // the object and one value; "a" is a key
		{`{"a":1,"b":2}`, 3},            // two keys, neither counted
		{`{"a":{"b":"c"}}`, 3},          // outer, inner, "c"
		{`[{"a":1},{"a":2}]`, 5},        // array, two objects, two numbers
		{`{"a":[1,{"b":2}]}`, 5},        // object, array, 1, object, 2
		{` { "a" : [ 1 ] } `, 3},        // whitespace everywhere
		{`{"":""}`, 2},                  // an empty key and an empty value
		{`{"a:b":1}`, 2},                // a colon inside a key
		{`{"a,b":1}`, 2},                // a comma inside a key
		{`{"a\"b":1}`, 2},               // an escaped quote inside a key
		{`{"a":"b\"c"}`, 2},             // an escaped quote inside a value
		{`{"a":"\\"}`, 2},               // a value that is one backslash
		{"{\"a\":\"\\u0022\"}", 2},      // an escaped quote as four hex digits
		{`["{",   "}"]`, 3},             // braces inside strings open nothing
		{`[[[1]]]`, 4},                  // three arrays and a number
		{`{"a":{}}`, 2},                 // an empty object as a value
		{`{"a":[]}`, 2},                 // an empty array as a value
		{`[{"a":{"b":1}},{"a":{"b":2}}]`, 7},
	}
	for _, c := range cases {
		got, ok := countValues([]byte(c.src), 1<<30)
		if !ok {
			t.Errorf("%s: the count came back untrustworthy", c.src)
			continue
		}
		if got != c.want {
			t.Errorf("%s: counted %d values, want %d", c.src, got, c.want)
		}
	}
}

// TestTheSourceCeilingNeverRefusesWhatTheConverterWouldAccept is the derivation
// in [shape.MaxSourceValues] as an executable claim.
//
// ⛔ The thinnest shape is the TABLE path, where the converter counts cells and
// not the objects holding them, so the source has the most values per node it
// will ever have. If the constant is sound anywhere it is sound there, and if
// it is wrong this is where it shows.
func TestTheSourceCeilingNeverRefusesWhatTheConverterWouldAccept(t *testing.T) {
	// n objects of one key each: the worst ratio there is.
	for _, n := range []int{1, 2, 10, 1000} {
		var b strings.Builder
		b.WriteByte('[')
		for i := range n {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`{"k":1}`)
		}
		b.WriteByte(']')
		src := []byte(b.String())

		values, ok := countValues(src, 1<<30)
		if !ok {
			t.Fatalf("n=%d: the count came back untrustworthy", n)
		}
		// What the converter counts: one for the array, one per cell.
		nodes := 1 + n
		// The claim: values <= 2*nodes, which is what makes
		// MaxSourceValues = 2*MaxNodes + 2 a refusal the converter would also
		// have made.
		if values > 2*nodes {
			t.Errorf("n=%d: %d source values against %d nodes, past the 2x the constant assumes",
				n, values, nodes)
		}
	}

	// And the constant itself, so a change to either one has to face the other.
	if shape.MaxSourceValues != 2*shape.MaxNodes+2 {
		t.Errorf("MaxSourceValues is %d, and the derivation says 2*%d+2 = %d",
			shape.MaxSourceValues, shape.MaxNodes, 2*shape.MaxNodes+2)
	}
}

// TestADocumentAtTheCeilingStillParses is the other half: the guard must not be
// so eager that an ordinary document meets it.
func TestADocumentAtTheCeilingStillParses(t *testing.T) {
	// 10 000 rows of 5 fields: 60 001 values, well inside the ceiling, and a
	// perfectly ordinary API page.
	var b strings.Builder
	b.WriteByte('[')
	for i := range 10_000 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"a":1,"b":2,"c":3,"d":4,"e":5}`)
	}
	b.WriteByte(']')
	if _, err := Parse([]byte(b.String()), Options{}); err != nil {
		t.Fatalf("an ordinary 10 000-row export was refused: %v", err)
	}
}

// TestTheGuardStopsCountingAtTheStdlibDepth: past encoding/json's own ceiling
// the count is worthless and says so, because the decoder refuses there anyway.
func TestTheGuardStopsCountingAtTheStdlibDepth(t *testing.T) {
	// ⛔ BOTH containers, because they are two cases of the same switch and a
	// test of one leaves the other's cutoff unexecuted — which is how the
	// object arm of this guard would have shipped unmeasured.
	for name, deep := range map[string][]byte{
		"arrays":  []byte(strings.Repeat("[", stdlibMaxNestingDepth+10)),
		"objects": []byte(strings.Repeat(`{"k":`, stdlibMaxNestingDepth+10)),
	} {
		if _, ok := countValues(deep, 1<<30); ok {
			t.Errorf("%s: the count claimed to be trustworthy past the stdlib's nesting ceiling", name)
		}
		if _, err := Parse(deep, Options{}); err == nil {
			t.Errorf("%s: a file nested past the stdlib ceiling was accepted", name)
		}
	}

	deep := []byte(strings.Repeat("[", stdlibMaxNestingDepth+10))
	// ⛔ And the file is still refused — by the decoder, which is the point of
	// handing it over rather than guessing.
	if _, err := Parse(deep, Options{}); err == nil {
		t.Error("a file nested past the stdlib ceiling was accepted")
	}

	// The control: one level inside the stdlib's ceiling, the count is good.
	ok2 := false
	shallow := []byte(strings.Repeat("[", 10) + strings.Repeat("]", 10))
	if _, ok2 = countValues(shallow, 1<<30); !ok2 {
		t.Error("a ten-deep document was called untrustworthy")
	}
}

// TestMalformedJsonIsTheDecodersToRefuse: the pre-scan must not invent syntax
// errors, or there are two sets of messages to keep in agreement.
func TestMalformedJsonIsTheDecodersToRefuse(t *testing.T) {
	for _, src := range []string{`{`, `[1,`, `}`, `"unterminated`, `tru`, `@`, `[1 2]`, ``} {
		if err := prescan([]byte(src), shape.MaxSourceValues); err != nil {
			t.Errorf("%q: the pre-scan refused it, and syntax is the decoder's job: %v", src, err)
		}
		// Every one of these must still fail, from the decoder.
		if _, err := Parse([]byte(src), Options{}); err == nil {
			t.Errorf("%q was accepted", src)
		}
	}
}

// TestAStrayByteDoesNotHangTheScan: endOfBareWord has to advance.
func TestAStrayByteDoesNotHangTheScan(t *testing.T) {
	done := make(chan struct{})
	go func() {
		countValues([]byte("@@@@@@@@"), 1<<30)
		countValues([]byte(strings.Repeat("\\", 100)), 1<<30)
		countValues([]byte(`"`), 1<<30)
		countValues([]byte(`{"a"`), 1<<30)
		close(done)
	}()
	<-done
}

// TestTheCeilingIsCallerChosen, and a caller's lower ceiling is reached first.
func TestTheCeilingIsCallerChosen(t *testing.T) {
	src := []byte(`[1,2,3,4,5]`) // six values
	if _, err := Parse(src, Options{MaxSourceValues: 6}); err != nil {
		t.Fatalf("six values against a ceiling of six: %v", err)
	}
	_, err := Parse(src, Options{MaxSourceValues: 5})
	var lim shape.LimitError
	if !asLimit(err, &lim) || lim.What != "values" {
		t.Fatalf("six values against a ceiling of five: want a values LimitError, got %v", err)
	}
	if lim.Limit != 5 {
		t.Errorf("the error names %d as the ceiling, and the caller said 5", lim.Limit)
	}
}
