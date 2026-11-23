// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fodydev/fodycloud/internal/auth"
)

// Live build logs: the worker's output is pushed to followers (the
// dashboard's and the API's server-sent event streams) as it is written.
//
// Offsets count characters since the start of the deployment
// (deployments.log_size), so they stay valid after the stored log is trimmed
// to its last MaxLogBytes. A follower resumes from the last offset it saw.

// MaxLogStreams is how many build-log streams a student can keep open.
const MaxLogStreams = 5

// ErrTooManyStreams refuses a stream beyond MaxLogStreams (HTTP 429).
var ErrTooManyStreams = errors.New("too many open log streams; close a log page first")

// LogEvent is one event of a followed build log. Exactly one of Text (with
// End, the offset after it), Status or Done is meaningful.
type LogEvent struct {
	Text   string
	End    int64
	Status string // the deployment's new status
	Done   bool   // the deployment is finished; the stream ends
}

// logHub fans log chunks and status changes out to the followers of each
// deployment. It lives in memory: the worker and the web server run in the
// same process.
type logHub struct {
	mu      sync.Mutex
	subs    map[int64]map[chan LogEvent]struct{}
	perUser map[int64]int
}

func (s *Service) hub() *logHub {
	s.hubOnce.Do(func() {
		s.logs = &logHub{subs: map[int64]map[chan LogEvent]struct{}{}, perUser: map[int64]int{}}
	})
	return s.logs
}

// subscribe registers a follower of a deployment for a user, or refuses it
// beyond MaxLogStreams.
func (h *logHub) subscribe(userID, deploymentID int64) (chan LogEvent, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.perUser[userID] >= MaxLogStreams {
		return nil, nil, ErrTooManyStreams
	}
	h.perUser[userID]++
	ch := make(chan LogEvent, 64)
	if h.subs[deploymentID] == nil {
		h.subs[deploymentID] = map[chan LogEvent]struct{}{}
	}
	h.subs[deploymentID][ch] = struct{}{}
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.perUser[userID]--
			if h.perUser[userID] <= 0 {
				delete(h.perUser, userID)
			}
			if _, ok := h.subs[deploymentID][ch]; ok {
				delete(h.subs[deploymentID], ch)
				close(ch)
			}
			if len(h.subs[deploymentID]) == 0 {
				delete(h.subs, deploymentID)
			}
		})
	}, nil
}

// publish sends an event to every follower of a deployment. A follower too
// slow to keep up is dropped (its channel closes); its client reconnects and
// resumes from its last offset.
func (h *logHub) publish(deploymentID int64, ev LogEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[deploymentID] {
		select {
		case ch <- ev:
		default:
			delete(h.subs[deploymentID], ch)
			close(ch)
		}
	}
	if len(h.subs[deploymentID]) == 0 {
		delete(h.subs, deploymentID)
	}
}

// catchUp returns the part of a stored log after offset from, and the
// offset at its end. log holds the last characters of a log of size
// characters in all. When from is older than what is kept, the whole stored
// log is returned and trimmed is true.
func catchUp(log string, size, from int64) (text string, end int64, trimmed bool) {
	r := []rune(log)
	start := size - int64(len(r)) // offset of r[0]
	switch {
	case from >= size:
		return "", size, false
	case from < start:
		return log, size, from > 0 || start > 0
	default:
		return string(r[from-start:]), size, false
	}
}

// finished reports whether a deployment status is final.
func finished(status string) bool {
	switch status {
	case "queued", "building", "deploying":
		return false
	}
	return true
}

// FollowBuildLog streams a deployment's log from offset from (0 = the
// start): first what is already stored, then new output as the worker writes
// it, and status changes. The channel closes after a Done event, or when ctx
// ends.
func (s *Service) FollowBuildLog(ctx context.Context, u *auth.User, name string, id, from int64) (<-chan LogEvent, error) {
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	// Subscribe before reading the stored log, so nothing written in between
	// is missed (overlap is skipped by offset).
	sub, unsubscribe, err := s.hub().subscribe(u.ID, id)
	if err != nil {
		return nil, err
	}
	var log, status string
	var size int64
	err = s.DB.QueryRow(ctx, `SELECT log, log_size, status FROM deployments WHERE app_id=$1 AND id=$2`, a.ID, id).Scan(&log, &size, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	if err != nil {
		unsubscribe()
		return nil, err
	}

	out := make(chan LogEvent, 16)
	go func() {
		defer close(out)
		defer unsubscribe()
		send := func(ev LogEvent) bool {
			select {
			case out <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}
		text, sent, trimmed := catchUp(log, size, from)
		if trimmed {
			text = "[… earlier output trimmed …]\n" + text
		}
		if text != "" && !send(LogEvent{Text: text, End: sent}) {
			return
		}
		if !send(LogEvent{Status: status}) {
			return
		}
		if finished(status) {
			send(LogEvent{Done: true, Status: status})
			return
		}
		// Status changes are pushed by the worker; this also catches the
		// ones made elsewhere (cancellation, deletion, a restart).
		check := time.NewTicker(3 * time.Second)
		defer check.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok { // too slow: end the stream, the client resumes
					return
				}
				if ev.Text != "" {
					if ev.End <= sent {
						continue // already sent with the stored log
					}
					// Keep only the part after what was sent.
					r := []rune(ev.Text)
					if skip := sent - (ev.End - int64(len(r))); skip > 0 {
						ev.Text = string(r[skip:])
					}
					sent = ev.End
					if !send(ev) {
						return
					}
					continue
				}
				if ev.Status != "" && ev.Status != status {
					status = ev.Status
					if !send(LogEvent{Status: status}) {
						return
					}
				}
				if finished(status) {
					s.flushTail(ctx, a.ID, id, sent, send)
					send(LogEvent{Done: true, Status: status})
					return
				}
			case <-check.C:
				var now string
				if err := s.DB.QueryRow(ctx, `SELECT status FROM deployments WHERE id=$1`, id).Scan(&now); err != nil {
					return
				}
				if now != status {
					status = now
					if !send(LogEvent{Status: status}) {
						return
					}
				}
				if finished(status) {
					s.flushTail(ctx, a.ID, id, sent, send)
					send(LogEvent{Done: true, Status: status})
					return
				}
			}
		}
	}()
	return out, nil
}

// flushTail sends what was stored after offset sent, before a stream ends
// (the worker flushes its last output just before finishing the job).
func (s *Service) flushTail(ctx context.Context, appID, id, sent int64, send func(LogEvent) bool) {
	var log string
	var size int64
	if err := s.DB.QueryRow(ctx, `SELECT log, log_size FROM deployments WHERE app_id=$1 AND id=$2`, appID, id).Scan(&log, &size); err != nil {
		return
	}
	if text, end, _ := catchUp(log, size, sent); text != "" {
		send(LogEvent{Text: text, End: end})
	}
}
