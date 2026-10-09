// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEachFormatGoesAllTheWayToMarkdown(t *testing.T) {
	for _, c := range []struct {
		name, body string
		want       []string
	}{
		{"a.csv", "produit;prix\npain;2,50\n", []string{"| produit | prix |", "pain", "2,50"}},
		{"b.json", `[{"id":1,"n":"ada"},{"id":2,"n":"alan"}]`, []string{"| id | n |", "ada"}},
		{"c.xml", `<r><a><x>1</x></a><a><x>2</x></a></r>`, []string{"| x |", "1"}},
		{"d.txt", "one\ntwo\n\nthree\n", []string{"one two", "three"}},
	} {
		var out, errw bytes.Buffer
		if code := run([]string{write(t, c.name, c.body)}, &out, &errw); code != 0 {
			t.Errorf("%s exited %d: %s", c.name, code, errw.String())
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(out.String(), w) {
				t.Errorf("%s: %q missing from:\n%s", c.name, w, out.String())
			}
		}
	}
}

func TestAFileWithNoExtensionIsRefusedRatherThanGuessed(t *testing.T) {
	// ⛔ A file with no extension could be any of the four, and reading JSON
	// as plain text produces a document that looks fine and says nothing.
	p := write(t, "data", `{"a":1}`)
	var out, errw bytes.Buffer
	if code := run([]string{p}, &out, &errw); code != 2 {
		t.Errorf("exited %d", code)
	}
	if !strings.Contains(errw.String(), "-format") {
		t.Errorf("the refusal does not say what to do: %q", errw.String())
	}
	// And with -format it goes through.
	out.Reset()
	errw.Reset()
	if code := run([]string{"-format", "json", p}, &out, &errw); code != 0 {
		t.Errorf("-format json exited %d: %s", code, errw.String())
	}
}

func TestTheHeaderFlagIsReadAndRefusedProperly(t *testing.T) {
	p := write(t, "x.csv", "1,2\n3,4\n")
	var out, errw bytes.Buffer
	if code := run([]string{"-header", "always", p}, &out, &errw); code != 0 {
		t.Fatalf("exited %d: %s", code, errw.String())
	}
	if !strings.Contains(out.String(), "| 1 | 2 |") {
		t.Errorf("-header always did not promote:\n%s", out.String())
	}
	out.Reset()
	errw.Reset()
	if code := run([]string{"-header", "perhaps", p}, &out, &errw); code == 0 {
		t.Error("-header perhaps was accepted")
	}
	if !strings.Contains(errw.String(), "detect, always or never") {
		t.Errorf("the refusal does not list the choices: %q", errw.String())
	}
}

func TestADelimiterMustBeOneCharacter(t *testing.T) {
	p := write(t, "x.csv", "a|b\n1|2\n")
	var out, errw bytes.Buffer
	if code := run([]string{"-delimiter", "||", p}, &out, &errw); code == 0 {
		t.Error("a two-character delimiter was accepted")
	}
	out.Reset()
	errw.Reset()
	if code := run([]string{"-delimiter", "|", "-header", "always", p}, &out, &errw); code != 0 {
		t.Fatalf("exited %d: %s", code, errw.String())
	}
	if !strings.Contains(out.String(), "| a | b |") {
		t.Errorf("an explicit pipe was not used:\n%s", out.String())
	}
}

func TestWritingToAFileSaysWhatItWrote(t *testing.T) {
	in := write(t, "x.csv", "a,b\n1,2\n")
	outPath := filepath.Join(filepath.Dir(in), "out.md")
	var out, errw bytes.Buffer
	if code := run([]string{"-o", outPath, in}, &out, &errw); code != 0 {
		t.Fatalf("exited %d: %s", code, errw.String())
	}
	b, err := os.ReadFile(outPath)
	if err != nil || len(b) == 0 {
		t.Fatalf("the file is %d bytes (%v)", len(b), err)
	}
	if !strings.Contains(out.String(), "blocks") {
		t.Errorf("it said %q", out.String())
	}
	// A path that cannot be written.
	out.Reset()
	errw.Reset()
	if code := run([]string{"-o", filepath.Dir(in), in}, &out, &errw); code == 0 {
		t.Error("writing over a directory succeeded")
	}
}

func TestEveryRefusalNamesWhatWentWrong(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "exactly one"},
		{[]string{"a", "b"}, "exactly one"},
		{[]string{"-nonesuch", "x.csv"}, ""},
		{[]string{filepath.Join(t.TempDir(), "absent.csv")}, "absent.csv"},
		{[]string{"-format", "yaml", write(t, "x.txt", "hi")}, "unknown format"},
	} {
		var out, errw bytes.Buffer
		code := run(c.args, &out, &errw)
		if code == 0 {
			t.Errorf("%v was accepted", c.args)
			continue
		}
		if c.want != "" && !strings.Contains(errw.String(), c.want) {
			t.Errorf("%v said %q, want %q in it", c.args, errw.String(), c.want)
		}
	}
}

func TestAnEmptyCSVIsReportedWithItsName(t *testing.T) {
	// ⛔ The error has to carry the FILE. Over a directory of a hundred
	// exports, "no rows" alone is a riddle.
	p := write(t, "empty.csv", "\n\n")
	var out, errw bytes.Buffer
	if code := run([]string{p}, &out, &errw); code != 1 {
		t.Errorf("exited %d", code)
	}
	if !strings.Contains(errw.String(), "empty.csv") {
		t.Errorf("the refusal does not name the file: %q", errw.String())
	}
}

func TestTheTitleAndTheRemainingFlagsReachTheReaders(t *testing.T) {
	var out, errw bytes.Buffer
	if code := run([]string{"-title", "Orders", write(t, "x.json", `{"a":1}`)}, &out, &errw); code != 0 {
		t.Fatalf("exited %d: %s", code, errw.String())
	}
	if !strings.Contains(out.String(), "Orders") {
		t.Errorf("the title did not reach the document:\n%s", out.String())
	}

	out.Reset()
	if code := run([]string{"-no-tables", write(t, "y.json", `[{"a":1},{"a":2}]`)}, &out, &errw); code != 0 {
		t.Fatalf("exited %d", code)
	}
	if strings.Contains(out.String(), "| a |") {
		t.Errorf("-no-tables still produced a table:\n%s", out.String())
	}

	out.Reset()
	if code := run([]string{"-verbatim", write(t, "z.txt", "a\n\nb\n")}, &out, &errw); code != 0 {
		t.Fatalf("exited %d", code)
	}
	if !strings.Contains(out.String(), "```") {
		t.Errorf("-verbatim did not produce a code block:\n%s", out.String())
	}
}

func TestEveryExtensionThisToolClaimsIsRecognised(t *testing.T) {
	for ext, want := range map[string]string{
		".csv": "csv", ".tsv": "csv",
		".json": "json", ".ndjson": "json",
		".xml": "xml",
		".txt": "text", ".text": "text", ".log": "text",
		".CSV": "csv", // and case does not matter
		".doc": "",    // and what it does not know, it says nothing about
	} {
		if got := fromName("x" + ext); got != want {
			t.Errorf("%s -> %q, want %q", ext, got, want)
		}
	}
}

func TestMainHandsBackTheExitCode(t *testing.T) {
	old, oldArgs := osExit, os.Args
	t.Cleanup(func() { osExit, os.Args = old, oldArgs })
	got := -1
	osExit = func(code int) { got = code }
	os.Args = []string{"richdata"}
	main()
	if got != 2 {
		t.Errorf("main exited %d with no arguments", got)
	}
}
