// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestTwoFailingStatusesAndWhichIsWhich.
//
// ⛔ 2 SAYS THE COMMAND LINE WAS WRONG, 1 SAYS THE WORK FAILED, and a caller
// that cannot tell them apart cannot decide whether retrying is sensible: it
// will retry a bad flag forever and give up on a file that was briefly
// unreadable. Both statuses existed before this command moved to cobra and
// neither was pinned, so the move could have collapsed them into one with
// nothing failing.
func TestTwoFailingStatusesAndWhichIsWhich(t *testing.T) {
	good := write(t, "x.csv", "a,b\n1,2\n")

	usage := []struct {
		name string
		args []string
	}{
		{"an unknown long flag", []string{"--nonesuch", good}},
		{"an unknown shorthand", []string{"-Z", good}},
		{"a flag with no value", []string{"--format"}},
		{"no arguments at all", nil},
		{"two input files", []string{good, good}},
		{"a file whose format cannot be told", []string{write(t, "data", `{"a":1}`)}},
	}
	for _, c := range usage {
		var out, errw bytes.Buffer
		if code := run(c.args, &out, &errw); code != 2 {
			t.Errorf("%s: exited %d, want 2 — %q", c.name, code, errw.String())
		}
	}

	work := []struct {
		name string
		args []string
	}{
		{"a file that is not there", []string{"/nonexistent/x.csv"}},
		{"a format that does not exist", []string{"--format", "yaml", good}},
		{"a value the reader refuses", []string{"--header", "perhaps", good}},
		{"a delimiter of two characters", []string{"--delimiter", "||", good}},
	}
	for _, c := range work {
		var out, errw bytes.Buffer
		if code := run(c.args, &out, &errw); code != 1 {
			t.Errorf("%s: exited %d, want 1 — %q", c.name, code, errw.String())
		}
	}
}

// TestEveryMessageIsPrefixedOnce.
//
// ⛔ cobra prints its own errors as "Error: ..." unless SilenceErrors is set,
// and this command prefixes every line "richdata:". Without the silence a
// refusal came out twice, once in each wording.
func TestEveryMessageIsPrefixedOnce(t *testing.T) {
	var out, errw bytes.Buffer
	run([]string{"--nonesuch", "x.csv"}, &out, &errw)
	s := errw.String()
	if n := strings.Count(s, "richdata:"); n != 1 {
		t.Errorf("the prefix appears %d times: %q", n, s)
	}
	if strings.Contains(s, "Error:") {
		t.Errorf("cobra printed its own error as well: %q", s)
	}
}

// TestTheMarkersIsCommandLineErrorLooksForStillExist.
//
// ⛔ THIS IS THE TEST THAT MAKES THE TEXT MATCHING HONEST. pflag offers no type
// and no sentinel for "that flag is wrong", so isCommandLineError matches on
// its wording — checked against pflag v1.0.9. An upgrade that rewords one of
// these would otherwise turn a usage error into an exit status of 1 in silence;
// here it fails, names the marker, and says where to look.
//
// ⛔ The argument COUNT is deliberately absent from this list: it is refused by
// this command's own validator as a usageError, decided by type. Every case
// moved out of text matching is one fewer thing an upgrade can reword.
func TestTheMarkersIsCommandLineErrorLooksForStillExist(t *testing.T) {
	cases := []struct {
		marker string
		args   []string
	}{
		{"unknown flag", []string{"--nonesuch", "x.csv"}},
		{"unknown shorthand flag", []string{"-Z", "x.csv"}},
		{"flag needs an argument", []string{"--format"}},
	}
	for _, c := range cases {
		cmd := RootCmd()
		cmd.SetArgs(c.args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		err := cmd.Execute()
		if err == nil {
			t.Errorf("%v was accepted, so the marker %q is untested", c.args, c.marker)
			continue
		}
		if !strings.Contains(err.Error(), c.marker) {
			t.Errorf("%v gave %q, which no longer contains %q — isCommandLineError "+
				"matches on that text and would now answer 1 instead of 2",
				c.args, err.Error(), c.marker)
		}
		if !isCommandLineError(err) {
			t.Errorf("%v: isCommandLineError said no to %q", c.args, err.Error())
		}
	}

	// ⛔ And the negative: a message from this command's own work must NOT be
	// read as a command-line error, or every failure would answer 2.
	for _, s := range []string{
		"x.csv: unknown format \"yaml\": csv, json, xml or text",
		"open /nonexistent/x.csv: no such file or directory",
	} {
		if isCommandLineError(errorString(s)) {
			t.Errorf("isCommandLineError claimed %q was about the command line", s)
		}
	}
}

// errorString is an error holding exactly this text.
type errorString string

func (e errorString) Error() string { return string(e) }
