#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Runs every static check of the repository; run it before each commit.
#
#   scripts/check.sh          (from anywhere in the repository)
#
# Needs Go and OpenTofu. Linters are pinned and fetched on demand:
# staticcheck and go-licenses through `go run` (MIT, Apache-2.0); shellcheck,
# reuse (GPL-3.0) and actionlint (MIT) from a local binary or their container
# image. The VM tests (qemu/) are separate: see CONTRIBUTING.md.
set -uo pipefail

STATICCHECK_VERSION="2026.2.2"
GO_LICENSES_VERSION="v2.0.1"
SHELLCHECK_IMAGE="docker.io/koalaman/shellcheck:v0.10.0"
REUSE_IMAGE="docker.io/fsfe/reuse:6.2.0"
ACTIONLINT_IMAGE="docker.io/rhysd/actionlint:1.7.7"

# Licenses accepted for Go dependencies: OSI-approved and compatible with the
# AGPL-3.0 that FodyCloud is distributed under. Add one only after checking both.
GO_ALLOWED_LICENSES="MIT,BSD-2-Clause,BSD-3-Clause,Apache-2.0,ISC,MPL-2.0,0BSD"

cd "$(git rev-parse --show-toplevel)" || exit 1
failed=()

# step NAME CMD…: runs a check and records a failure without stopping.
step() {
  local name="$1"; shift
  printf '==> %s\n' "$name"
  if ! "$@"; then
    failed+=("$name")
  fi
}

gofmt_clean() {
  local files
  files="$(gofmt -l .)"
  [[ -z "$files" ]] || { echo "not gofmt-formatted:"; echo "$files"; return 1; }
}

staticcheck() { go run "honnef.co/go/tools/cmd/staticcheck@$STATICCHECK_VERSION" ./...; }

# go_licenses: every Go package linked into the binary has an allowed license.
# FodyCloud's own packages (AGPL-3.0) are skipped; the standard library is
# skipped by go-licenses itself. Its warnings about assembly files it can't
# inspect (and the file list after each) are filtered out; errors still show.
go_licenses() {
  local status
  go run "github.com/google/go-licenses/v2@$GO_LICENSES_VERSION" check ./... \
    --allowed_licenses="$GO_ALLOWED_LICENSES" --ignore github.com/fodydev/fodycloud \
    2>&1 | { grep -vE '^W[0-9]{4} .*contains non-Go code|^/.*\.s$' || true; }
  status="${PIPESTATUS[0]}"
  return "$status"
}

# tool NAME IMAGE ARGS…: runs NAME from a local binary, else from its pinned
# container image (Docker or Podman) with the repository mounted read-only.
# The container's user may not own the checkout (on GitHub runners it doesn't),
# so git there would refuse the repository as "dubious ownership" and tools like
# reuse would stop honouring .gitignore (and lint terraform/.terraform/, etc.):
# mark /mnt as a safe directory.
tool() {
  local name="$1" image="$2"; shift 2
  local git_safe=(-e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0=/mnt)
  if command -v "$name" >/dev/null; then
    "$name" "$@"
  elif command -v docker >/dev/null; then
    docker run --rm "${git_safe[@]}" -v "$PWD:/mnt:ro" -w /mnt "$image" "$@"
  elif command -v podman >/dev/null; then
    podman run --rm "${git_safe[@]}" -v "$PWD:/mnt:ro,Z" -w /mnt "$image" "$@"
  else
    echo "$name not found: install it, or Docker/Podman to run $image"
    return 1
  fi
}

shellcheck_all() {
  local scripts
  # Tracked and new scripts, minus files deleted or renamed but not yet committed.
  mapfile -t scripts < <(git ls-files --cached --others --exclude-standard '*.sh' | while IFS= read -r f; do [[ -f "$f" ]] && echo "$f"; done)
  tool shellcheck "$SHELLCHECK_IMAGE" -x "${scripts[@]}"
}

# reuse_lint: every file states its copyright and license (SPDX header or
# REUSE.toml), and every license used has its text in LICENSES/.
reuse_lint() { tool reuse "$REUSE_IMAGE" lint --quiet || { tool reuse "$REUSE_IMAGE" lint | head -40; return 1; }; }

tofu_checks() {
  tofu fmt -check -recursive . &&
    tofu -chdir=terraform init -backend=false -input=false >/dev/null &&
    tofu -chdir=terraform validate
}

step "gofmt" gofmt_clean
step "go vet" go vet ./...
step "go test" go test ./...
step "staticcheck $STATICCHECK_VERSION" staticcheck
step "shellcheck" shellcheck_all
step "tofu fmt + validate" tofu_checks
step "actionlint (GitHub workflows)" tool actionlint "$ACTIONLINT_IMAGE" -color=false
step "licenses: REUSE" reuse_lint
step "licenses: Go dependencies" go_licenses

if (( ${#failed[@]} )); then
  printf '\nFAILED: %s\n' "${failed[*]}"
  exit 1
fi
printf '\nAll checks passed.\n'
