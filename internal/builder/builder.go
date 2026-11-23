// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package builder turns a public Git repository into a Docker image with
// Railpack, inside a resource-limited buildx builder on an isolated network.
package builder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/fodydev/fodycloud/internal/dockercli"
	"github.com/fodydev/fodycloud/internal/repo"
	"github.com/fodydev/fodycloud/internal/stacks"
)

// Builder builds application images.
type Builder struct {
	Docker        *dockercli.Docker
	Git           *repo.Git
	Name          string  // buildx builder name
	Network       string  // network the builder container joins
	CPUs          float64 // CPU limit of the builder container
	Memory        string  // memory limit of the builder container
	Frontend      string  // Railpack BuildKit frontend image
	WorkDir       string
	MaxRepoSizeMB int64
	RailpackBin   string
}

// Spec describes one build.
type Spec struct {
	AppID        int64
	AppName      string
	DeploymentID int64
	Repo         repo.Repo
	Branch       string
	// Stack is the stack the student confirmed (for Build), or a starting
	// point for Detect (only RootDir is used).
	Stack stacks.Stack
	// Env is available to the build steps (as BuildKit secrets) and baked
	// nowhere into the image; the deployer sets it again at runtime.
	Env map[string]string
}

// ErrNoStartCommand means Railpack could not figure out how to start the app.
var ErrNoStartCommand = errors.New("could not detect how to start this app. " +
	"Change the app's stack and set a run command that starts a server listening on $PORT " +
	"(for example: gunicorn --bind 0.0.0.0:$PORT app:app, or node server.js)")

// ConfigFile is FodyCloud's Railpack config, written into the clone when a
// stack is chosen (it takes precedence over a railpack.json in the repository).
const ConfigFile = ".fodycloud-railpack.json"

// PrepareArgs returns the `railpack prepare` arguments for a build with the
// given stack options. App env vars are passed by name (values become build
// secrets); Railpack configuration variables come from the stack.
func PrepareArgs(src, planFile, infoFile string, o stacks.Options, appEnv map[string]string) []string {
	args := []string{"prepare", src, "--plan-out", planFile, "--info-out", infoFile}
	if o.RequireStart {
		// Never build an image that cannot start (it would only restart-loop).
		args = append(args, "--error-missing-start")
	}
	if o.Config != nil {
		args = append(args, "--config-file", ConfigFile)
	}
	if o.BuildCommand != "" {
		args = append(args, "--build-cmd", o.BuildCommand)
	}
	if o.StartCommand != "" {
		args = append(args, "--start-cmd", o.StartCommand)
	}
	env := BuildEnv(o, appEnv)
	for _, k := range sortedKeys(env) {
		// Only the names end up in the plan; values are passed as secrets.
		args = append(args, "--env", k+"="+env[k])
	}
	return args
}

// BuildEnv merges the stack's Railpack variables into the app's env vars
// (the stack wins). Railpack turns every --env variable into a build secret,
// so this same map must also be given to buildx (SecretArgs).
func BuildEnv(o stacks.Options, appEnv map[string]string) map[string]string {
	env := make(map[string]string, len(appEnv)+len(o.Env))
	for k, v := range appEnv {
		env[k] = v
	}
	for k, v := range o.Env {
		env[k] = v
	}
	return env
}

// SecretArgs returns the buildx --secret flags, the environment that carries
// their values (prefixed so a variable named PATH or DOCKER_HOST cannot affect
// the docker CLI itself) and the secrets-hash that invalidates cached layers
// when a value changes.
func SecretArgs(env map[string]string) (args, cliEnv []string, hash string) {
	h := sha256.New()
	for _, k := range sortedKeys(env) {
		args = append(args, "--secret", "id="+k+",env=FCSECRET_"+k)
		cliEnv = append(cliEnv, "FCSECRET_"+k+"="+env[k])
		fmt.Fprintf(h, "%s=%s\n", k, env[k])
	}
	return args, cliEnv, hex.EncodeToString(h.Sum(nil))
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// EnsureBuilder creates the dedicated buildx builder if it does not exist.
// Builds run inside its BuildKit container, never directly on the Docker host.
func (b *Builder) EnsureBuilder(ctx context.Context) error {
	if _, err := b.Docker.Run(ctx, "buildx", "inspect", b.Name); err == nil {
		return nil
	}
	cpuQuota := strconv.Itoa(int(b.CPUs * 100000))
	_, err := b.Docker.Run(ctx, "buildx", "create",
		"--name", b.Name,
		"--driver", "docker-container",
		"--driver-opt", "network="+b.Network,
		"--driver-opt", "memory="+b.Memory,
		"--driver-opt", "cpu-quota="+cpuQuota,
		"--driver-opt", "cpu-period=100000",
		"--bootstrap")
	return err
}

// checkout shallow-clones the branch into dir/src, records the commit and
// enforces the size limit.
func (b *Builder) checkout(ctx context.Context, s Spec, dir string, log io.Writer) (src, commit string, err error) {
	if err := os.RemoveAll(dir); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	src = filepath.Join(dir, "src")
	fmt.Fprintf(log, "==> Cloning %s (branch %s)\n", s.Repo.CloneURL, s.Branch)
	clone := b.Git.Command(ctx, "clone", "--depth", "1", "--single-branch", "--no-tags",
		"--branch", s.Branch, "--", s.Repo.CloneURL, src)
	clone.Stdout, clone.Stderr = log, log
	if err := clone.Run(); err != nil {
		return "", "", fmt.Errorf("git clone failed: %w", err)
	}
	out, err := b.Git.Command(ctx, "-C", src, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", "", fmt.Errorf("git rev-parse: %w", err)
	}
	commit = strings.TrimSpace(string(out))
	fmt.Fprintf(log, "    commit %s\n", commit)
	os.RemoveAll(filepath.Join(src, ".git"))
	size, err := dirSize(src)
	if err != nil {
		return "", commit, err
	}
	if max := b.MaxRepoSizeMB << 20; size > max {
		return "", commit, fmt.Errorf("repository is %d MB, the limit is %d MB", size>>20, b.MaxRepoSizeMB)
	}
	return src, commit, nil
}

func (b *Builder) prepare(ctx context.Context, dir, src string, o stacks.Options, appEnv map[string]string, log io.Writer) (planFile, infoFile, out string, err error) {
	planFile, infoFile = filepath.Join(dir, "railpack-plan.json"), filepath.Join(dir, "railpack-info.json")
	if o.Config != nil {
		if err := os.WriteFile(filepath.Join(src, ConfigFile), o.Config, 0o644); err != nil {
			return "", "", "", err
		}
	}
	var buf bytes.Buffer
	prep := exec.CommandContext(ctx, b.railpack(), PrepareArgs(src, planFile, infoFile, o, appEnv)...)
	prep.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir}
	prep.Stdout, prep.Stderr = io.MultiWriter(log, &buf), io.MultiWriter(log, &buf)
	err = prep.Run()
	return planFile, infoFile, buf.String(), err
}

// Detect analyses the repository without building it: it returns what
// Railpack recognises, mapped to the FodyCloud catalog, for the student to
// review. It starts in s.Stack.RootDir (the repository root by default); if
// nothing is found there, it tries the most likely subfolders (monorepos,
// apps in frontend/ or app/…) and suggests the first one that works. A
// repository Railpack can't plan is not an error: the detection then says
// nothing was found.
func (b *Builder) Detect(ctx context.Context, s Spec, log io.Writer) (stacks.Detection, error) {
	dir := filepath.Join(b.WorkDir, fmt.Sprintf("detect-%d", s.DeploymentID))
	defer os.RemoveAll(dir)
	src, _, err := b.checkout(ctx, s, dir, log)
	if err != nil {
		return stacks.Detection{Message: "The repository could not be fetched: " + err.Error()}, err
	}
	analyse := func(rel string) (stacks.Detection, error) {
		work, err := WorkingDir(src, rel)
		if err != nil {
			return stacks.Detection{}, err
		}
		fmt.Fprintf(log, "==> Analysing %s (Railpack)\n", folderName(rel))
		planFile, infoFile, _, _ := b.prepare(ctx, dir, work, stacks.Options{}, nil, log)
		info, _ := os.ReadFile(infoFile)
		plan, _ := os.ReadFile(planFile)
		os.Remove(infoFile)
		os.Remove(planFile)
		// os.Root: hints are read from the clone only, never through a
		// symbolic link pointing outside it.
		var files fs.FS
		if root, err := os.OpenRoot(work); err == nil {
			defer root.Close()
			files = root.FS()
		}
		return stacks.Detect(info, plan, files), nil
	}
	d, err := analyse(s.Stack.RootDir)
	if err != nil {
		d = stacks.Detection{Message: err.Error()}
		fmt.Fprintf(log, "==> %s\n", d.Message)
		return d, nil
	}
	if !d.Detected && s.Stack.RootDir == "" {
		for _, rel := range Candidates(src, 4) {
			if sub, err := analyse(rel); err == nil && sub.Detected {
				sub.Suggested.RootDir = rel
				sub.RootDir = rel
				sub.Message = fmt.Sprintf("Nothing to build at the repository root, but the folder %s/ looks like %s. Check it below, change it if it's wrong, then deploy.",
					rel, stacks.Label(*sub.Suggested))
				d = sub
				break
			}
		}
	}
	if d.Suggested != nil && s.Stack.RootDir != "" {
		d.Suggested.RootDir = s.Stack.RootDir
		d.RootDir = s.Stack.RootDir
	}
	fmt.Fprintf(log, "==> %s\n", d.Message)
	return d, nil
}

func folderName(rel string) string {
	if rel == "" {
		return "the repository root"
	}
	return "the folder " + rel + "/"
}

// WorkingDir resolves a validated relative folder inside the clone. Symlinks
// are resolved first, so a repository can't point the build outside itself.
func WorkingDir(src, rel string) (string, error) {
	if rel == "" {
		return src, nil
	}
	root, err := filepath.EvalSymlinks(src)
	if err != nil {
		return "", err
	}
	p, err := filepath.EvalSymlinks(filepath.Join(src, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("the working directory %s/ does not exist in this branch", rel)
	}
	if p != root && !strings.HasPrefix(p, root+string(filepath.Separator)) {
		return "", fmt.Errorf("the working directory %s/ points outside the repository", rel)
	}
	if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("the working directory %s/ is not a folder", rel)
	}
	return p, nil
}

// projectMarkers are files that make a folder worth analysing.
var projectMarkers = []string{"package.json", "requirements.txt", "pyproject.toml", "Pipfile", "manage.py", "index.html",
	"composer.json", "index.php", "go.mod", "pom.xml", "build.gradle", "build.gradle.kts"}

// Candidates lists up to max subfolders (two levels deep, shallowest first)
// that contain a project file, skipping dependencies and hidden folders.
func Candidates(src string, max int) []string {
	var found []string
	for depth := 1; depth <= 2 && len(found) < max; depth++ {
		var level []string
		filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck
			if err != nil || !d.IsDir() || p == src {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "venv" || name == "__pycache__" {
				return filepath.SkipDir
			}
			rel, _ := filepath.Rel(src, p)
			n := strings.Count(rel, string(filepath.Separator)) + 1
			if n > depth {
				return filepath.SkipDir
			}
			if n == depth {
				for _, m := range projectMarkers {
					if fi, err := os.Lstat(filepath.Join(p, m)); err == nil && fi.Mode().IsRegular() {
						level = append(level, filepath.ToSlash(rel))
						break
					}
				}
			}
			return nil
		})
		sort.Strings(level)
		found = append(found, level...)
	}
	if len(found) > max {
		found = found[:max]
	}
	return found
}

// Build clones the repository, plans it with Railpack using the confirmed
// stack and builds the image. It returns the image and the commit built.
func (b *Builder) Build(ctx context.Context, s Spec, log io.Writer) (image, commit string, err error) {
	dir := filepath.Join(b.WorkDir, fmt.Sprintf("deploy-%d", s.DeploymentID))
	defer os.RemoveAll(dir)
	repoDir, commit, err := b.checkout(ctx, s, dir, log)
	if err != nil {
		return "", commit, err
	}
	src, err := WorkingDir(repoDir, s.Stack.RootDir)
	if err != nil {
		return "", commit, err
	}
	if s.Stack.RootDir != "" {
		fmt.Fprintf(log, "==> Working directory: %s/\n", s.Stack.RootDir)
	}
	if s.Stack.Language == "" {
		return "", commit, errors.New("the app's stack isn't confirmed yet")
	}
	o := s.Stack.Railpack()
	fmt.Fprintf(log, "==> Preparing build (Railpack): %s / %s", s.Stack.Language, s.Stack.Framework)
	if s.Stack.Version != "" {
		fmt.Fprintf(log, " %s", s.Stack.Version)
	}
	fmt.Fprintf(log, "\n    build: %s\n    run:   %s\n", orNone(o.BuildCommand), orStatic(o.StartCommand, s.Stack.PublishDir))
	planFile, infoFile, out, err := b.prepare(ctx, dir, src, o, s.Env, log)
	if err != nil {
		writeInfoLogs(infoFile, log)
		if strings.Contains(strings.ToLower(out), "start command") {
			return "", commit, ErrNoStartCommand
		}
		return "", commit, errors.New("could not prepare the build for this repository (see build log); check the app's stack")
	}

	image = fmt.Sprintf("fodycloud/app-%s:%d", s.AppName, s.DeploymentID)
	fmt.Fprintf(log, "==> Building image %s\n", image)
	if len(s.Env) > 0 {
		fmt.Fprintf(log, "    %d environment variable(s) available to the build\n", len(s.Env))
	}
	secretArgs, secretEnv, secretsHash := SecretArgs(BuildEnv(o, s.Env))
	args := []string{"buildx", "build",
		"--builder", b.Name,
		"--progress", "plain",
		"--build-arg", "BUILDKIT_SYNTAX=" + b.Frontend,
		// Isolate Railpack mount caches per app (multi-tenant builder).
		"--build-arg", fmt.Sprintf("cache-key=app-%d", s.AppID),
		"--label", "fodycloud.app=" + s.AppName,
		"--label", fmt.Sprintf("fodycloud.deployment=%d", s.DeploymentID),
		"--build-arg", "secrets-hash=" + secretsHash,
		"--file", planFile,
		"--tag", image,
		"--load",
	}
	args = append(append(args, secretArgs...), src)
	err = b.Docker.StreamEnv(ctx, log, secretEnv, args...)
	if err != nil {
		return "", commit, fmt.Errorf("image build failed: %w", err)
	}
	return image, commit, nil
}

func orStatic(start, dir string) string {
	if start != "" {
		return start
	}
	return "serve the static files in " + dir
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func (b *Builder) railpack() string {
	if b.RailpackBin != "" {
		return b.RailpackBin
	}
	return "railpack"
}

func writeInfoLogs(path string, w io.Writer) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var info struct {
		Logs []struct {
			Level string `json:"level"`
			Msg   string `json:"msg"`
		} `json:"logs"`
	}
	if json.Unmarshal(raw, &info) == nil {
		for _, l := range info.Logs {
			fmt.Fprintf(w, "    [%s] %s\n", l.Level, l.Msg)
		}
	}
}

func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}
