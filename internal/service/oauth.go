// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"time"

	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/oauth"
)

// RecentSignIn is how recent a sign-in must be for a student without a
// password to set one, change their email or delete their account.
const RecentSignIn = 15 * time.Minute

// ErrSignInAgain asks a student without a password to sign in again before
// a sensitive change.
var ErrSignInAgain = &InvalidError{Msg: "for your security, log out and log in again, then retry within 15 minutes"}

// OAuthSignIn is the outcome of signing in with a forge: a student to start
// a session for, or a sign-up waiting for the terms (its browser token).
type OAuthSignIn struct {
	User          *auth.User
	PendingSignup string
}

func identity(p *oauth.Provider, id oauth.Identity) auth.Identity {
	return auth.Identity{Provider: p.Name, Issuer: p.Issuer, Subject: id.Subject, Username: id.Username}
}

// SignInWithForge signs a student in with a forge account. A linked account
// signs in (auth.ErrBanned if suspended). Otherwise, a new email starts a
// sign-up that waits for the terms; an email that an account already uses
// is refused: the student signs in with their password and connects the
// forge on the Account page, so a forge account never takes over an
// existing FodyCloud account.
func (s *Service) SignInWithForge(ctx context.Context, a *auth.Store, p *oauth.Provider, id oauth.Identity) (OAuthSignIn, error) {
	u, err := a.UserByIdentity(ctx, p.Issuer, id.Subject, id.Username)
	if err == nil {
		s.Log.Info("forge sign-in", "user_id", u.ID, "provider", p.Name)
		return OAuthSignIn{User: u}, nil
	}
	if !errors.Is(err, auth.ErrNoIdentity) {
		return OAuthSignIn{}, err
	}
	email, err := auth.NormalizeEmail(id.Email)
	if err != nil {
		return OAuthSignIn{}, invalid("%s gave an email address FodyCloud can't use", p.Label)
	}
	if used, err := a.EmailInUse(ctx, email); err != nil {
		return OAuthSignIn{}, err
	} else if used {
		return OAuthSignIn{}, invalid("An account already uses %s. Log in with your password, then connect %s on the Account page.", email, p.Label)
	}
	tok, err := a.CreatePendingSignup(ctx, email, identity(p, id))
	if err != nil {
		return OAuthSignIn{}, err
	}
	return OAuthSignIn{PendingSignup: tok}, nil
}

// PendingForgeSignup describes a forge sign-up waiting for the terms.
func (s *Service) PendingForgeSignup(ctx context.Context, a *auth.Store, token string) (email string, id auth.Identity, err error) {
	email, id, err = a.PendingSignup(ctx, token)
	if errors.Is(err, auth.ErrNoPendingSignup) {
		return "", id, invalid("This sign-up has expired. Start again.")
	}
	return email, id, err
}

// CompleteForgeSignup creates the account of a forge sign-up once the
// student accepted the terms (termsVersion is recorded). Its email is the
// forge's verified one, and it has no password.
func (s *Service) CompleteForgeSignup(ctx context.Context, a *auth.Store, token string, accepted bool, termsVersion string) (*auth.User, error) {
	if !accepted {
		return nil, invalid("Accept the Terms of service and the Acceptable-use policy to create an account.")
	}
	email, id, err := a.ConsumePendingSignup(ctx, token)
	if errors.Is(err, auth.ErrNoPendingSignup) {
		return nil, invalid("This sign-up has expired. Start again.")
	}
	if err != nil {
		return nil, err
	}
	u, err := a.CreateOAuthUser(ctx, email, id)
	switch {
	case errors.Is(err, auth.ErrEmailTaken):
		return nil, invalid("An account already uses %s. Log in with your password instead.", email)
	case errors.Is(err, auth.ErrIdentityTaken):
		return nil, invalid("This forge account is already connected to a FodyCloud account. Log in instead.")
	case err != nil:
		return nil, err
	}
	if err := a.AcceptTerms(ctx, u.ID, termsVersion); err != nil {
		s.Log.Error("record terms acceptance", "err", err)
	}
	s.Log.Info("forge sign-up", "user_id", u.ID, "provider", id.Provider)
	return u, nil
}

// ConnectForge links a forge account to the signed-in student, who can then
// sign in with it. The student is told by email.
func (s *Service) ConnectForge(ctx context.Context, a *auth.Store, u *auth.User, p *oauth.Provider, id oauth.Identity) error {
	if err := requireWrite(u); err != nil {
		return err
	}
	err := a.LinkIdentity(ctx, u.ID, identity(p, id))
	switch {
	case errors.Is(err, auth.ErrIdentityTaken):
		return invalid("This %s account is already connected to a FodyCloud account.", p.Label)
	case errors.Is(err, auth.ErrProviderLinked):
		return invalid("Your account is already connected to %s. Disconnect it first.", p.Label)
	case err != nil:
		return err
	}
	s.mail(ctx, u.Email, "forge-connected", map[string]any{"Provider": p.Label, "Username": id.Username})
	return nil
}

// DisconnectForge unlinks a student's forge account. The last way to sign in
// can't be removed: a student without a password keeps at least one forge.
// label names the forge in messages and email.
func (s *Service) DisconnectForge(ctx context.Context, a *auth.Store, u *auth.User, provider, label string) error {
	if err := requireWrite(u); err != nil {
		return err
	}
	ids, err := a.Identities(ctx, u.ID)
	if err != nil {
		return err
	}
	linked := false
	for _, i := range ids {
		linked = linked || i.Provider == provider
	}
	if !linked {
		return invalid("Your account isn't connected to %s.", label)
	}
	hasPassword, err := a.HasPassword(ctx, u.ID)
	if err != nil {
		return err
	}
	if !hasPassword && len(ids) == 1 {
		return invalid("Set a password before you disconnect %s: it is your only way to log in.", label)
	}
	if _, err := a.UnlinkIdentity(ctx, u.ID, provider); err != nil {
		return err
	}
	s.mail(ctx, u.Email, "forge-disconnected", map[string]any{"Provider": label})
	return nil
}

// SetFirstPassword gives a password to a student who signed up through a
// forge, after a recent sign-in. Other sessions are signed out; session (the
// current one's cookie value) and API tokens stay valid.
func (s *Service) SetFirstPassword(ctx context.Context, a *auth.Store, u *auth.User, session, password string) error {
	if err := requireWrite(u); err != nil {
		return err
	}
	has, err := a.HasPassword(ctx, u.ID)
	if err != nil {
		return err
	}
	if has {
		return invalid("Your account already has a password: use Change password.")
	}
	if err := s.requireRecentSignIn(u); err != nil {
		return err
	}
	if err := a.ChangePassword(ctx, u.ID, password, session); err != nil {
		return invalid("%s", err.Error())
	}
	s.mail(ctx, u.Email, "password-changed", nil)
	return nil
}

// ConfirmIdentity checks that the signed-in student is who they say before a
// sensitive change: their password, or, for a student without one, a recent
// sign-in. ErrWrongPassword counts toward the sign-in lockout.
func (s *Service) ConfirmIdentity(ctx context.Context, a *auth.Store, u *auth.User, password string) error {
	has, err := a.HasPassword(ctx, u.ID)
	if err != nil {
		return err
	}
	if !has {
		return s.requireRecentSignIn(u)
	}
	if _, err := a.Authenticate(ctx, u.Email, password); err != nil {
		return ErrWrongPassword
	}
	return nil
}

func (s *Service) requireRecentSignIn(u *auth.User) error {
	if u.SignedInAt.IsZero() || s.now().Sub(u.SignedInAt) > RecentSignIn {
		return ErrSignInAgain
	}
	return nil
}

// PurgeForgeSignups deletes the forge sign-ups whose terms were never
// accepted, once expired (run every few minutes).
func (s *Service) PurgeForgeSignups(ctx context.Context) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM oauth_signups WHERE expires_at < now()`)
	return err
}
