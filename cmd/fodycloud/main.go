// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command fodycloud is the FodyCloud control plane: dashboard, REST API, MCP
// server, build/deploy worker and admin CLI in one binary.
//
//	fodycloud serve
//	fodycloud migrate
//	fodycloud admin create-user  --email E          (student; password in FC_NEW_PASSWORD)
//	fodycloud admin create-admin --email E          (admin account; password in FC_NEW_PASSWORD)
//	fodycloud admin set-plan     --email E --plan student [--until 2027-06-30] [--note "..."]
//	fodycloud admin expire       (apply subscription expiry and send plan reminders now instead of waiting for the timer)
//
// Students and admins are separate accounts; run `fodycloud` for every command.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// Version is set at build time.
var Version = "dev"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if len(os.Args) < 2 {
		usage()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(ctx, log)
	case "migrate":
		err = withApp(ctx, log, func(a *app) error { return nil })
	case "admin":
		err = admin(ctx, log, os.Args[2:])
	case "version":
		fmt.Println(Version)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: fodycloud serve | migrate | version

students:
       fodycloud admin create-user  --email E                  (password in FC_NEW_PASSWORD, or --password P)
       fodycloud admin create-token --email E --name N [--scopes apps:read,apps:write]
       fodycloud admin set-plan     --email E --plan PLAN [--until YYYY-MM-DD] [--note TEXT]
       fodycloud admin revoke-plan  --email E
       fodycloud admin expire
       fodycloud admin ban          --email E --reason TEXT
       fodycloud admin unban        --email E
       fodycloud admin ban-repo     --repo URL [--reason TEXT]
       fodycloud admin unban-repo   --repo URL
       fodycloud admin notify       --subject S --body B         (emails every admin)

admins (separate accounts: they can't use the dashboard):
       fodycloud admin create-admin         --email E          (password in FC_NEW_PASSWORD, or --password P)
       fodycloud admin reset-admin-password --email E          (signs the admin out, revokes their admin tokens)
       fodycloud admin create-admin-token   --email E --name N (works only on /v1/admin)
       fodycloud admin check-storage                          (measure volumes and databases, apply database size limits now)
       fodycloud admin list-admins
       fodycloud admin remove-admin         --email E          (refuses to remove the last admin)`)
	os.Exit(2)
}
