#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Creates or updates the repository's issue labels (used by the issue forms in
# .github/ISSUE_TEMPLATE/ and CONTRIBUTING.md). Idempotent.
#
#   scripts/github-labels.sh [OWNER/REPO]     (default: fodydev/fodycloud)
#
# Needs the GitHub CLI (gh, MIT), signed in with admin rights on the repository.
set -euo pipefail

repo="${1:-fodydev/fodycloud}"

# name|color|description
labels=(
  "bug|d73a4a|Something doesn't work as documented"
  "enhancement|a2eeef|A new feature or an improvement"
  "stack-request|5319e7|A new language, framework or version"
  "docs|0075ca|User, developer, admin or operations documentation"
  "security|b60205|Hardening (vulnerabilities are reported privately: SECURITY.md)"
  "infrastructure|c5def5|OpenTofu, server scripts, the test VM"
  "triage|ededed|Needs a first look by a maintainer"
  "good first issue|7057ff|Small and well described: a good start"
  "help wanted|008672|Maintainers would welcome a contribution"
  "question|d876e3|A question rather than a change"
  "duplicate|cfd3d7|Already reported"
  "wontfix|ffffff|Out of scope for FodyCloud"
)

for entry in "${labels[@]}"; do
  IFS='|' read -r name color description <<<"$entry"
  gh label create "$name" --repo "$repo" --color "$color" --description "$description" --force
done
