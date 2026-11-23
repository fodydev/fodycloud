// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package ratelimit

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(3, time.Minute)
	l.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("request %d refused within burst", i)
		}
	}
	ok, wait := l.Allow("a")
	if ok || wait < time.Second {
		t.Fatalf("4th request allowed=%v wait=%v", ok, wait)
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("keys must be independent")
	}
	now = now.Add(20 * time.Second) // 3/min = 1 token per 20s
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("token not refilled")
	}
}

func TestLockout(t *testing.T) {
	now := time.Unix(0, 0)
	l := NewLockout(3, 15*time.Minute)
	l.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		if locked, _ := l.Locked("u"); locked {
			t.Fatal("locked too early")
		}
		l.Fail("u")
	}
	if locked, wait := l.Locked("u"); !locked || wait != 15*time.Minute {
		t.Fatalf("expected lock for 15m, got %v %v", locked, wait)
	}
	now = now.Add(15 * time.Minute)
	if locked, _ := l.Locked("u"); locked {
		t.Fatal("lock must expire")
	}
	l.Fail("u")
	l.Reset("u")
	if locked, _ := l.Locked("u"); locked {
		t.Fatal("reset must clear failures")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.20.1.3:5000"
	if ip := ClientIP(r); ip != "172.20.1.3" {
		t.Errorf("no XFF: %s", ip)
	}
	r.Header.Set("X-Forwarded-For", "192.0.2.66, 198.51.100.7") // client forged the first hop
	if ip := ClientIP(r); ip != "198.51.100.7" {
		t.Errorf("must use the last hop, got %s", ip)
	}
	r.Header.Set("X-Forwarded-For", "garbage")
	if ip := ClientIP(r); ip != "172.20.1.3" {
		t.Errorf("invalid XFF must fall back, got %s", ip)
	}
}
