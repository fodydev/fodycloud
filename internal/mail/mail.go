// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package mail queues and sends transactional email. Messages go to the
// email_outbox table first, so requests never wait for SMTP; a background
// loop delivers them with retries.
package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config is the SMTP configuration. An empty Host disables delivery
// (messages are only logged).
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// Mailer queues and delivers email.
type Mailer struct {
	DB  *pgxpool.Pool
	Cfg Config
	Log *slog.Logger
	// MaxAttempts before a message is abandoned.
	MaxAttempts int
}

// Enabled reports whether SMTP delivery is configured.
func (m *Mailer) Enabled() bool { return m != nil && m.Cfg.Host != "" }

// Queue stores a message for delivery.
func (m *Mailer) Queue(ctx context.Context, to, subject, body string) error {
	if !m.Enabled() {
		m.Log.Info("email not sent (SMTP not configured)", "to", to, "subject", subject)
		return nil
	}
	if strings.ContainsAny(to+subject, "\r\n") {
		return errors.New("mail: invalid header value")
	}
	_, err := m.DB.Exec(ctx, `INSERT INTO email_outbox(to_addr, subject, body) VALUES ($1,$2,$3)`, to, subject, body)
	return err
}

// Template renders one of the built-in templates and queues it.
func (m *Mailer) Template(ctx context.Context, to, name string, data any) error {
	t := templates.Lookup(name)
	if t == nil {
		return fmt.Errorf("mail: unknown template %q", name)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}
	subject, body, _ := strings.Cut(buf.String(), "\n")
	return m.Queue(ctx, to, strings.TrimPrefix(subject, "Subject: "), strings.TrimLeft(body, "\n"))
}

// Run delivers queued messages until ctx is cancelled.
func (m *Mailer) Run(ctx context.Context) {
	if !m.Enabled() {
		return
	}
	maxAttempts := m.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 8
	}
	for {
		m.deliverBatch(ctx, maxAttempts)
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (m *Mailer) deliverBatch(ctx context.Context, maxAttempts int) {
	rows, err := m.DB.Query(ctx, `SELECT id, to_addr, subject, body, attempts FROM email_outbox
		WHERE sent_at IS NULL AND attempts < $1 AND next_attempt_at <= now() ORDER BY id LIMIT 20`, maxAttempts)
	if err != nil {
		m.Log.Error("mail outbox", "err", err)
		return
	}
	type msg struct {
		id                int64
		to, subject, body string
		attempts          int
	}
	msgs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (msg, error) {
		var x msg
		return x, r.Scan(&x.id, &x.to, &x.subject, &x.body, &x.attempts)
	})
	if err != nil {
		m.Log.Error("mail outbox", "err", err)
		return
	}
	for _, x := range msgs {
		if err := m.send(x.to, x.subject, x.body); err != nil {
			backoff := time.Duration(1<<min(x.attempts, 10)) * time.Minute
			m.DB.Exec(ctx, `UPDATE email_outbox SET attempts=attempts+1, last_error=$2, next_attempt_at=now()+$3::interval WHERE id=$1`, //nolint:errcheck
				x.id, err.Error(), backoff.String())
			m.Log.Warn("email delivery failed", "to", x.to, "attempt", x.attempts+1, "err", err)
			continue
		}
		m.DB.Exec(ctx, `UPDATE email_outbox SET sent_at=now(), attempts=attempts+1 WHERE id=$1`, x.id) //nolint:errcheck
	}
}

// Message builds the RFC 5322 message.
func Message(from, to, subject, body string, now time.Time) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\n", from, to, mime.QEncoding.Encode("utf-8", subject), now.Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(b.String())
}

func (m *Mailer) send(to, subject, body string) error {
	addr := net.JoinHostPort(m.Cfg.Host, strconv.Itoa(m.Cfg.Port))
	tlsCfg := &tls.Config{ServerName: m.Cfg.Host, MinVersion: tls.VersionTLS12}
	var c *smtp.Client
	if m.Cfg.Port == 465 { // implicit TLS
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", addr, tlsCfg)
		if err != nil {
			return err
		}
		if c, err = smtp.NewClient(conn, m.Cfg.Host); err != nil {
			return err
		}
	} else {
		conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
		if err != nil {
			return err
		}
		if c, err = smtp.NewClient(conn, m.Cfg.Host); err != nil {
			return err
		}
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				c.Close()
				return err
			}
		} else if m.Cfg.Username != "" {
			c.Close()
			return errors.New("SMTP server does not offer STARTTLS; refusing to send credentials in clear text")
		}
	}
	defer c.Close()
	if m.Cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", m.Cfg.Username, m.Cfg.Password, m.Cfg.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(bareAddress(m.Cfg.From)); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(Message(m.Cfg.From, to, subject, body, time.Now())); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func bareAddress(from string) string {
	if i := strings.LastIndexByte(from, '<'); i >= 0 {
		return strings.TrimSuffix(from[i+1:], ">")
	}
	return from
}

// Templates: the first line is "Subject: ...".
var templates = template.Must(template.New("").Parse(`
{{define "verify"}}Subject: Confirm your FodyCloud email
Hello,

Please confirm your email address to start deploying on FodyCloud:

{{.URL}}

The link is valid for 24 hours. If you did not create an account, ignore this email.

— FodyCloud
{{end}}
{{define "reset"}}Subject: Reset your FodyCloud password
Hello,

Someone (hopefully you) asked to reset your FodyCloud password:

{{.URL}}

The link is valid for 1 hour and can be used once. Resetting signs you out everywhere
and revokes your API tokens. If you did not ask for this, ignore this email.

— FodyCloud
{{end}}
{{define "password-changed"}}Subject: Your FodyCloud password was changed
Hello,

The password of your FodyCloud account was just changed, and your other
sessions were signed out.

If you didn't do it, reset your password now with "Forgot your password?" on
the sign-in page, and contact the FodyCloud team.

— FodyCloud
{{end}}
{{define "forge-connected"}}Subject: {{.Provider}} was connected to your FodyCloud account
Hello,

The {{.Provider}} account {{.Username}} was connected to your FodyCloud
account. You can now log in with it.

If you didn't do it, disconnect it on the Account page, change your password,
and contact the FodyCloud team.

— FodyCloud
{{end}}
{{define "forge-disconnected"}}Subject: {{.Provider}} was disconnected from your FodyCloud account
Hello,

{{.Provider}} was disconnected from your FodyCloud account: you can't log in
with it any more.

If you didn't do it, change your password and contact the FodyCloud team.

— FodyCloud
{{end}}
{{define "email-change-confirm"}}Subject: Confirm your new FodyCloud email
Hello,

Someone asked to use this address for the FodyCloud account {{.Old}}.
To confirm, open this link (valid for 1 hour):

{{.URL}}

If you didn't ask for it, ignore this email: nothing changes.

— FodyCloud
{{end}}
{{define "email-change-requested"}}Subject: Change of your FodyCloud email requested
Hello,

A change of your FodyCloud account's email to {{.New}} was requested. It
applies only once the link sent to {{.New}} is used.

If you didn't ask for it, change your password now and contact the FodyCloud
team.

— FodyCloud
{{end}}
{{define "email-changed"}}Subject: Your FodyCloud email was changed
Hello,

Your FodyCloud account now uses {{.New}}. This address will no longer receive
FodyCloud emails.

If you didn't do it, contact the FodyCloud team right away.

— FodyCloud
{{end}}
{{define "account-deleted"}}Subject: Your FodyCloud account was deleted
Hello,

Your FodyCloud account and its apps were deleted, as you asked. Your apps no
longer answer at their addresses.

Encrypted backups of the platform age out within 6 months; your data is not
restored from them except to repair the platform.

— FodyCloud
{{end}}
{{define "db-full"}}Subject: The database of {{.App}} is full
Hello,

The PostgreSQL database of your app {{.App}} uses {{.Size}}, more than the
{{.Limit}} your plan allows. It is now read-only: your app can read its data,
but not change it.

To make room, delete data you no longer need, then compact the database. The
guide explains how: search for "database is full" in the FodyCloud docs. Writes
are allowed again within 10 minutes once the database fits its limit.

A database that keeps growing past its limit is blocked.

— FodyCloud
{{end}}
{{define "db-writable"}}Subject: The database of {{.App}} accepts writes again
Hello,

The PostgreSQL database of your app {{.App}} now uses {{.Size}}, within the
{{.Limit}} your plan allows: your app can change its data again.

— FodyCloud
{{end}}
{{define "plan-expiring"}}Subject: Your FodyCloud {{.Plan}} plan ends {{.When}}
Hello,

Your {{.Plan}} plan is valid until {{.Until}}. After that your account goes back
to the free plan: apps above the free limits will be stopped (never deleted).

Contact the FodyCloud team to renew.

— FodyCloud
{{end}}
{{define "plan-expired"}}Subject: Your FodyCloud plan has ended
Hello,

Your {{.Plan}} plan has ended and your account is back on the free plan.
Apps above the free limits were stopped; they are kept and can be started again
after renewing.

— FodyCloud
{{end}}
{{define "banned"}}Subject: Your FodyCloud account has been suspended
Hello,

Your FodyCloud account has been suspended and your apps were stopped.
Reason: {{.Reason}}

If you think this is a mistake, reply to this email.

— FodyCloud
{{end}}
{{define "app-abandon-warning"}}Subject: Your FodyCloud app {{.App}} will be stopped on {{.StopOn}}
Hello,

Your app {{.App}} ({{.URL}}) has had no visits and no deployments since {{.Since}}.
To free resources for other students, apps unused for {{.Days}} days on the
{{.Plan}} plan are stopped. Yours will be stopped on {{.StopOn}}.

To keep it running, visit it or deploy it again before then. A stopped app is
never deleted: you can start it again from the dashboard at any time.

— FodyCloud
{{end}}
{{define "app-abandon-stopped"}}Subject: Your FodyCloud app {{.App}} was stopped (unused)
Hello,

Your app {{.App}} had no visits and no deployments since {{.Since}}, so it was
stopped as announced. Nothing was deleted: start it again from the dashboard
whenever you need it.

— FodyCloud
{{end}}
{{define "app-stopped-abuse"}}Subject: Your FodyCloud app {{.App}} was stopped
Hello,

Your app {{.App}} used its full CPU allocation for a long time, twice within 24 hours,
and was stopped automatically. Free hosting cannot be used for mining or other
CPU-intensive background work.

You can start it again from the dashboard. Repeated abuse leads to suspension.

— FodyCloud
{{end}}
{{define "admin-alert"}}Subject: [FodyCloud admin] {{.Subject}}
{{.Body}}
{{end}}
`))
