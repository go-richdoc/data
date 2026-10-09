// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

func TestWritingMarkdownThatFailsIsReported(t *testing.T) {
	// ⛔ Nothing these readers produce can make the Markdown writer fail, so
	// without a seam this branch is a claim nobody has tested — and the day a
	// reader starts producing something the writer refuses, the tool would
	// have to be the thing that says so.
	was := writeMarkdown
	t.Cleanup(func() { writeMarkdown = was })
	writeMarkdown = func(*richdoc.Document) ([]byte, error) {
		return nil, errors.New("the writer refused it")
	}

	p := filepath.Join(t.TempDir(), "x.csv")
	if err := os.WriteFile(p, []byte("a,b\n1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errw bytes.Buffer
	if code := run([]string{p}, &out, &errw); code != 1 {
		t.Errorf("exited %d", code)
	}
	if !strings.Contains(errw.String(), "writing Markdown") {
		t.Errorf("it said %q", errw.String())
	}
}

func TestHeaderNeverIsAcceptedAsAWord(t *testing.T) {
	// `never` and `no` both reach the same mode, and neither had a test: the
	// flag parser accepted them and nothing checked the mode came back.
	for _, word := range []string{"never", "no", "NEVER"} {
		got, err := headerMode(word)
		if err != nil {
			t.Errorf("%q: %v", word, err)
		}
		if got.String() != "never" {
			t.Errorf("%q gave %v", word, got)
		}
	}
	for _, word := range []string{"always", "yes", "Always"} {
		got, err := headerMode(word)
		if err != nil || got.String() != "always" {
			t.Errorf("%q gave %v, %v", word, got, err)
		}
	}
	for _, word := range []string{"", "detect"} {
		got, err := headerMode(word)
		if err != nil || got.String() != "detect" {
			t.Errorf("%q gave %v, %v", word, got, err)
		}
	}
}
