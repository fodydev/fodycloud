// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fodydev/fodycloud/internal/auth"
)

// Link lifetimes.
const (
	VerifyTTL      = 24 * time.Hour
	ResetTTL       = time.Hour
	EmailChangeTTL = time.Hour
)

// SendVerification emails a confirmation link.
func (s *Service) SendVerification(ctx context.Context, a *auth.Store, u *auth.User) error {
	if u.EmailVerified {
		return nil
	}
	tok, err := a.CreateEmailToken(ctx, u.ID, auth.PurposeVerify, VerifyTTL)
	if err != nil {
		return err
	}
	s.mail(ctx, u.Email, "verify", map[string]any{"URL": s.PublicURL + "/verify?token=" + tok})
	return nil
}

// VerifyEmail consumes a verification link.
func (s *Service) VerifyEmail(ctx context.Context, a *auth.Store, token string) error {
	id, err := a.ConsumeEmailToken(ctx, token, auth.PurposeVerify)
	if err != nil {
		return &InvalidError{Msg: err.Error()}
	}
	return a.MarkVerified(ctx, id)
}

// RequestPasswordReset emails a reset link. It never reveals whether the
// account exists.
func (s *Service) RequestPasswordReset(ctx context.Context, a *auth.Store, email string) error {
	var id int64
	var addr string
	err := s.DB.QueryRow(ctx, `SELECT id, email FROM users WHERE email=$1 AND banned_at IS NULL`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&id, &addr)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	tok, err := a.CreateEmailToken(ctx, id, auth.PurposeReset, ResetTTL)
	if err != nil {
		return err
	}
	s.mail(ctx, addr, "reset", map[string]any{"URL": s.PublicURL + "/reset?token=" + tok})
	return nil
}

// ResetPassword consumes a reset link, sets the password, confirms the email
// (the link proves ownership) and revokes every session and token.
func (s *Service) ResetPassword(ctx context.Context, a *auth.Store, token, password string) error {
	if err := auth.ValidatePassword(password); err != nil {
		return &InvalidError{Msg: err.Error()}
	}
	id, err := a.ConsumeEmailToken(ctx, token, auth.PurposeReset)
	if err != nil {
		return &InvalidError{Msg: err.Error()}
	}
	if err := a.SetPassword(ctx, id, password); err != nil {
		return err
	}
	return a.MarkVerified(ctx, id)
}

// ErrWrongPassword means the password typed to confirm an account change is
// wrong (the web counts it toward the account's sign-in lockout).
var ErrWrongPassword = &InvalidError{Msg: "the current password is wrong"}

// ChangePassword changes a signed-in student's password after checking the
// current one. Other sessions are signed out; this session (its cookie value)
// and API tokens stay valid. The student is told by email.
func (s *Service) ChangePassword(ctx context.Context, a *auth.Store, u *auth.User, session, current, password string) error {
	if _, err := a.Authenticate(ctx, u.Email, current); err != nil {
		return ErrWrongPassword
	}
	if err := a.ChangePassword(ctx, u.ID, password, session); err != nil {
		return invalid("%s", err.Error())
	}
	s.mail(ctx, u.Email, "password-changed", nil)
	return nil
}

// RequestEmailChange starts a change of email address: a link goes to the
// new address, and the change applies only when it is used. The current
// address is told.
func (s *Service) RequestEmailChange(ctx context.Context, a *auth.Store, u *auth.User, newEmail, password string) error {
	if _, err := a.Authenticate(ctx, u.Email, password); err != nil {
		return ErrWrongPassword
	}
	email, err := auth.NormalizeEmail(newEmail)
	if err != nil {
		return invalid("%s", err.Error())
	}
	if email == u.Email {
		return invalid("this is already your email address")
	}
	if used, err := a.EmailInUse(ctx, email); err != nil {
		return err
	} else if used {
		return invalid("%s", auth.ErrEmailTaken.Error())
	}
	tok, err := a.CreateEmailChangeToken(ctx, u.ID, email, EmailChangeTTL)
	if err != nil {
		return err
	}
	s.mail(ctx, email, "email-change-confirm", map[string]any{"Old": u.Email, "URL": s.PublicURL + "/verify-email?token=" + tok})
	s.mail(ctx, u.Email, "email-change-requested", map[string]any{"New": email})
	return nil
}

// ConfirmEmailChange applies a change of email address from its link, and
// tells the previous address.
func (s *Service) ConfirmEmailChange(ctx context.Context, a *auth.Store, token string) (string, error) {
	userID, email, err := a.ConsumeEmailChangeToken(ctx, token)
	if err != nil {
		return "", invalid("%s", err.Error())
	}
	var old string
	if err := s.DB.QueryRow(ctx, `SELECT email FROM users WHERE id=$1`, userID).Scan(&old); err != nil {
		return "", err
	}
	if err := a.ChangeEmail(ctx, userID, email); err != nil {
		return "", invalid("%s", err.Error())
	}
	s.mail(ctx, old, "email-changed", map[string]any{"New": email})
	s.Log.Info("email changed", "user_id", userID)
	return email, nil
}

// DeleteAccount deletes a student's account and data: every app (containers
// and images), then the account and everything attached to it. confirm must
// repeat the account's email. The student is told by email.
func (s *Service) DeleteAccount(ctx context.Context, a *auth.Store, u *auth.User, confirm string) error {
	if err := requireWrite(u); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(confirm), u.Email) {
		return invalid("confirmation does not match your email address")
	}
	if _, err := s.DB.Exec(ctx, `UPDATE deployments SET status='cancelled'
		WHERE status='queued' AND app_id IN (SELECT id FROM apps WHERE user_id=$1)`, u.ID); err != nil {
		return err
	}
	rows, err := s.DB.Query(ctx, `SELECT id, name FROM apps WHERE user_id=$1`, u.ID)
	if err != nil {
		return err
	}
	type app struct {
		id   int64
		name string
	}
	apps, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (app, error) {
		var a app
		return a, r.Scan(&a.id, &a.name)
	})
	if err != nil {
		return err
	}
	for _, a := range apps {
		if err := s.Deployer.Remove(ctx, a.name); err != nil {
			return fmt.Errorf("remove app %s: %w", a.name, err)
		}
		if err := s.dropStorage(ctx, a.id, a.name); err != nil {
			return fmt.Errorf("remove the storage of %s: %w", a.name, err)
		}
	}
	if err := a.DeleteUser(ctx, u.ID); err != nil {
		return err
	}
	s.mail(ctx, u.Email, "account-deleted", nil)
	s.Log.Info("account deleted", "user_id", u.ID, "apps", len(apps))
	return nil
}
