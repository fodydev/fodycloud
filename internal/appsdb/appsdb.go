// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package appsdb manages the students' PostgreSQL databases on the shared
// apps-postgres server (not the platform's own database): one database and
// one role per app, both named after the app's numeric ID, never after
// anything a student typed.
//
// Apps reach the server on the fodycloud-apps network (a firewall exception
// allows only port 5432 to it); pg_hba lets each role open only the database
// of the same name. The control plane connects as the superuser over the
// internal appsdb network.
package appsdb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server is the apps' PostgreSQL server.
type Server struct {
	AdminURL string // superuser connection, over the appsdb network
	Host     string // address apps use (on fodycloud-apps)
	Port     int

	once sync.Once
	pool *pgxpool.Pool
	err  error
}

// Enabled reports whether managed databases are configured.
func (s *Server) Enabled() bool { return s != nil && s.AdminURL != "" }

func (s *Server) db(ctx context.Context) (*pgxpool.Pool, error) {
	s.once.Do(func() {
		cfg, err := pgxpool.ParseConfig(s.AdminURL)
		if err != nil {
			s.err = err
			return
		}
		cfg.MaxConns = 4
		s.pool, s.err = pgxpool.NewWithConfig(ctx, cfg)
	})
	return s.pool, s.err
}

// Name is the database and role name of an app.
func Name(appID int64) string { return "app_" + strconv.FormatInt(appID, 10) }

func ident(appID int64) string { return pgx.Identifier{Name(appID)}.Sanitize() }

// URL is the connection URL given to the app.
func (s *Server) URL(appID int64, password string) string {
	u := url.URL{Scheme: "postgresql", User: url.UserPassword(Name(appID), password),
		Host: s.Host + ":" + strconv.Itoa(s.Port), Path: "/" + Name(appID)}
	return u.String()
}

// Env returns the variables injected into the app: DATABASE_URL and the
// standard libpq ones.
func (s *Server) Env(appID int64, password string) map[string]string {
	return map[string]string{
		"DATABASE_URL": s.URL(appID, password),
		"PGHOST":       s.Host,
		"PGPORT":       strconv.Itoa(s.Port),
		"PGUSER":       Name(appID),
		"PGPASSWORD":   password,
		"PGDATABASE":   Name(appID),
	}
}

// Create makes the app's role and database. The role owns the database; no
// one else (PUBLIC) may connect to it.
func (s *Server) Create(ctx context.Context, appID int64, password string, connections int) error {
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	id := ident(appID)
	// CREATE ROLE/DATABASE take no bind parameters: the password is quoted as
	// a literal, the names are identifiers built from the numeric ID.
	stmts := []string{
		fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD %s CONNECTION LIMIT %d", id, literal(password), connections),
		fmt.Sprintf("CREATE DATABASE %s OWNER %s", id, id),
		fmt.Sprintf("REVOKE ALL ON DATABASE %s FROM PUBLIC", id),
	}
	for _, q := range stmts {
		if _, err := db.Exec(ctx, q); err != nil {
			return fmt.Errorf("create database: %w", err)
		}
	}
	return nil
}

// Drop deletes the app's database (closing its connections) and role.
func (s *Server) Drop(ctx context.Context, appID int64) error {
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	id := ident(appID)
	for _, q := range []string{
		fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", id),
		fmt.Sprintf("DROP ROLE IF EXISTS %s", id),
	} {
		if _, err := db.Exec(ctx, q); err != nil {
			return fmt.Errorf("drop database: %w", err)
		}
	}
	return nil
}

// Size returns the database's size in bytes.
func (s *Server) Size(ctx context.Context, appID int64) (int64, error) {
	db, err := s.db(ctx)
	if err != nil {
		return 0, err
	}
	var n int64
	err = db.QueryRow(ctx, "SELECT pg_database_size($1)", Name(appID)).Scan(&n)
	return n, err
}

// SetReadOnly makes new transactions of the database read-only (or writable
// again) and closes its current sessions so the setting applies at once.
func (s *Server) SetReadOnly(ctx context.Context, appID int64, readOnly bool) error {
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	q := fmt.Sprintf("ALTER DATABASE %s SET default_transaction_read_only = on", ident(appID))
	if !readOnly {
		q = fmt.Sprintf("ALTER DATABASE %s RESET default_transaction_read_only", ident(appID))
	}
	if _, err := db.Exec(ctx, q); err != nil {
		return err
	}
	return s.terminate(ctx, db, appID)
}

// SetConnectionLimit changes how many connections the app may open (0 =
// blocked) and, when blocking, closes its sessions.
func (s *Server) SetConnectionLimit(ctx context.Context, appID int64, n int) error {
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	if _, err := db.Exec(ctx, fmt.Sprintf("ALTER ROLE %s CONNECTION LIMIT %d", ident(appID), n)); err != nil {
		return err
	}
	if n == 0 {
		return s.terminate(ctx, db, appID)
	}
	return nil
}

func (s *Server) terminate(ctx context.Context, db *pgxpool.Pool, appID int64) error {
	_, err := db.Exec(ctx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1", Name(appID))
	return err
}

// literal quotes a string as an SQL literal (single quotes doubled; the
// server runs with standard_conforming_strings, the default).
func literal(s string) string {
	out := []byte{'\''}
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'')
		}
		out = append(out, s[i])
	}
	return string(append(out, '\''))
}
