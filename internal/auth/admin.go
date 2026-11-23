// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// AdminSessionTTL is the lifetime of an admin-panel session.
const AdminSessionTTL = 8 * time.Hour

// AdminTokenPrefix marks admin API tokens. They only work on /v1/admin.
const AdminTokenPrefix = "fca_"

// Admin is an authenticated FodyCloud admin. It is a separate account from
// students (User): it can't use the dashboard, own apps or use student
// tokens. ID 0 is the built-in CLI admin (stored as NULL in audit columns).
type Admin struct {
	ID    int64
	Email string
}

// CreateAdmin registers an admin account. An email used by a student is
// refused like any taken email.
func (s *Store) CreateAdmin(ctx context.Context, email, password string) (*Admin, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return nil, err
	}
	a := &Admin{Email: email}
	err = s.withEmailLock(ctx, email, "users", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO admins(email, password_hash) VALUES ($1,$2)
			ON CONFLICT (email) DO NOTHING RETURNING id`, email, string(hash)).Scan(&a.ID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEmailTaken
	}
	return a, err
}

// AuthenticateAdmin checks an admin's email/password pair. Unknown emails
// (including students') get the same answer as a wrong password.
func (s *Store) AuthenticateAdmin(ctx context.Context, email, password string) (*Admin, error) {
	var a Admin
	var hash string
	err := s.DB.QueryRow(ctx, `SELECT id, email, password_hash FROM admins WHERE email=$1`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&a.ID, &a.Email, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password)) //nolint:errcheck // constant-ish time
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return &a, nil
}

// AdminByEmail looks an admin up by email.
func (s *Store) AdminByEmail(ctx context.Context, email string) (*Admin, error) {
	var a Admin
	err := s.DB.QueryRow(ctx, `SELECT id, email FROM admins WHERE email=$1`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&a.ID, &a.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("admin %s not found", email)
	}
	return &a, err
}

// SetAdminPassword changes an admin's password, then signs them out
// everywhere and revokes their admin tokens.
func (s *Store) SetAdminPassword(ctx context.Context, adminID int64, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, `UPDATE admins SET password_hash=$2 WHERE id=$1`, adminID, string(hash)); err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, `DELETE FROM admin_sessions WHERE admin_id=$1`, adminID); err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `UPDATE admin_tokens SET revoked_at=now() WHERE admin_id=$1 AND revoked_at IS NULL`, adminID)
	return err
}

// AdminInfo describes an admin account for listings.
type AdminInfo struct {
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// ListAdmins lists the admin accounts, oldest first.
func (s *Store) ListAdmins(ctx context.Context) ([]AdminInfo, error) {
	rows, err := s.DB.Query(ctx, `SELECT email, created_at FROM admins ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[AdminInfo])
}

// ErrLastAdmin refuses to remove the only admin account.
var ErrLastAdmin = errors.New("this is the last admin account: create another one before removing it")

// RemoveAdmin deletes an admin account with its sessions and admin tokens
// (audit records keep the change, without the admin's name). The last admin
// can't be removed.
func (s *Store) RemoveAdmin(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT id FROM admins FOR UPDATE) a`).Scan(&count); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM admins WHERE email=$1`, email)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("admin %s not found", email)
	}
	if count <= 1 {
		return ErrLastAdmin
	}
	return tx.Commit(ctx)
}

// CreateAdminSession starts an admin-panel session and returns the cookie value.
func (s *Store) CreateAdminSession(ctx context.Context, adminID int64) (string, error) {
	tok, err := randomToken(32)
	if err != nil {
		return "", err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO admin_sessions(token_hash, admin_id, expires_at) VALUES ($1,$2,$3)`,
		Hash(tok), adminID, time.Now().Add(AdminSessionTTL))
	return tok, err
}

// AdminSession resolves an admin-panel session cookie value.
func (s *Store) AdminSession(ctx context.Context, token string) (*Admin, error) {
	if token == "" {
		return nil, ErrUnauthenticated
	}
	var a Admin
	err := s.DB.QueryRow(ctx, `SELECT a.id, a.email FROM admin_sessions s JOIN admins a ON a.id=s.admin_id
		WHERE s.token_hash=$1 AND s.expires_at > now()`, Hash(token)).Scan(&a.ID, &a.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	return &a, err
}

// DeleteAdminSession signs an admin-panel session out.
func (s *Store) DeleteAdminSession(ctx context.Context, token string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM admin_sessions WHERE token_hash=$1`, Hash(token))
	return err
}

// CreateAdminToken issues an admin API token. The secret is returned once
// and only its SHA-256 hash is stored.
func (s *Store) CreateAdminToken(ctx context.Context, adminID int64, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return "", fmt.Errorf("token name must be 1-64 characters")
	}
	secret, err := randomToken(32)
	if err != nil {
		return "", err
	}
	tok := AdminTokenPrefix + secret
	_, err = s.DB.Exec(ctx, `INSERT INTO admin_tokens(admin_id, name, token_hash, prefix) VALUES ($1,$2,$3,$4)`,
		adminID, name, Hash(tok), tok[:len(AdminTokenPrefix)+6])
	return tok, err
}

// AdminTokenAdmin resolves a bearer admin API token.
func (s *Store) AdminTokenAdmin(ctx context.Context, token string) (*Admin, error) {
	if !strings.HasPrefix(token, AdminTokenPrefix) {
		return nil, ErrUnauthenticated
	}
	var a Admin
	err := s.DB.QueryRow(ctx, `UPDATE admin_tokens t SET last_used_at = now()
		FROM admins a WHERE a.id = t.admin_id AND t.token_hash=$1 AND t.revoked_at IS NULL
		RETURNING a.id, a.email`, Hash(token)).Scan(&a.ID, &a.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	return &a, err
}

// ListAdminTokens lists an admin's active tokens.
func (s *Store) ListAdminTokens(ctx context.Context, adminID int64) ([]APIToken, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, name, prefix, created_at, last_used_at FROM admin_tokens
		WHERE admin_id=$1 AND revoked_at IS NULL ORDER BY id`, adminID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (APIToken, error) {
		var t APIToken
		err := r.Scan(&t.ID, &t.Name, &t.Prefix, &t.CreatedAt, &t.LastUsedAt)
		return t, err
	})
}

// RevokeAdminToken revokes one of the admin's tokens.
func (s *Store) RevokeAdminToken(ctx context.Context, adminID, tokenID int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE admin_tokens SET revoked_at=now() WHERE id=$1 AND admin_id=$2`, tokenID, adminID)
	return err
}
