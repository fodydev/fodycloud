// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package repo validates student repositories: only public repositories on
// GitHub, GitLab or Codeberg, fetched over HTTPS without any credentials.
package repo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// AllowedHosts are the supported Git providers.
var AllowedHosts = map[string]string{
	"github.com":   "GitHub",
	"gitlab.com":   "GitLab",
	"codeberg.org": "Codeberg",
}

var (
	ErrUnsupported   = errors.New("only public repositories on github.com, gitlab.com or codeberg.org are supported")
	ErrNotPublic     = errors.New("repository not found or not public (private repositories are not supported)")
	ErrBranchMissing = errors.New("branch not found in repository")
)

var (
	segment   = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	branchRef = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)
)

// Repo is a parsed repository reference.
type Repo struct {
	Host     string
	Path     string // owner/name (GitLab: group/subgroup/name)
	CloneURL string // canonical https://host/path.git
}

// Parse validates a repository URL and returns its canonical form.
func Parse(raw string) (Repo, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return Repo{}, ErrUnsupported
	}
	host := strings.ToLower(u.Hostname())
	if _, ok := AllowedHosts[host]; !ok {
		return Repo{}, ErrUnsupported
	}
	p := strings.Trim(u.Path, "/")
	p = strings.TrimSuffix(p, ".git")
	parts := strings.Split(p, "/")
	maxParts := 2
	if host == "gitlab.com" {
		maxParts = 20 // GitLab supports nested groups
	}
	if len(parts) < 2 || len(parts) > maxParts {
		return Repo{}, ErrUnsupported
	}
	for _, s := range parts {
		if !segment.MatchString(s) || s == "." || s == ".." || strings.HasPrefix(s, "-") {
			return Repo{}, ErrUnsupported
		}
	}
	p = strings.Join(parts, "/")
	return Repo{Host: host, Path: p, CloneURL: "https://" + host + "/" + p + ".git"}, nil
}

// ValidateBranch checks a branch name.
func ValidateBranch(b string) error {
	if !branchRef.MatchString(b) || strings.Contains(b, "..") || strings.HasPrefix(b, "-") || strings.HasPrefix(b, "/") {
		return fmt.Errorf("invalid branch name")
	}
	return nil
}

// Git runs git with a hardened, credential-free configuration.
type Git struct {
	Bin     string
	Timeout time.Duration
}

// SafeArgs are prepended to every git invocation: HTTPS only, no credential
// helpers, no prompts, no submodules, no hooks.
var SafeArgs = []string{
	"-c", "protocol.allow=never",
	"-c", "protocol.https.allow=always",
	"-c", "credential.helper=",
	"-c", "core.askPass=",
	"-c", "core.hooksPath=/dev/null",
	"-c", "submodule.recurse=false",
	"-c", "http.followRedirects=initial",
}

// Env is the environment used for git commands.
func (g *Git) Env() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=/nonexistent",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=/bin/false",
		"SSH_ASKPASS=/bin/false",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
	}
}

// Command builds a hardened git command.
func (g *Git) Command(ctx context.Context, args ...string) *exec.Cmd {
	bin := g.Bin
	if bin == "" {
		bin = "git"
	}
	cmd := exec.CommandContext(ctx, bin, append(append([]string{}, SafeArgs...), args...)...)
	cmd.Env = g.Env()
	return cmd
}

// ResolveBranch checks that the repository is publicly readable and returns
// the commit SHA at the head of the branch.
func (g *Git) ResolveBranch(ctx context.Context, r Repo, branch string) (string, error) {
	if err := ValidateBranch(branch); err != nil {
		return "", err
	}
	timeout := g.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := g.Command(ctx, "ls-remote", "--", r.CloneURL, "refs/heads/"+branch)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("timed out contacting %s", r.Host)
		}
		msg := strings.ToLower(stderr.String())
		switch {
		case strings.Contains(msg, "could not resolve host"), strings.Contains(msg, "unable to access") && !strings.Contains(msg, "403") && !strings.Contains(msg, "401"):
			return "", fmt.Errorf("could not reach %s, try again later", r.Host)
		default:
			// Private and missing repositories both end up asking for credentials.
			return "", ErrNotPublic
		}
	}
	fields := strings.Fields(out.String())
	if len(fields) < 2 {
		return "", ErrBranchMissing
	}
	return fields[0], nil
}
