// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package appsdb

import (
	"strings"
	"testing"
)

func TestNamesComeFromIDs(t *testing.T) {
	if Name(42) != "app_42" || ident(42) != `"app_42"` {
		t.Fatalf("got %s %s", Name(42), ident(42))
	}
}

func TestLiteral(t *testing.T) {
	if got := literal(`a'b`); got != `'a''b'` {
		t.Errorf("got %s", got)
	}
}

func TestEnv(t *testing.T) {
	s := &Server{Host: "172.30.0.3", Port: 5432}
	env := s.Env(7, "p@ss/word")
	if env["DATABASE_URL"] != "postgresql://app_7:p%40ss%2Fword@172.30.0.3:5432/app_7" {
		t.Errorf("url: %s", env["DATABASE_URL"])
	}
	for _, k := range []string{"PGHOST", "PGPORT", "PGUSER", "PGPASSWORD", "PGDATABASE"} {
		if env[k] == "" {
			t.Errorf("%s missing", k)
		}
	}
	if !strings.HasPrefix(env["PGUSER"], "app_") || env["PGUSER"] != env["PGDATABASE"] {
		t.Errorf("role and database: %v", env)
	}
}
