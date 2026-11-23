// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package docs embeds the user and developer documentation (Markdown) and
// defines its navigation. <domain>/docs (and dashboard.<domain>/docs) serve the public guides, admin.<domain>/docs the admin guide; the same files
// are readable on the code forge.
package docs

import "embed"

// FS holds README.md (the index) and the guides.
//
//go:embed README.md user/*.md developer/*.md admin/*.md legal/*.md
var FS embed.FS

// Page is one document; Slug is its path without ".md", e.g. "user/stacks".
type Page struct {
	Slug  string
	Title string
}

// Section is a guide shown as a group in the sidebar.
type Section struct {
	Title string
	Pages []Page
}

// Nav is the reading order: the sidebar and the previous/next links follow it.
var Nav = []Section{
	{Title: "User guide", Pages: []Page{
		{"user/getting-started", "Getting started"},
		{"user/deploy-an-app", "Deploy your own app"},
		{"user/stacks", "Choose your stack"},
		{"user/environment-variables", "Environment variables"},
		{"user/custom-domains", "Use your own domain"},
		{"user/logs-and-troubleshooting", "Logs and troubleshooting"},
		{"user/health-checks", "Health checks"},
		{"user/storage", "Keep your app's files"},
		{"user/databases", "Databases"},
		{"user/plans-and-limits", "Plans and limits"},
		{"user/account-and-security", "Account and security"},
	}},
	{Title: "Developer guide", Pages: []Page{
		{"developer/overview", "Developer overview"},
		{"developer/authentication", "Authentication"},
		{"developer/api-reference", "API reference"},
		{"developer/build-and-runtime", "Builds and runtime"},
		{"developer/ci-cd", "Deploy from CI"},
		{"developer/mcp", "AI assistants (MCP)"},
	}},
}

// PlaceholderDomain is the example domain used in the documents; the
// dashboard replaces it with the platform's real domain.
const PlaceholderDomain = "fodycloud.example"

// PlansMarker precedes the default plans table; the dashboard replaces the
// table that follows it with the platform's live plans.
const PlansMarker = "<!-- fodycloud:plans -->"

// AdminNav is the admin guide, served only on admin.<domain>/docs to signed-in
// admins. Its first page is the guide's home.
var AdminNav = []Section{
	{Title: "Admin guide", Pages: []Page{
		{"admin/overview", "Admin overview"},
		{"admin/plans", "Plans"},
		{"admin/users-and-suspensions", "Users and suspensions"},
		{"admin/abuse-and-bans", "Abuse and repository bans"},
		{"admin/api", "Admin API"},
	}},
}

// LegalNav lists the policies served at <domain>/legal/… and accepted at
// sign-up. Their placeholders (%OPERATOR_NAME%…) come from the operator's
// settings.
var LegalNav = []Section{
	{Title: "Legal", Pages: []Page{
		{"legal/terms", "Terms of service"},
		{"legal/acceptable-use", "Acceptable-use policy"},
		{"legal/privacy", "Privacy policy"},
	}},
}

// LegalVersion identifies the current policies; it is stored with each
// student's acceptance. Change it (and LegalEffectiveDate) when the texts
// change materially.
const (
	LegalVersion       = "2026-11-23"
	LegalEffectiveDate = "23 November 2026"
)
