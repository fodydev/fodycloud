// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/plans"
	"github.com/fodydev/fodycloud/internal/service"
)

func admin(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) == 0 {
		usage()
	}
	fs := flag.NewFlagSet("admin "+args[0], flag.ExitOnError)
	email := fs.String("email", "", "account email")
	password := fs.String("password", "", "password (create-user, create-admin, reset-admin-password); prefer FC_NEW_PASSWORD")
	plan := fs.String("plan", "", "plan name (set-plan)")
	until := fs.String("until", "", "last valid day YYYY-MM-DD, empty = no expiry (set-plan)")
	note := fs.String("note", "", "note stored in the audit log (set-plan)")
	name := fs.String("name", "", "token name (create-token)")
	scopes := fs.String("scopes", auth.ScopeRead+","+auth.ScopeWrite, "token scopes (create-token)")
	reason := fs.String("reason", "", "reason (ban, ban-repo)")
	repoURL := fs.String("repo", "", "repository URL (ban-repo, unban-repo)")
	subject := fs.String("subject", "", "subject (notify)")
	body := fs.String("body", "", "body (notify)")
	fs.Parse(args[1:]) //nolint:errcheck
	// cliAdmin is the admin the CLI acts as: the admin account named by
	// FC_ADMIN_EMAIL when it exists (for the audit log), else the built-in
	// CLI admin (ID 0, recorded as NULL).
	cliAdmin := func(a *app) *auth.Admin {
		if by, err := a.auth.AdminByEmail(ctx, os.Getenv("FC_ADMIN_EMAIL")); err == nil {
			return by
		}
		return &auth.Admin{Email: "cli"}
	}
	newPassword := func() string {
		if *password != "" {
			return *password
		}
		return os.Getenv("FC_NEW_PASSWORD") // preferred: keeps the password out of the process list
	}
	switch args[0] {
	case "notify": // used by host scripts (backups) to alert admins
		return withApp(ctx, log, func(a *app) error {
			a.svc.NotifyAdmins(ctx, *subject, *body)
			return nil
		})
	case "ban-repo", "unban-repo":
		return withApp(ctx, log, func(a *app) error {
			if args[0] == "unban-repo" {
				return a.svc.UnbanRepo(ctx, cliAdmin(a), *repoURL)
			}
			url, err := a.svc.BanRepo(ctx, cliAdmin(a), *repoURL, *reason)
			if err == nil {
				fmt.Println("banned", url)
			}
			return err
		})
	}
	if args[0] == "expire" {
		return withApp(ctx, log, func(a *app) error {
			n, err := a.svc.ExpireSubscriptions(ctx)
			if err != nil {
				return err
			}
			r, err := a.svc.SendReminders(ctx)
			if err == nil {
				fmt.Printf("%d subscription(s) expired, %d reminder(s) sent\n", n, r)
			}
			return err
		})
	}
	if args[0] == "check-storage" { // measure volumes and databases, enforce database size limits now
		return withApp(ctx, log, func(a *app) error {
			if err := a.svc.CheckStorage(ctx); err != nil {
				return err
			}
			fmt.Println("storage checked")
			return nil
		})
	}
	if args[0] == "list-admins" {
		return withApp(ctx, log, func(a *app) error {
			admins, err := a.auth.ListAdmins(ctx)
			if err != nil {
				return err
			}
			for _, ad := range admins {
				fmt.Printf("%s\tsince %s\n", ad.Email, ad.CreatedAt.Format("2006-01-02"))
			}
			return nil
		})
	}
	if *email == "" {
		return fmt.Errorf("--email is required")
	}
	return withApp(ctx, log, func(a *app) error {
		switch args[0] {
		case "create-user":
			u, err := a.auth.CreateUser(ctx, *email, newPassword(), true) // created by an admin: email trusted
			if err != nil {
				return err
			}
			fmt.Printf("created user %s (id %d)\n", u.Email, u.ID)
		case "create-admin":
			ad, err := a.auth.CreateAdmin(ctx, *email, newPassword())
			if err != nil {
				return err
			}
			fmt.Printf("created admin %s (id %d)\n", ad.Email, ad.ID)
		case "remove-admin":
			if err := a.auth.RemoveAdmin(ctx, *email); err != nil {
				return err
			}
			fmt.Println("removed admin", *email)
		case "reset-admin-password":
			ad, err := a.auth.AdminByEmail(ctx, *email)
			if err != nil {
				return err
			}
			if err := a.auth.SetAdminPassword(ctx, ad.ID, newPassword()); err != nil {
				return err
			}
			fmt.Println("password changed for admin", ad.Email, "(signed out, admin tokens revoked)")
		case "create-admin-token":
			ad, err := a.auth.AdminByEmail(ctx, *email)
			if err != nil {
				return err
			}
			tok, err := a.auth.CreateAdminToken(ctx, ad.ID, *name)
			if err != nil {
				return err
			}
			fmt.Println(tok)
		case "set-plan", "revoke-plan":
			p := *plan
			if args[0] == "revoke-plan" {
				p = plans.Default
			}
			var validUntil *time.Time
			if *until != "" {
				t, err := time.Parse("2006-01-02", *until)
				if err != nil {
					return fmt.Errorf("--until: %w", err)
				}
				t = t.Add(24*time.Hour - time.Second)
				validUntil = &t
			}
			info, err := setPlanAs(ctx, a, cliAdmin(a), *email, p, validUntil, *note)
			if err != nil {
				return err
			}
			fmt.Printf("%s is now on the %s plan (valid until %s)\n", *email, info.Plan, fmtTime(info.ValidUntil))
		case "ban":
			if err := a.svc.Ban(ctx, cliAdmin(a), a.auth, *email, *reason); err != nil {
				return err
			}
			fmt.Println(*email, "suspended")
		case "unban":
			if err := a.svc.Unban(ctx, cliAdmin(a), *email); err != nil {
				return err
			}
			fmt.Println(*email, "reinstated")
		case "create-token":
			u, err := a.auth.UserByEmail(ctx, *email)
			if err != nil {
				return err
			}
			tok, err := a.auth.CreateToken(ctx, u.ID, *name, strings.Split(*scopes, ","))
			if err != nil {
				return err
			}
			fmt.Println(tok)
		default:
			usage()
		}
		return nil
	})
}

// setPlanAs assigns a plan; the built-in CLI admin (no account) is noted as "[cli]".
func setPlanAs(ctx context.Context, a *app, by *auth.Admin, email, plan string, until *time.Time, note string) (*service.PlanInfo, error) {
	if by.ID == 0 {
		note = strings.TrimSpace("[cli] " + note)
	}
	return a.svc.SetPlan(ctx, by, email, plan, until, note)
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return "no expiry"
	}
	return t.Format("2006-01-02")
}
