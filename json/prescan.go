// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package json

import (
	"github.com/go-richdoc/data/shape"
)

// stdlibMaxNestingDepth is encoding/json's own ceiling on nesting:
// maxNestingDepth = 10000, encoding/json/scanner.go:148, and the same constant
// at encoding/json/jsontext/state.go:53 for the v2 path.
//
// ⛔ It is here so this file can STOP at the point where the decoder is
// guaranteed to refuse. Past that depth the count below would be wrong and it
// does not matter, because nothing gets decoded.
const stdlibMaxNestingDepth = 10000

// countValues is how many JSON values the source declares, read straight from
// the bytes, before anything is decoded.
//
// ⛔ IT COUNTS VALUES AND NOT TOKENS, because an object KEY is a string token
// that is not a value. Counting keys would put the total above what the
// converter will count and this guard would then refuse documents the converter
// accepts — a false refusal, which is worse than the slow acceptance it
// replaced. The whole state machine exists for that one distinction: inside an
// object, a string after `{` or `,` is a key and a string after `:` is a value.
//
// ⛔ IT DOES NOT VALIDATE. Handed malformed JSON it returns some count and no
// error, and encoding/json then produces the real message with the real offset.
// A second opinion on syntax would be a second set of error strings to keep
// agreeing with the first.
//
// ⛔ IT STOPS AT THE END OF THE FIRST TOP-LEVEL VALUE, because that is where
// the decoder stops: Decoder.Decode reads ONE value and leaves the rest of the
// stream alone. A first draft scanned to the end of the file and the fuzz target
// broke it in 700 milliseconds with `0 0 0[0[0[0` — nine values to this
// function, one value to the decoder, so the guard refused a file the converter
// accepts. The minimised input is committed under
// testdata/fuzz/FuzzPrescanAgreesWithTheConverter.
//
// The second return says whether the count can be trusted: it is false once the
// nesting passes [stdlibMaxNestingDepth], where the decoder refuses anyway.
func countValues(src []byte, limit int) (int, bool) {
	// stack holds one byte per open container: '{' or '['. expectKey is
	// meaningful only when the innermost container is an object. started says
	// the top-level value has begun, so that closing it ends the scan.
	var stack []byte
	expectKey := false
	started := false
	count := 0

	i := 0
	for i < len(src) {
		c := src[i]
		switch c {
		case ' ', '\t', '\r', '\n':
			i++
		case '{':
			count++
			started = true
			if len(stack) >= stdlibMaxNestingDepth {
				return count, false
			}
			stack = append(stack, '{')
			expectKey = true
			i++
		case '[':
			count++
			started = true
			if len(stack) >= stdlibMaxNestingDepth {
				return count, false
			}
			stack = append(stack, '[')
			expectKey = false
			i++
		case '}', ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			expectKey = len(stack) > 0 && stack[len(stack)-1] == '{'
			i++
		case ':':
			expectKey = false
			i++
		case ',':
			expectKey = len(stack) > 0 && stack[len(stack)-1] == '{'
			i++
		case '"':
			if !expectKey {
				count++
				started = true
			}
			i = endOfString(src, i)
		default:
			// A number, true, false, null — or a byte that makes no sense here,
			// which the decoder will complain about. Either way one value's
			// worth of bytes is skipped.
			if !expectKey {
				count++
				started = true
			}
			i = endOfBareWord(src, i)
		}
		if started && len(stack) == 0 {
			// The top-level value is closed. Whatever follows belongs to a
			// second value, and the decoder will never read it.
			return count, true
		}
		if count > limit {
			// ⛔ Stop at the limit rather than finishing the count. The caller
			// needs to know THAT it was passed, and a hostile file is exactly
			// the one where finishing would cost the most.
			return count, true
		}
	}
	return count, true
}

// endOfString is the index just past the string starting at src[i], which is a
// double quote. A backslash escapes the next byte, which is the only thing a
// scan has to know to find the closing quote — ሴ needs no special case
// because none of its four hex digits can be a quote.
func endOfString(src []byte, i int) int {
	i++ // the opening quote
	for i < len(src) {
		switch src[i] {
		case '\\':
			i += 2
		case '"':
			return i + 1
		default:
			i++
		}
	}
	return i
}

// endOfBareWord is the index just past a number, true, false, null, or whatever
// else sits here: everything up to the next structural byte or space.
//
// ⛔ It must advance by at least one byte even on a byte it does not understand,
// or a file of one stray character is an endless loop.
func endOfBareWord(src []byte, i int) int {
	j := i + 1
	for j < len(src) {
		switch src[j] {
		case ',', ':', '{', '}', '[', ']', ' ', '\t', '\r', '\n', '"':
			return j
		}
		j++
	}
	return j
}

// prescan refuses a source that declares more values than the ceiling, before
// encoding/json allocates anything for them.
func prescan(src []byte, maxValues int) error {
	n, ok := countValues(src, maxValues)
	if ok && n > maxValues {
		return shape.LimitError{What: "values", Limit: maxValues}
	}
	return nil
}
