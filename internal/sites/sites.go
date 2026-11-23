// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package sites names the platform's addresses. Each one has a single role:
//
//	<domain>            public home page, and the guides at /docs
//	dashboard.<domain>  sign-in, the students' dashboard, and the guides at /docs
//	admin.<domain>      admin panel and admin guide (own sign-in)
//	api.<domain>        REST API (/v1)
//	mcp.<domain>        MCP server (/mcp)
//	*.apps.<domain>     student apps
package sites

import "strings"

// Site is one of the platform's addresses.
type Site int

const (
	Unknown Site = iota
	Home
	Dashboard
	Admin
	Docs // the guides: /docs on the home site (no host of its own)
	API
	MCP
	App // <app>.apps.<domain>
)

// Sites resolves hosts and builds absolute URLs for a base domain.
type Sites struct {
	Base string // e.g. fodycloud.example
}

// For returns the sites of a base domain.
func For(base string) Sites { return Sites{Base: strings.ToLower(base)} }

var prefixes = map[Site]string{Dashboard: "dashboard.", Admin: "admin.", API: "api.", MCP: "mcp."}

// Host is the host name of a site ("" for App and Unknown).
func (s Sites) Host(site Site) string {
	if site == Home || site == Docs {
		return s.Base
	}
	if p, ok := prefixes[site]; ok {
		return p + s.Base
	}
	return ""
}

// URL is the absolute https URL of a path on a site. Docs paths are under
// /docs on the home site: URL(Docs, "/user/stacks") = "https://<domain>/docs/user/stacks".
func (s Sites) URL(site Site, path string) string {
	if site == Docs {
		if path == "/" || path == "" {
			path = ""
		}
		path = "/docs" + path
	}
	if path == "" {
		path = "/"
	}
	return "https://" + s.Host(site) + path
}

// Of tells which site a request host (possibly with a port) belongs to.
func (s Sites) Of(host string) Site {
	host, _, _ = strings.Cut(strings.ToLower(host), ":")
	host = strings.TrimSuffix(host, ".")
	if host == s.Base {
		return Home
	}
	for site, p := range prefixes {
		if host == p+s.Base {
			return site
		}
	}
	if strings.HasSuffix(host, ".apps."+s.Base) && !strings.Contains(strings.TrimSuffix(host, ".apps."+s.Base), ".") {
		return App
	}
	return Unknown
}

// ByName maps template names ("docs", "dashboard"…) to sites.
func ByName(name string) Site {
	switch name {
	case "home":
		return Home
	case "dashboard":
		return Dashboard
	case "admin":
		return Admin
	case "docs":
		return Docs
	case "api":
		return API
	case "mcp":
		return MCP
	}
	return Unknown
}
