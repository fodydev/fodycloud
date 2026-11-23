// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package builder

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fodydev/fodycloud/internal/stacks"
)

func TestPrepareArgsStack(t *testing.T) {
	st := stacks.Stack{Language: "python", Framework: "fastapi", Version: "3.12"}
	st.Normalize() //nolint:errcheck
	args := strings.Join(PrepareArgs("/src", "/p.json", "/i.json", st.Railpack(), map[string]string{"B": "2", "A": "1"}), " ")
	for _, want := range []string{"--config-file " + ConfigFile, "--build-cmd pip install uvicorn", "--start-cmd uvicorn main:app --host 0.0.0.0 --port $PORT",
		"--env A=1 --env B=2", "--error-missing-start"} {
		if !strings.Contains(args, want) {
			t.Errorf("args %q missing %q", args, want)
		}
	}
}

func TestPrepareArgsStatic(t *testing.T) {
	st := stacks.Stack{Language: "static", Framework: "static", PublishDir: "public"}
	st.Normalize() //nolint:errcheck
	args := strings.Join(PrepareArgs("/src", "/p.json", "/i.json", st.Railpack(), nil), " ")
	if strings.Contains(args, "--error-missing-start") || strings.Contains(args, "--start-cmd") {
		t.Errorf("static sites have no start command: %q", args)
	}
	if !strings.Contains(args, "--env RAILPACK_STATIC_FILE_ROOT=public") {
		t.Errorf("publish dir not passed: %q", args)
	}
}

func TestSecretArgs(t *testing.T) {
	args, env, hash := SecretArgs(map[string]string{"PATH": "/evil", "TOKEN": "s3cret"})
	if !slices.Equal(args, []string{"--secret", "id=PATH,env=FCSECRET_PATH", "--secret", "id=TOKEN,env=FCSECRET_TOKEN"}) {
		t.Errorf("args = %v", args)
	}
	// Values travel under a prefix so they can never override the docker CLI's own PATH/DOCKER_HOST.
	if !slices.Equal(env, []string{"FCSECRET_PATH=/evil", "FCSECRET_TOKEN=s3cret"}) {
		t.Errorf("env = %v", env)
	}
	_, _, hash2 := SecretArgs(map[string]string{"PATH": "/evil", "TOKEN": "changed"})
	if hash == hash2 || len(hash) != 64 {
		t.Error("secrets-hash must change when a value changes")
	}
	if _, _, h := SecretArgs(nil); h == "" {
		t.Error("empty env must still produce a hash")
	}
}

func TestStaticEnvIsAlsoASecret(t *testing.T) {
	// Railpack declares every --env variable as a build secret: buildx must get them all.
	st := stacks.Stack{Language: "static", Framework: "static", PublishDir: "public"}
	st.Normalize() //nolint:errcheck
	o := st.Railpack()
	args, cliEnv, _ := SecretArgs(BuildEnv(o, map[string]string{"FOO": "bar", "RAILPACK_STATIC_FILE_ROOT": "student"}))
	joined := strings.Join(args, " ")
	for _, want := range []string{"id=RAILPACK_STATIC_FILE_ROOT,env=FCSECRET_RAILPACK_STATIC_FILE_ROOT", "id=FOO,env=FCSECRET_FOO"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing secret %q in %q", want, joined)
		}
	}
	if !slices.Contains(cliEnv, "FCSECRET_RAILPACK_STATIC_FILE_ROOT=public") {
		t.Errorf("the stack's publish dir must win: %v", cliEnv)
	}
	prep := strings.Join(PrepareArgs("/s", "/p", "/i", o, map[string]string{"RAILPACK_STATIC_FILE_ROOT": "student"}), " ")
	if strings.Count(prep, "RAILPACK_STATIC_FILE_ROOT") != 1 {
		t.Errorf("duplicate --env: %q", prep)
	}
}

func TestWorkingDir(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "kaat.cm", "src"), 0o755)  //nolint:errcheck
	os.WriteFile(filepath.Join(src, "README.md"), nil, 0o644) //nolint:errcheck
	os.Symlink("/etc", filepath.Join(src, "escape"))          //nolint:errcheck
	os.Symlink("kaat.cm", filepath.Join(src, "alias"))        //nolint:errcheck
	if p, err := WorkingDir(src, "kaat.cm"); err != nil || !strings.HasSuffix(p, "kaat.cm") {
		t.Errorf("subfolder: %q %v", p, err)
	}
	if _, err := WorkingDir(src, "alias"); err != nil {
		t.Errorf("a symlink inside the repository is fine: %v", err)
	}
	for _, bad := range []string{"escape", "missing", "README.md"} {
		if _, err := WorkingDir(src, bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestCandidates(t *testing.T) {
	src := t.TempDir()
	touch := func(p string) {
		os.MkdirAll(filepath.Dir(filepath.Join(src, p)), 0o755)  //nolint:errcheck
		os.WriteFile(filepath.Join(src, p), []byte("{}"), 0o644) //nolint:errcheck
	}
	// kaat's layout: the app is in kaat.cm/, docs elsewhere.
	touch("README.md")
	touch("branding/README.md")
	touch("kaat.cm/package.json")
	touch("kaat.cm/node_modules/x/package.json")
	touch(".github/package.json")
	touch("apps/api/requirements.txt")
	touch("apps/web/index.html")
	got := Candidates(src, 4)
	want := []string{"kaat.cm", "apps/api", "apps/web"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if len(Candidates(src, 1)) != 1 {
		t.Error("max not applied")
	}
}

func TestCandidatesNewStacks(t *testing.T) {
	src := t.TempDir()
	for _, p := range []string{"api/go.mod", "site/composer.json", "complete/pom.xml", "legacy/build.gradle.kts", "vendor/x/composer.json"} {
		os.MkdirAll(filepath.Dir(filepath.Join(src, p)), 0o755) //nolint:errcheck
		os.WriteFile(filepath.Join(src, p), []byte("x"), 0o644) //nolint:errcheck
	}
	got := Candidates(src, 10)
	slices.Sort(got)
	if want := []string{"api", "complete", "legacy", "site"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
