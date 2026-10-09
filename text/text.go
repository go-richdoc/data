// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package text reads plain text into a [richdoc.Document].
//
// # The whole difficulty is that plain text has no structure
//
// ⛔ So this invents as little as possible. A blank line ends a paragraph, and
// that is the only rule everybody agrees on. It does NOT guess that an
// underlined line is a heading, that an indented run is a quotation, or that a
// line starting with a dash is a list: those are Markdown's rules, and a file
// that wanted them should be read by [github.com/go-richdoc/markdown].
//
// What it does do is keep text that was laid out ON PURPOSE. A run of lines
// that are indented, or that hold runs of two or more spaces, is kept verbatim
// as a code block — because a table drawn with spaces, a log, or a column of
// figures becomes nonsense when its lines are flowed together into a
// paragraph, and that is what every naive reader does to it.
package text

import (
	"io"
	"strings"
	"unicode/utf8"

	"github.com/go-richdoc/data/shape"
	"github.com/go-richdoc/richdoc"
)

// Options says how to read.
type Options struct {
	// Title, when set, becomes a level-1 heading above the document.
	Title string

	// Verbatim keeps the WHOLE file as one code block, laid out exactly as it
	// arrived. It is what a log or a listing wants, and it is the one reading
	// that never loses anything.
	Verbatim bool

	// MaxBlocks bounds how many blocks are built. Zero means
	// [shape.MaxRows] — a text file of a hundred thousand paragraphs is
	// already past what anybody reads.
	MaxBlocks int
}

// Parse reads plain text.
func Parse(src []byte, opt Options) (*richdoc.Document, error) {
	if len(src) > shape.MaxInputBytes {
		return nil, shape.LimitError{What: "bytes", Limit: shape.MaxInputBytes}
	}
	max := opt.MaxBlocks
	if max <= 0 {
		max = shape.MaxRows
	}

	// ⛔ Invalid UTF-8 is repaired rather than refused. A plain text file is
	// whatever somebody's editor saved, and half the world's .txt is Latin-1 or
	// Windows-1252. Refusing it would be correct and useless; a run of
	// replacement characters is visible, which is what a person needs in order
	// to notice and re-save it.
	s := string(src)
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	var blocks []richdoc.Block
	if opt.Title != "" {
		blocks = append(blocks, shape.Heading(1, opt.Title))
	}
	if opt.Verbatim {
		return &richdoc.Document{Blocks: append(blocks,
			richdoc.CodeBlock{Text: s})}, nil
	}

	for _, para := range splitParagraphs(s) {
		if len(blocks) >= max {
			return nil, shape.LimitError{What: "blocks", Limit: max}
		}
		if laidOut(para) {
			blocks = append(blocks, richdoc.CodeBlock{Text: strings.Join(para, "\n")})
			continue
		}
		blocks = append(blocks, shape.Para(strings.Join(trimmed(para), " ")))
	}
	if len(blocks) == 0 {
		return &richdoc.Document{}, nil
	}
	return &richdoc.Document{Blocks: blocks}, nil
}

// ParseReader is Parse over a reader, bounded at [shape.MaxInputBytes].
func ParseReader(r io.Reader, opt Options) (*richdoc.Document, error) {
	b, err := io.ReadAll(io.LimitReader(r, shape.MaxInputBytes+1))
	if err != nil {
		return nil, err
	}
	return Parse(b, opt)
}

// splitParagraphs groups lines, a blank line ending a group.
func splitParagraphs(s string) [][]string {
	var out [][]string
	var cur []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			if len(cur) > 0 {
				out = append(out, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// laidOut says whether a paragraph's lines were positioned deliberately.
//
// ⛔ Two signals, and both have to be about the paragraph as a WHOLE. One
// indented line in otherwise flowing prose is a wrapped sentence, not a
// listing — so a single line is never laid out, and a majority of the lines
// has to carry the signal.
func laidOut(lines []string) bool {
	if len(lines) < 2 {
		return false
	}
	marked := 0
	for _, l := range lines {
		if strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") ||
			strings.Contains(strings.TrimSpace(l), "  ") {
			marked++
		}
	}
	return marked*2 > len(lines)
}

func trimmed(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimSpace(l)
	}
	return out
}
