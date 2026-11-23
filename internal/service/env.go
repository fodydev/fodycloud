// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"bufio"
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/fodydev/fodycloud/internal/auth"
)

var envKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// ValidateEnv checks environment variable names and values.
func ValidateEnv(env map[string]*string) error {
	if len(env) > 100 {
		return invalid("at most 100 environment variables per app")
	}
	for k, v := range env {
		if !envKey.MatchString(k) {
			return invalid("invalid variable name %q (letters, digits and _, not starting with a digit)", k)
		}
		if k == "PORT" {
			return invalid("PORT is set by FodyCloud")
		}
		if slices.Contains(ReservedEnv, k) {
			return invalid("%s is set by FodyCloud for the app's storage and databases", k)
		}
		if strings.HasPrefix(k, "RAILPACK_") {
			return invalid("%s: RAILPACK_* variables are reserved; use the stack settings instead", k)
		}
		if v != nil && len(*v) > 32<<10 {
			return invalid("value of %s is too long (max 32 KB)", k)
		}
	}
	return nil
}

// ParseDotEnv parses KEY=VALUE lines (blank lines and # comments ignored;
// optional surrounding quotes are removed). Used by the dashboard form.
func ParseDotEnv(text string) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64<<10), 64<<10)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, invalid("environment line %d: expected KEY=VALUE", n)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		out[k] = v
	}
	if err := sc.Err(); err != nil {
		return nil, invalid("environment: %v", err)
	}
	return out, nil
}

func ptrMap(m map[string]string) map[string]*string {
	out := make(map[string]*string, len(m))
	for k, v := range m {
		out[k] = &v
	}
	return out
}

// ListEnv returns the app's environment variables, masked unless reveal is set.
func (s *Service) ListEnv(ctx context.Context, u *auth.User, name string, reveal bool) (map[string]string, error) {
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	env, err := s.appEnv(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	if !reveal {
		for k, v := range env {
			env[k] = mask(v)
		}
	}
	return env, nil
}

func mask(v string) string {
	if len(v) <= 4 {
		return "****"
	}
	return v[:2] + strings.Repeat("*", min(len(v)-2, 12))
}

// SetEnv sets (non-nil) or deletes (nil) variables, then restarts a running
// app so the change takes effect.
func (s *Service) SetEnv(ctx context.Context, u *auth.User, name string, changes map[string]*string) (*Deployment, error) {
	if err := requireWrite(u); err != nil {
		return nil, err
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	if err := ValidateEnv(changes); err != nil {
		return nil, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for k, v := range changes {
		if v == nil {
			_, err = tx.Exec(ctx, `DELETE FROM env_vars WHERE app_id=$1 AND key=$2`, a.ID, k)
		} else {
			_, err = tx.Exec(ctx, `INSERT INTO env_vars(app_id,key,value) VALUES ($1,$2,$3)
				ON CONFLICT (app_id,key) DO UPDATE SET value=EXCLUDED.value`, a.ID, k, *v)
		}
		if err != nil {
			return nil, err
		}
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM env_vars WHERE app_id=$1`, a.ID).Scan(&count); err != nil {
		return nil, err
	}
	if count > 100 {
		return nil, invalid("at most 100 environment variables per app")
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if a.Status == "running" {
		return s.Restart(ctx, u, name)
	}
	return nil, nil
}

func (s *Service) appEnv(ctx context.Context, appID int64) (map[string]string, error) {
	rows, err := s.DB.Query(ctx, `SELECT key, value FROM env_vars WHERE app_id=$1`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	env := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		env[k] = v
	}
	return env, rows.Err()
}
