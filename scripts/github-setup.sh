#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Applies the GitHub repository's settings: merge options, security features,
# the protection of main, and the labels. Run it once when the repository is
# created, and again after changing it; it is idempotent.
#
#   scripts/github-setup.sh [OWNER/REPO]     (default: fodydev/fodycloud)
#
# Needs the GitHub CLI (gh, MIT) signed in with admin rights on the repository,
# and jq. The required checks are the job names of .github/workflows/check.yml.
set -euo pipefail

repo="${1:-fodydev/fodycloud}"
cd "$(git rev-parse --show-toplevel)"

echo ">> repository options"
# Squash or rebase merges, branches deleted after merging, and a sign-off
# required for commits made in the web interface (DCO, CONTRIBUTING.md).
gh api -X PATCH "repos/$repo" --silent \
  -F has_wiki=false -F has_projects=false -F has_discussions=false \
  -F allow_squash_merge=true -F allow_rebase_merge=true -F allow_merge_commit=false \
  -F delete_branch_on_merge=true -F web_commit_signoff_required=true \
  -f homepage="https://fodydev.org" \
  -f description="Cloud hosting for students: deploy apps from public Git repositories, with plans, HTTPS, databases, an API and an MCP server."

echo ">> security features"
gh api -X PUT "repos/$repo/private-vulnerability-reporting" --silent # SECURITY.md
gh api -X PUT "repos/$repo/vulnerability-alerts" --silent           # Dependabot alerts
gh api -X PUT "repos/$repo/automated-security-fixes" --silent       # Dependabot security updates
gh api -X PATCH "repos/$repo" --silent --input - <<'EOF'
{"security_and_analysis": {"secret_scanning": {"status": "enabled"}, "secret_scanning_push_protection": {"status": "enabled"}}}
EOF

echo ">> protection of main (ruleset)"
# Changes reach main through pull requests with one approval and both checks
# green; no force pushes or deletion. Repository admins may merge their own
# pull request (bypass_mode: pull_request) while the team is small.
ruleset="$(jq -n '{
  name: "main",
  target: "branch",
  enforcement: "active",
  conditions: {ref_name: {include: ["~DEFAULT_BRANCH"], exclude: []}},
  bypass_actors: [{actor_id: 5, actor_type: "RepositoryRole", bypass_mode: "pull_request"}],
  rules: [
    {type: "deletion"},
    {type: "non_fast_forward"},
    {type: "pull_request", parameters: {
      required_approving_review_count: 1, dismiss_stale_reviews_on_push: true,
      require_code_owner_review: false, require_last_push_approval: false,
      required_review_thread_resolution: true}},
    {type: "required_status_checks", parameters: {
      strict_required_status_checks_policy: false,
      required_status_checks: [
        {context: "scripts/check.sh", integration_id: 15368},
        {context: "Sign-off (DCO)", integration_id: 15368}]}}
  ]}')"
id="$(gh api "repos/$repo/rulesets" --jq '.[] | select(.name == "main") | .id')"
if [[ -n "$id" ]]; then
  gh api -X PUT "repos/$repo/rulesets/$id" --silent --input - <<<"$ruleset"
else
  gh api -X POST "repos/$repo/rulesets" --silent --input - <<<"$ruleset"
fi

echo ">> labels"
scripts/github-labels.sh "$repo"

echo "Done: https://github.com/$repo/settings"
