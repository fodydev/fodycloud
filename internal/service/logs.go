// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/fodydev/fodycloud/internal/auth"
)

// MaxLogBytes caps logs returned to clients.
const MaxLogBytes = 64 << 10

// BuildLog returns a deployment's log (its last MaxLogBytes, without
// terminal escapes) and the offset at its end, from which a live stream
// continues it (FollowBuildLog).
func (s *Service) BuildLog(ctx context.Context, u *auth.User, name string, id int64) (string, int64, error) {
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return "", 0, err
	}
	var log string
	var size int64
	err = s.DB.QueryRow(ctx, `SELECT log, log_size FROM deployments WHERE app_id=$1 AND id=$2`, a.ID, id).Scan(&log, &size)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	return tailBytes(log, MaxLogBytes), size, err
}

// RuntimeLogs returns the last lines printed by the running app.
func (s *Service) RuntimeLogs(ctx context.Context, u *auth.User, name string, tail int) (string, error) {
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return "", err
	}
	if tail <= 0 || tail > 1000 {
		tail = 200
	}
	out, err := s.Deployer.Logs(ctx, a.Name, tail)
	return tailBytes(out, MaxLogBytes), err
}

// tailBytes keeps the end of a log, without terminal escape sequences
// (colours from npm, Vite, BuildKit…), which would show up as garbage.
func tailBytes(s string, n int) string {
	s = StripANSI(s)
	if len(s) <= n {
		return s
	}
	return "…(truncated)…\n" + s[len(s)-n:]
}

// ansiSeq matches CSI sequences (colours, cursor moves) and OSC sequences
// (titles, hyperlinks) emitted by build tools.
var ansiSeq = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[@-Z\\-_])`)

// StripANSI removes terminal escape sequences from a log.
func StripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	return ansiSeq.ReplaceAllString(s, "")
}
