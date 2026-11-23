// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package ratelimit provides in-memory rate limiting for a single control
// plane node: token buckets keyed by client IP, token or user, and an account
// lockout after repeated login failures.
package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limiter is a set of token buckets sharing one rate.
type Limiter struct {
	rate  float64 // tokens per second
	burst float64
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
	sweep   time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New allows `events` per `per`, with bursts up to `events`.
func New(events int, per time.Duration) *Limiter {
	return &Limiter{rate: float64(events) / per.Seconds(), burst: float64(events), now: time.Now, buckets: map[string]*bucket{}}
}

// Allow consumes one token for key. When refused it returns how long to wait.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.gc(now)
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
	return false, max(wait, time.Second)
}

// gc drops buckets that are full again (idle keys), at most once a minute.
func (l *Limiter) gc(now time.Time) {
	if now.Sub(l.sweep) < time.Minute {
		return
	}
	l.sweep = now
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*l.rate >= l.burst {
			delete(l.buckets, k)
		}
	}
}

// Lockout locks a key (an account) after too many failures in a window.
type Lockout struct {
	max    int
	window time.Duration
	now    func() time.Time

	mu       sync.Mutex
	failures map[string][]time.Time
}

// NewLockout locks a key after max failures within window.
func NewLockout(max int, window time.Duration) *Lockout {
	return &Lockout{max: max, window: window, now: time.Now, failures: map[string][]time.Time{}}
}

func (l *Lockout) recent(key string, now time.Time) []time.Time {
	var keep []time.Time
	for _, t := range l.failures[key] {
		if now.Sub(t) < l.window {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(l.failures, key)
	} else {
		l.failures[key] = keep
	}
	return keep
}

// Locked reports whether key is locked and for how long.
func (l *Lockout) Locked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	f := l.recent(key, now)
	if len(f) < l.max {
		return false, 0
	}
	return true, l.window - now.Sub(f[len(f)-l.max])
}

// Fail records a failure for key.
func (l *Lockout) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.failures[key] = append(l.recent(key, now), now)
}

// Reset clears failures after a success.
func (l *Lockout) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

// ClientIP returns the client address. The control plane is only reachable
// through Traefik, which appends the real peer to X-Forwarded-For, so the
// LAST entry is trustworthy (earlier ones can be forged by the client).
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(ip) != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// TooMany writes a 429 response with Retry-After.
func TooMany(w http.ResponseWriter, wait time.Duration, msg string) {
	w.Header().Set("Retry-After", itoa(int(wait.Seconds()+0.999)))
	http.Error(w, msg, http.StatusTooManyRequests)
}

func itoa(n int) string {
	if n <= 0 {
		return "1"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
