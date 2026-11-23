// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package dockercli is a small wrapper around the docker CLI. The control
// plane already needs the CLI for `docker buildx`, so it is used for
// everything instead of pulling in the (large, frequently re-versioned) SDK.
package dockercli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Docker runs docker CLI commands.
type Docker struct {
	Bin string
}

func (d *Docker) bin() string {
	if d.Bin == "" {
		return "docker"
	}
	return d.Bin
}

// Run executes a docker command and returns trimmed stdout.
func (d *Docker) Run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, d.bin(), args...)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", firstArgs(args), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// RunInput is Run with input on the command's stdin.
func (d *Docker) RunInput(ctx context.Context, input string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, d.bin(), args...)
	var out, stderr bytes.Buffer
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", firstArgs(args), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// Combined executes a docker command and returns stdout and stderr interleaved
// (used for `docker logs`, which replays the container's stderr on stderr).
func (d *Docker) Combined(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, d.bin(), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", firstArgs(args), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Stream executes a docker command writing stdout and stderr to w.
func (d *Docker) Stream(ctx context.Context, w io.Writer, args ...string) error {
	return d.StreamEnv(ctx, w, nil, args...)
}

// StreamEnv is Stream with extra environment variables for the docker CLI
// process (used to hand build secrets to `docker buildx build --secret`).
func (d *Docker) StreamEnv(ctx context.Context, w io.Writer, extraEnv []string, args ...string) error {
	cmd := exec.CommandContext(ctx, d.bin(), args...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s: %w", firstArgs(args), err)
	}
	return nil
}

// Lines splits command output into non-empty lines.
func Lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func firstArgs(args []string) string {
	if len(args) > 2 {
		args = args[:2]
	}
	return strings.Join(args, " ")
}
