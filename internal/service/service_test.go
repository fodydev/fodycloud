// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"strings"
	"testing"
	"time"
)

func TestValidateAppName(t *testing.T) {
	for _, n := range []string{"abc", "my-app", "portfolio2026", "a1-b2-c3"} {
		if err := ValidateAppName(n); err != nil {
			t.Errorf("ValidateAppName(%q) = %v", n, err)
		}
	}
	for _, n := range []string{"", "ab", "1app", "-app", "app-", "My-App", "a--b", "app_x", "admin", "mcp", "dashboard",
		"waytoolongnamewaytoolongnamewaytoo"} {
		if err := ValidateAppName(n); err == nil {
			t.Errorf("ValidateAppName(%q) accepted", n)
		}
	}
}

func TestMask(t *testing.T) {
	if got := mask("abc"); got != "****" {
		t.Errorf("mask(short) = %q", got)
	}
	if got := mask("supersecretvalue1234567890"); got != "su************" {
		t.Errorf("mask(long) = %q", got)
	}
}

func TestDomainLimit(t *testing.T) {
	if err := domainLimit("student", 5, 4); err != nil {
		t.Errorf("4 of 5: %v", err)
	}
	err := domainLimit("student", 5, 5)
	if err == nil || err.Error() != "the student plan allows 5 custom domains per app; remove one first" {
		t.Errorf("5 of 5: %v", err)
	}
}

func TestCatchUp(t *testing.T) {
	cases := []struct {
		log     string
		size    int64
		from    int64
		text    string
		end     int64
		trimmed bool
	}{
		{"hello", 5, 0, "hello", 5, false},
		{"hello", 5, 2, "llo", 5, false},
		{"hello", 5, 5, "", 5, false},
		{"hello", 5, 9, "", 5, false},
		{"llo", 5, 0, "llo", 5, true}, // the first 2 characters were trimmed
		{"llo", 5, 1, "llo", 5, true},
		{"llo", 5, 3, "lo", 5, false},
		{"héllo", 5, 1, "éllo", 5, false}, // offsets count characters
	}
	for _, c := range cases {
		text, end, trimmed := catchUp(c.log, c.size, c.from)
		if text != c.text || end != c.end || trimmed != c.trimmed {
			t.Errorf("catchUp(%q, %d, %d) = %q, %d, %v; want %q, %d, %v", c.log, c.size, c.from, text, end, trimmed, c.text, c.end, c.trimmed)
		}
	}
}

func TestLogHub(t *testing.T) {
	h := &logHub{subs: map[int64]map[chan LogEvent]struct{}{}, perUser: map[int64]int{}}
	ch, stop, err := h.subscribe(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	h.publish(10, LogEvent{Text: "a", End: 1})
	h.publish(11, LogEvent{Text: "other deployment", End: 16})
	if ev := <-ch; ev.Text != "a" {
		t.Fatalf("got %+v", ev)
	}
	// Per-user limit.
	var stops []func()
	for i := 1; i < MaxLogStreams; i++ {
		_, s, err := h.subscribe(1, 10)
		if err != nil {
			t.Fatalf("stream %d: %v", i+1, err)
		}
		stops = append(stops, s)
	}
	if _, _, err := h.subscribe(1, 10); err != ErrTooManyStreams {
		t.Fatalf("over the limit: %v", err)
	}
	if _, s, err := h.subscribe(2, 10); err != nil {
		t.Fatalf("another user: %v", err)
	} else {
		s()
	}
	stop()
	stop() // safe twice
	if _, ok := <-ch; ok {
		t.Fatal("channel not closed after unsubscribe")
	}
	// A slow follower is dropped.
	slow, s2, _ := h.subscribe(3, 20)
	for i := 0; i < cap(slow)+1; i++ {
		h.publish(20, LogEvent{Text: "x", End: int64(i + 1)})
	}
	n := 0
	for range slow {
		n++
	}
	if n != cap(slow) {
		t.Fatalf("slow follower got %d events before being dropped, want %d", n, cap(slow))
	}
	s2()
	for _, s := range stops {
		s()
	}
	if len(h.perUser) != 0 || len(h.subs) != 0 {
		t.Fatalf("leftovers: %v %v", h.perUser, h.subs)
	}
}

func TestFormatDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0: "0s", 9 * time.Second: "9s", 1500 * time.Millisecond: "2s", 65 * time.Second: "1m 05s",
		59*time.Minute + 59*time.Second: "59m 59s", 62 * time.Minute: "1h 02m",
	} {
		if got := FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%s) = %q, want %q", d, got, want)
		}
	}
}

func TestCleanHealthPath(t *testing.T) {
	for in, want := range map[string]string{"": "", " /healthz ": "/healthz", "/": "/", "/api/health?full=1": "/api/health?full=1"} {
		if got, err := CleanHealthPath(in); err != nil || got != want {
			t.Errorf("CleanHealthPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"healthz", "/a b", "/x\r\nHost: evil", "http://x/", "/" + strings.Repeat("a", 200)} {
		if _, err := CleanHealthPath(bad); err == nil {
			t.Errorf("CleanHealthPath(%q) accepted", bad)
		}
	}
}

func TestDBState(t *testing.T) {
	const limit = 100
	cases := []struct {
		size                 int64
		wasReadOnly, blocked bool
		readOnly, block      bool
	}{
		{90, false, false, false, false},
		{100, false, false, false, false},
		{101, false, false, true, false}, // full: read-only
		{300, false, false, true, false}, // a bulk import: read-only first, not blocked
		{120, true, false, true, false},  // read-only, still within 120%
		{121, true, false, true, true},   // grew while read-only: blocked
		{110, true, true, true, true},    // blocked until it fits the limit
		{100, true, true, false, false},  // fits again (data deleted, or a bigger plan)
	}
	for _, c := range cases {
		ro, b := dbState(c.size, limit, c.wasReadOnly, c.blocked)
		if ro != c.readOnly || b != c.block {
			t.Errorf("dbState(%d, %d, %v, %v) = %v, %v; want %v, %v", c.size, limit, c.wasReadOnly, c.blocked, ro, b, c.readOnly, c.block)
		}
	}
}

func TestReservedEnv(t *testing.T) {
	for _, k := range []string{"DATABASE_URL", "REDIS_URL", "PGPASSWORD", "FODYCLOUD_DATA_DIR"} {
		v := "x"
		if err := ValidateEnv(map[string]*string{k: &v}); err == nil || !strings.Contains(err.Error(), "set by FodyCloud") {
			t.Errorf("%s: %v", k, err)
		}
	}
}

func TestValkeyURLAndSizes(t *testing.T) {
	if got := valkeyURL("abc"); got != "redis://default:abc@valkey:6379/0" {
		t.Errorf("valkey url: %s", got)
	}
	for n, want := range map[int64]string{512 << 10: "512 KB", 20 << 20: "20 MB", 3 << 29: "1.5 GB"} {
		if got := HumanBytes(n); got != want {
			t.Errorf("HumanBytes(%d) = %s, want %s", n, got, want)
		}
	}
}
