// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package auth handles the two kinds of accounts and their credentials:
// students (User: dashboard sessions, fc_ API tokens) and admins (Admin, in
// admin.go: admin-panel sessions, fca_ admin tokens). They live in separate
// tables, and an email belongs to one kind of account only.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Token scopes.
const (
	ScopeRead  = "apps:read"
	ScopeWrite = "apps:write"
)

// TokenPrefix marks students' API tokens (helps secret scanners).
// Admin tokens use AdminTokenPrefix ("fca_"), which never matches it.
const TokenPrefix = "fc_"

// SessionTTL is the lifetime of a dashboard session.
const SessionTTL = 7 * 24 * time.Hour

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUnauthenticated    = errors.New("authentication required")
	ErrEmailTaken         = errors.New("an account with this email already exists")
	ErrBanned             = errors.New("this account is suspended")
	ErrInvalidLink        = errors.New("this link is invalid or has expired")
)

// User is an authenticated student. Admins are a different type (Admin).
type User struct {
	ID            int64
	Email         string
	EmailVerified bool
	// Scopes granted to the current credential. Browser sessions get all scopes.
	Scopes []string
}

// Can reports whether the current credential has the scope.
func (u *User) Can(scope string) bool { return slices.Contains(u.Scopes, scope) }

// Store persists users, sessions and tokens.
type Store struct{ DB *pgxpool.Pool }

// NormalizeEmail validates and lower-cases an email address.
func NormalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", fmt.Errorf("invalid email address")
	}
	return email, nil
}

// ValidatePassword enforces a minimum password policy.
func ValidatePassword(pw string) error {
	if len(pw) < 10 {
		return fmt.Errorf("password must be at least 10 characters")
	}
	if len(pw) > 72 { // bcrypt limit
		return fmt.Errorf("password must be at most 72 characters")
	}
	return nil
}

// CreateUser registers a new student. verified marks the email as already
// confirmed (admin-created accounts, or when email delivery is disabled).
// An email used by an admin is refused like any taken email.
func (s *Store) CreateUser(ctx context.Context, email, password string, verified bool) (*User, error) {
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
	u := &User{Email: email, EmailVerified: verified, Scopes: allScopes()}
	err = s.withEmailLock(ctx, email, "admins", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO users(email, password_hash, email_verified_at)
			 VALUES ($1,$2, CASE WHEN $3 THEN now() END)
			 ON CONFLICT (email) DO NOTHING RETURNING id`, email, string(hash), verified).Scan(&u.ID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEmailTaken
	}
	return u, err
}

// withEmailLock runs an account insert in a transaction that holds a lock
// shared by student and admin creation, after checking that the email isn't
// used by the other kind of account (otherTable). The insert returns
// pgx.ErrNoRows when the email is taken in its own table.
func (s *Store) withEmailLock(ctx context.Context, email, otherTable string, insert func(pgx.Tx) error) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('fodycloud-account-email'))`); err != nil {
		return err
	}
	var taken bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+otherTable+` WHERE email=$1)`, email).Scan(&taken); err != nil {
		return err
	}
	if taken {
		return ErrEmailTaken
	}
	if err := insert(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AcceptTerms records that a student accepted a version of the terms of
// service and acceptable-use policy.
func (s *Store) AcceptTerms(ctx context.Context, userID int64, version string) error {
	_, err := s.DB.Exec(ctx, `UPDATE users SET terms_accepted_at=now(), terms_version=$2 WHERE id=$1`, userID, version)
	return err
}

// Authenticate checks an email/password pair.
func (s *Store) Authenticate(ctx context.Context, email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var u User
	var hash string
	var banned bool
	err := s.DB.QueryRow(ctx, `SELECT id, email, email_verified_at IS NOT NULL, banned_at IS NOT NULL, password_hash
		FROM users WHERE email=$1`, email).
		Scan(&u.ID, &u.Email, &u.EmailVerified, &banned, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		// Constant-ish time: still run bcrypt against a dummy hash.
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password)) //nolint:errcheck
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	if banned { // only revealed to someone who knows the password
		return nil, ErrBanned
	}
	u.Scopes = allScopes()
	return &u, nil
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), 12)

// UserByEmail looks a user up by email.
func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := s.DB.QueryRow(ctx, `SELECT id, email, email_verified_at IS NOT NULL FROM users WHERE email=$1`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&u.ID, &u.Email, &u.EmailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("user %s not found", email)
	}
	u.Scopes = allScopes()
	return &u, err
}

// CreateSession starts a dashboard session and returns the cookie value.
func (s *Store) CreateSession(ctx context.Context, userID int64) (string, error) {
	tok, err := randomToken(32)
	if err != nil {
		return "", err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO sessions(token_hash, user_id, expires_at) VALUES ($1,$2,$3)`,
		Hash(tok), userID, time.Now().Add(SessionTTL))
	return tok, err
}

// SessionUser resolves a dashboard session cookie value.
func (s *Store) SessionUser(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, ErrUnauthenticated
	}
	var u User
	err := s.DB.QueryRow(ctx, `SELECT u.id, u.email, u.email_verified_at IS NOT NULL FROM sessions s JOIN users u ON u.id=s.user_id
		WHERE s.token_hash=$1 AND s.expires_at > now() AND u.banned_at IS NULL`,
		Hash(token)).Scan(&u.ID, &u.Email, &u.EmailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	u.Scopes = allScopes()
	return &u, err
}

// DeleteSession logs a browser session out.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, Hash(token))
	return err
}

// APIToken describes a token without its secret.
type APIToken struct {
	ID         int64
	Name       string
	Prefix     string
	Scopes     []string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// ValidateScopes checks requested scopes; read is implied by write.
func ValidateScopes(scopes []string) ([]string, error) {
	out := []string{ScopeRead}
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		switch s {
		case "", ScopeRead:
		case ScopeWrite:
			out = append(out, ScopeWrite)
		default:
			return nil, fmt.Errorf("unknown scope %q (allowed: %s, %s)", s, ScopeRead, ScopeWrite)
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// CreateToken issues an API token. The secret is returned once and only its
// SHA-256 hash is stored.
func (s *Store) CreateToken(ctx context.Context, userID int64, name string, scopes []string) (string, error) {
	scopes, err := ValidateScopes(scopes)
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return "", fmt.Errorf("token name must be 1-64 characters")
	}
	secret, err := randomToken(32)
	if err != nil {
		return "", err
	}
	tok := TokenPrefix + secret
	_, err = s.DB.Exec(ctx, `INSERT INTO api_tokens(user_id, name, token_hash, prefix, scopes) VALUES ($1,$2,$3,$4,$5)`,
		userID, name, Hash(tok), tok[:len(TokenPrefix)+6], scopes)
	return tok, err
}

// TokenUser resolves a bearer API token.
func (s *Store) TokenUser(ctx context.Context, token string) (*User, error) {
	if !strings.HasPrefix(token, TokenPrefix) {
		return nil, ErrUnauthenticated
	}
	var u User
	err := s.DB.QueryRow(ctx, `UPDATE api_tokens t SET last_used_at = now()
		FROM users u WHERE u.id = t.user_id AND t.token_hash=$1 AND t.revoked_at IS NULL AND u.banned_at IS NULL
		RETURNING u.id, u.email, u.email_verified_at IS NOT NULL, t.scopes`, Hash(token)).
		Scan(&u.ID, &u.Email, &u.EmailVerified, &u.Scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	return &u, err
}

// ListTokens lists a user's active tokens.
func (s *Store) ListTokens(ctx context.Context, userID int64) ([]APIToken, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, name, prefix, scopes, created_at, last_used_at FROM api_tokens
		WHERE user_id=$1 AND revoked_at IS NULL ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (APIToken, error) {
		var t APIToken
		err := r.Scan(&t.ID, &t.Name, &t.Prefix, &t.Scopes, &t.CreatedAt, &t.LastUsedAt)
		return t, err
	})
}

// RevokeToken revokes one of the user's tokens.
func (s *Store) RevokeToken(ctx context.Context, userID, tokenID int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE api_tokens SET revoked_at=now() WHERE id=$1 AND user_id=$2`, tokenID, userID)
	return err
}

// Email-link purposes.
const (
	PurposeVerify = "verify"
	PurposeReset  = "reset"
	PurposeEmail  = "email" // change of email address (the token carries the new one)
)

// CreateEmailToken issues a single-use link token (only its hash is stored).
func (s *Store) CreateEmailToken(ctx context.Context, userID int64, purpose string, ttl time.Duration) (string, error) {
	tok, err := randomToken(32)
	if err != nil {
		return "", err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO email_tokens(token_hash, user_id, purpose, expires_at) VALUES ($1,$2,$3,$4)`,
		Hash(tok), userID, purpose, time.Now().Add(ttl))
	return tok, err
}

// ConsumeEmailToken atomically uses a link token and returns its user.
func (s *Store) ConsumeEmailToken(ctx context.Context, token, purpose string) (int64, error) {
	var userID int64
	err := s.DB.QueryRow(ctx, `UPDATE email_tokens SET used_at=now()
		WHERE token_hash=$1 AND purpose=$2 AND used_at IS NULL AND expires_at > now() RETURNING user_id`,
		Hash(token), purpose).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrInvalidLink
	}
	return userID, err
}

// CreateEmailChangeToken issues a single-use link that switches a student's
// email to newEmail when used.
func (s *Store) CreateEmailChangeToken(ctx context.Context, userID int64, newEmail string, ttl time.Duration) (string, error) {
	tok, err := randomToken(32)
	if err != nil {
		return "", err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO email_tokens(token_hash, user_id, purpose, new_email, expires_at) VALUES ($1,$2,$3,$4,$5)`,
		Hash(tok), userID, PurposeEmail, newEmail, time.Now().Add(ttl))
	return tok, err
}

// ConsumeEmailChangeToken uses an email-change link and returns the student
// and the address to switch to.
func (s *Store) ConsumeEmailChangeToken(ctx context.Context, token string) (userID int64, newEmail string, err error) {
	err = s.DB.QueryRow(ctx, `UPDATE email_tokens SET used_at=now()
		WHERE token_hash=$1 AND purpose=$2 AND used_at IS NULL AND expires_at > now() RETURNING user_id, new_email`,
		Hash(token), PurposeEmail).Scan(&userID, &newEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", ErrInvalidLink
	}
	return userID, newEmail, err
}

// EmailInUse reports whether a student or an admin account uses the email.
func (s *Store) EmailInUse(ctx context.Context, email string) (bool, error) {
	var used bool
	err := s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE email=$1) OR EXISTS (SELECT 1 FROM admins WHERE email=$1)`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&used)
	return used, err
}

// ChangeEmail switches a student to a new, confirmed address, unless a
// student or an admin already uses it (ErrEmailTaken).
func (s *Store) ChangeEmail(ctx context.Context, userID int64, email string) error {
	email, err := NormalizeEmail(email)
	if err != nil {
		return err
	}
	err = s.withEmailLock(ctx, email, "admins", func(tx pgx.Tx) error {
		var id int64
		return tx.QueryRow(ctx, `UPDATE users SET email=$2, email_verified_at=now() WHERE id=$1
			AND NOT EXISTS (SELECT 1 FROM users WHERE email=$2) RETURNING id`, userID, email).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEmailTaken
	}
	return err
}

// ChangePassword sets a new password and signs out every other session of
// the student (keepSession is the current one's cookie value). API tokens
// stay valid: the student knew the current password.
func (s *Store) ChangePassword(ctx context.Context, userID int64, password, keepSession string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1`, userID, string(hash)); err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1 AND token_hash <> $2`, userID, Hash(keepSession))
	return err
}

// DeleteUser deletes a student and, through foreign keys, everything
// attached to it (apps, deployments, env vars, domains, metrics, tokens,
// sessions, plans, email links, reminders, abuse flags).
func (s *Store) DeleteUser(ctx context.Context, userID int64) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	return err
}

// MarkVerified confirms a user's email address.
func (s *Store) MarkVerified(ctx context.Context, userID int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE users SET email_verified_at = COALESCE(email_verified_at, now()) WHERE id=$1`, userID)
	return err
}

// SetPassword changes a password and signs the user out everywhere.
func (s *Store) SetPassword(ctx context.Context, userID int64, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1`, userID, string(hash)); err != nil {
		return err
	}
	return s.RevokeAll(ctx, userID)
}

// RevokeAll deletes every session and revokes every API token of a user.
func (s *Store) RevokeAll(ctx context.Context, userID int64) error {
	if _, err := s.DB.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID); err != nil {
		return err
	}
	_, err := s.DB.Exec(ctx, `UPDATE api_tokens SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, userID)
	return err
}

// Hash returns the hex SHA-256 of a secret (tokens are high-entropy, so no salt is needed).
func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func allScopes() []string { return []string{ScopeRead, ScopeWrite} }
