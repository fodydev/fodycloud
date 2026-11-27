// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"testing"
	"time"

	"github.com/fodydev/fodycloud/internal/auth"
)

func TestRequireRecentSignIn(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	s := &Service{Now: func() time.Time { return now }}
	for _, c := range []struct {
		name   string
		signed time.Time
		ok     bool
	}{
		{"just now", now.Add(-time.Minute), true},
		{"at the limit", now.Add(-RecentSignIn), true},
		{"too old", now.Add(-RecentSignIn - time.Second), false},
		{"API token (no session)", time.Time{}, false},
	} {
		err := s.requireRecentSignIn(&auth.User{SignedInAt: c.signed})
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
