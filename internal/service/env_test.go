// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"strings"
	"testing"
)

func TestValidateEnv(t *testing.T) {
	v := "x"
	if err := ValidateEnv(map[string]*string{"SECRET_KEY": &v, "_X1": &v, "GONE": nil}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"PORT", "RAILPACK_START_CMD", "1ABC", "A-B", "A B", ""} {
		if err := ValidateEnv(map[string]*string{k: &v}); err == nil {
			t.Errorf("ValidateEnv accepted %q", k)
		}
	}
	big := strings.Repeat("x", 32<<10+1)
	if err := ValidateEnv(map[string]*string{"BIG": &big}); err == nil {
		t.Error("oversized value accepted")
	}
}

func TestParseDotEnv(t *testing.T) {
	env, err := ParseDotEnv("# comment\n\nA=1\nexport B = two words \nC=\"quoted value\"\nD='single'\nE=a=b\nF=\n")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "1", "B": "two words", "C": "quoted value", "D": "single", "E": "a=b", "F": ""}
	if len(env) != len(want) {
		t.Fatalf("got %v", env)
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s = %q, want %q", k, env[k], v)
		}
	}
	if _, err := ParseDotEnv("NOEQUALS"); err == nil {
		t.Error("line without = accepted")
	}
}

func TestStripANSI(t *testing.T) {
	in := "#23 4.151 \x1b[2mdist/\x1b[22m\x1b[32mindex.html\x1b[39m \x1b[1m\x1b[33m779.78 kB\x1b[39m\x1b[22m \x1b]8;;https://x\x07link\x1b]8;;\x07 done"
	if got, want := StripANSI(in), "#23 4.151 dist/index.html 779.78 kB link done"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if StripANSI("plain") != "plain" {
		t.Error("plain text changed")
	}
}
