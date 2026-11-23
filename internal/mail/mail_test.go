// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package mail

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestMessage(t *testing.T) {
	msg := string(Message("FodyCloud <no-reply@fc.cm>", "a@b.cm", "Héllo", "line1\nline2", time.Unix(0, 0).UTC()))
	for _, want := range []string{"From: FodyCloud <no-reply@fc.cm>\r\n", "To: a@b.cm\r\n", "Subject: =?utf-8?q?H=C3=A9llo?=\r\n",
		"Content-Type: text/plain; charset=utf-8\r\n", "\r\n\r\nline1\r\nline2"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestTemplatesRender(t *testing.T) {
	data := map[string]any{"URL": "https://x/verify?t=1", "Plan": "student", "When": "in 7 days", "Until": "2027-01-01",
		"Reason": "spam", "App": "blog", "Subject": "s", "Body": "b"}
	for _, name := range []string{"verify", "reset", "plan-expiring", "plan-expired", "banned", "app-stopped-abuse", "admin-alert"} {
		var sb strings.Builder
		if err := templates.ExecuteTemplate(&sb, name, data); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.HasPrefix(strings.TrimLeft(sb.String(), "\n"), "Subject: ") {
			t.Errorf("%s: first line must be the subject", name)
		}
	}
}

func TestQueueRejectsHeaderInjection(t *testing.T) {
	m := &Mailer{Cfg: Config{Host: "smtp.example"}, Log: slog.Default()}
	if err := m.Queue(context.Background(), "a@b.cm\r\nBcc: x@y", "s", "b"); err == nil {
		t.Fatal("header injection accepted")
	}
}

func TestBareAddress(t *testing.T) {
	if got := bareAddress("FodyCloud <no-reply@fc.cm>"); got != "no-reply@fc.cm" {
		t.Errorf("got %q", got)
	}
}
