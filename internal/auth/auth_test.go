// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"slices"
	"testing"
)

func TestValidateScopes(t *testing.T) {
	got, err := ValidateScopes([]string{"apps:write"})
	if err != nil || !slices.Equal(got, []string{ScopeRead, ScopeWrite}) {
		t.Fatalf("write implies read: got %v, %v", got, err)
	}
	got, err = ValidateScopes(nil)
	if err != nil || !slices.Equal(got, []string{ScopeRead}) {
		t.Fatalf("default is read-only: got %v, %v", got, err)
	}
	if _, err := ValidateScopes([]string{"admin"}); err == nil {
		t.Fatal("unknown scope accepted")
	}
}

func TestUserCan(t *testing.T) {
	ro := &User{Scopes: []string{ScopeRead}}
	if ro.Can(ScopeWrite) || !ro.Can(ScopeRead) {
		t.Fatal("read-only user scopes wrong")
	}
}

func TestHashIsStableAndNotPlaintext(t *testing.T) {
	h := Hash("fc_secret")
	if h != Hash("fc_secret") || h == "fc_secret" || len(h) != 64 {
		t.Fatalf("unexpected hash %q", h)
	}
}

func TestNormalizeEmail(t *testing.T) {
	if e, err := NormalizeEmail("  Student@Univ.CM "); err != nil || e != "student@univ.cm" {
		t.Fatalf("got %q, %v", e, err)
	}
	for _, bad := range []string{"", "nope", "a@", "Name <a@b.cm>"} {
		if _, err := NormalizeEmail(bad); err == nil {
			t.Errorf("NormalizeEmail(%q) accepted", bad)
		}
	}
}
