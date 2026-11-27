// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Identity is a forge account (GitHub, GitLab, Codeberg) linked to a
// student, who can then sign in with it.
type Identity struct {
	Provider  string // github | gitlab | codeberg
	Issuer    string // the forge instance's base URL
	Subject   string // the forge's account ID
	Username  string
	CreatedAt time.Time
}

var (
	ErrNoIdentity       = errors.New("no account is linked to this forge account")
	ErrIdentityTaken    = errors.New("this forge account is linked to another account")
	ErrProviderLinked   = errors.New("a forge account of this kind is already linked")
	ErrNoPendingSignup  = errors.New("this sign-up has expired")
	pendingSignupTTL    = 15 * time.Minute
	uniqueViolationCode = "23505"
)

// UserByIdentity returns the student a forge account is linked to
// (ErrNoIdentity if none, ErrBanned if suspended), and records its current
// username.
func (s *Store) UserByIdentity(ctx context.Context, issuer, subject, username string) (*User, error) {
	var u User
	var banned bool
	err := s.DB.QueryRow(ctx, `UPDATE user_identities i SET username=$3 FROM users u
		WHERE u.id=i.user_id AND i.issuer=$1 AND i.subject=$2
		RETURNING u.id, u.email, u.email_verified_at IS NOT NULL, u.banned_at IS NOT NULL`, issuer, subject, username).
		Scan(&u.ID, &u.Email, &u.EmailVerified, &banned)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoIdentity
	}
	if err != nil {
		return nil, err
	}
	if banned {
		return nil, ErrBanned
	}
	u.Scopes = allScopes()
	return &u, nil
}

// CreateOAuthUser registers a student from a forge account: the email is
// the forge's verified one, there is no password, and the identity is linked
// in the same transaction. ErrEmailTaken if a student or an admin uses the
// email, ErrIdentityTaken if the forge account was linked meanwhile.
func (s *Store) CreateOAuthUser(ctx context.Context, email string, id Identity) (*User, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	u := &User{Email: email, EmailVerified: true, Scopes: allScopes()}
	err = s.withEmailLock(ctx, email, "admins", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO users(email, password_hash, email_verified_at) VALUES ($1, '', now())
			ON CONFLICT (email) DO NOTHING RETURNING id`, email).Scan(&u.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO user_identities(user_id, provider, issuer, subject, username) VALUES ($1,$2,$3,$4,$5)`,
			u.ID, id.Provider, id.Issuer, id.Subject, id.Username)
		if isUniqueViolation(err) {
			return ErrIdentityTaken
		}
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEmailTaken
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// LinkIdentity links a forge account to a student. ErrIdentityTaken if it is
// linked to another student (or already to this one), ErrProviderLinked if
// the student already has an account of this provider.
func (s *Store) LinkIdentity(ctx context.Context, userID int64, id Identity) error {
	var owner int64
	err := s.DB.QueryRow(ctx, `SELECT user_id FROM user_identities WHERE issuer=$1 AND subject=$2`, id.Issuer, id.Subject).Scan(&owner)
	if err == nil {
		return ErrIdentityTaken
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO user_identities(user_id, provider, issuer, subject, username) VALUES ($1,$2,$3,$4,$5)`,
		userID, id.Provider, id.Issuer, id.Subject, id.Username)
	if isUniqueViolation(err) {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.ConstraintName == "user_identities_user_id_provider_key" {
			return ErrProviderLinked
		}
		return ErrIdentityTaken
	}
	return err
}

// UnlinkIdentity removes a student's forge account of a provider; false if
// there was none.
func (s *Store) UnlinkIdentity(ctx context.Context, userID int64, provider string) (bool, error) {
	tag, err := s.DB.Exec(ctx, `DELETE FROM user_identities WHERE user_id=$1 AND provider=$2`, userID, provider)
	return tag.RowsAffected() > 0, err
}

// Identities lists a student's linked forge accounts.
func (s *Store) Identities(ctx context.Context, userID int64) ([]Identity, error) {
	rows, err := s.DB.Query(ctx, `SELECT provider, issuer, subject, username, created_at FROM user_identities
		WHERE user_id=$1 ORDER BY provider`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Identity, error) {
		var i Identity
		return i, r.Scan(&i.Provider, &i.Issuer, &i.Subject, &i.Username, &i.CreatedAt)
	})
}

// HasPassword reports whether a student can sign in with a password
// (accounts created through a forge have none until they set one).
func (s *Store) HasPassword(ctx context.Context, userID int64) (bool, error) {
	var has bool
	err := s.DB.QueryRow(ctx, `SELECT password_hash <> '' FROM users WHERE id=$1`, userID).Scan(&has)
	return has, err
}

// CreatePendingSignup stores a forge sign-up until the student accepts the
// terms, and returns the browser's token (only its hash is stored).
func (s *Store) CreatePendingSignup(ctx context.Context, email string, id Identity) (string, error) {
	tok, err := randomToken(32)
	if err != nil {
		return "", err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO oauth_signups(token_hash, provider, issuer, subject, username, email, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, Hash(tok), id.Provider, id.Issuer, id.Subject, id.Username, email, time.Now().Add(pendingSignupTTL))
	return tok, err
}

// PendingSignup reads a forge sign-up waiting for the terms
// (ErrNoPendingSignup if unknown or expired).
func (s *Store) PendingSignup(ctx context.Context, token string) (string, Identity, error) {
	return s.pendingSignup(ctx, `SELECT email, provider, issuer, subject, username FROM oauth_signups
		WHERE token_hash=$1 AND expires_at > now()`, token)
}

// ConsumePendingSignup reads and deletes a forge sign-up, atomically.
func (s *Store) ConsumePendingSignup(ctx context.Context, token string) (string, Identity, error) {
	return s.pendingSignup(ctx, `DELETE FROM oauth_signups WHERE token_hash=$1 AND expires_at > now()
		RETURNING email, provider, issuer, subject, username`, token)
}

func (s *Store) pendingSignup(ctx context.Context, query, token string) (string, Identity, error) {
	var email string
	var id Identity
	if token == "" {
		return "", id, ErrNoPendingSignup
	}
	err := s.DB.QueryRow(ctx, query, Hash(token)).Scan(&email, &id.Provider, &id.Issuer, &id.Subject, &id.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", id, ErrNoPendingSignup
	}
	return email, id, err
}

func isUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == uniqueViolationCode
}
