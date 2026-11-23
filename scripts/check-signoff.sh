#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Checks that every commit in a range is signed off by its author (the
# Developer Certificate of Origin, see CONTRIBUTING.md): its message has a
# "Signed-off-by: Name <email>" line with the author's name and email.
#
#   scripts/check-signoff.sh [RANGE]     (default: origin/main..HEAD)
#
# Merge commits are skipped, and so are Dependabot's: a bot can't certify
# anything, and its changes are reviewed in the pull request.
set -euo pipefail

range="${1:-origin/main..HEAD}"
bad=0 count=0

while IFS= read -r sha; do
  [[ -n "$sha" ]] || continue
  count=$((count + 1))
  author="$(git log -1 --format='%an <%ae>' "$sha")"
  if [[ "$author" == "dependabot[bot] <"* ]]; then
    continue
  fi
  # %(trailers:key=…,valueonly) prints each sign-off on its own line.
  if ! git log -1 --format='%(trailers:key=Signed-off-by,valueonly)' "$sha" | grep -qxF -- "$author"; then
    echo "not signed off by its author ($author): $(git log -1 --format='%h %s' "$sha")"
    bad=$((bad + 1))
  fi
done < <(git rev-list --no-merges "$range")

if (( bad )); then
  echo
  echo "$bad of $count commit(s) lack a sign-off. Add it with 'git rebase --signoff <base>'"
  echo "(or 'git commit --amend -s' for the last commit), then push again."
  exit 1
fi
echo "All $count commit(s) are signed off."
