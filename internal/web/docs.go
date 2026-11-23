// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/fodydev/fodycloud/docs"
	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/config"
	"github.com/fodydev/fodycloud/internal/plans"
	"github.com/fodydev/fodycloud/internal/sites"
)

// repoURL is where documents link for repository files that aren't guides
// (README, runbooks).
const repoURL = "https://github.com/fodydev/fodycloud/blob/main/"

// docPage is a rendered documentation page.
type docPage struct {
	Slug    string // e.g. "user/stacks"; "" for the public index
	Title   string
	Section string
	HTML    template.HTML
	Prev    *docs.Page
	Next    *docs.Page
}

// docSet is a group of documents served together under one URL scheme.
type docSet struct {
	Name  string // "docs" (/docs on the home site and the dashboard) or "admin" (admin.<domain>/docs)
	Index string // file shown at the set's root, if any
	Nav   []docs.Section
	Href  func(slug string) string // URL path of a page of this set
}

func publicDocs() docSet {
	return docSet{Name: "docs", Index: "README.md", Nav: docs.Nav, Href: func(slug string) string {
		if slug == "" {
			return "/docs"
		}
		return "/docs/" + slug
	}}
}

// legalDocs are the policies, served at <domain>/legal/<page>.
func legalDocs() docSet {
	return docSet{Name: "legal", Nav: docs.LegalNav, Href: func(slug string) string { return "/" + slug }}
}

func adminDocs() docSet {
	return docSet{Name: "admin", Nav: docs.AdminNav, Href: func(slug string) string {
		if slug == docs.AdminNav[0].Pages[0].Slug {
			return "/docs" // the first page is the guide's home
		}
		return "/docs/" + strings.TrimPrefix(slug, "admin/")
	}}
}

func (d docSet) has(slug string) bool {
	for _, s := range d.Nav {
		for _, p := range s.Pages {
			if p.Slug == slug {
				return true
			}
		}
	}
	return false
}

// docsView is what the docs template receives.
type docsView struct {
	Page  *docPage
	Nav   []docs.Section
	Home  string // URL of the set's home
	Title string // the set's name, e.g. "Documentation"
	Href  func(slug string) string
}

// renderDocs renders every document of a set once: the domain placeholder
// becomes the platform's domain and the plans table becomes plansTable
// (Markdown), when given. Raw HTML in the sources is not rendered.
func renderDocs(set docSet, st sites.Sites, plansTable string, vars map[string]string) (map[string]*docPage, error) {
	var order []docs.Page
	section := map[string]string{}
	for _, s := range set.Nav {
		for _, p := range s.Pages {
			order = append(order, p)
			section[p.Slug] = s.Title
		}
	}
	links := docLinks{set: set, public: publicDocs(), legal: legalDocs(), sites: st}
	out := map[string]*docPage{}
	render := func(slug, file, title string) (*docPage, error) {
		src, err := fs.ReadFile(docs.FS, file)
		if err != nil {
			return nil, err
		}
		md := string(src)
		if plansTable != "" {
			md = replacePlansTable(md, plansTable)
		}
		if st.Base != "" {
			md = strings.ReplaceAll(md, docs.PlaceholderDomain, st.Base)
		}
		for k, v := range vars {
			md = strings.ReplaceAll(md, k, v)
		}
		l := links
		l.dir = path.Dir(file)
		gm := goldmark.New(
			goldmark.WithExtensions(extension.GFM),
			goldmark.WithParserOptions(parser.WithAutoHeadingID(),
				parser.WithASTTransformers(util.Prioritized(l, 100))),
		)
		var buf bytes.Buffer
		if err := gm.Convert([]byte(md), &buf); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		return &docPage{Slug: slug, Title: title, Section: section[slug], HTML: template.HTML(buf.String())}, nil //nolint:gosec // goldmark escapes raw HTML (WithUnsafe is off)
	}
	if set.Index != "" {
		index, err := render("", set.Index, "Documentation")
		if err != nil {
			return nil, err
		}
		out[""] = index
	}
	for i, p := range order {
		pg, err := render(p.Slug, p.Slug+".md", p.Title)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			pg.Prev = &order[i-1]
		}
		if i+1 < len(order) {
			pg.Next = &order[i+1]
		}
		out[p.Slug] = pg
	}
	return out, nil
}

// replacePlansTable swaps the Markdown table that follows the plans marker.
func replacePlansTable(md, table string) string {
	i := strings.Index(md, docs.PlansMarker)
	if i < 0 {
		return md
	}
	lines := strings.SplitAfter(strings.TrimLeft(md[i+len(docs.PlansMarker):], "\r\n"), "\n")
	k := 0
	for k < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[k]), "|") {
		k++
	}
	return md[:i] + strings.TrimRight(table, "\n") + "\n" + strings.Join(lines[k:], "")
}

// docLinks rewrites relative links between documents (x.md, ../y/z.md#a):
// to a page of the same set, a path on this site; to a public guide from
// the admin guide, an absolute https://<domain>/docs URL; to another repository
// file (README, runbooks), the code forge.
type docLinks struct {
	dir                string
	set, public, legal docSet
	sites              sites.Sites
}

func (d docLinks) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) { //nolint:errcheck
		if l, ok := n.(*ast.Link); ok && entering {
			l.Destination = []byte(d.href(string(l.Destination)))
		}
		return ast.WalkContinue, nil
	})
}

func (d docLinks) href(dest string) string {
	if strings.Contains(dest, "://") || strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "mailto:") {
		return dest
	}
	file, frag, _ := strings.Cut(dest, "#")
	if frag != "" {
		frag = "#" + frag
	}
	clean := path.Clean(path.Join(d.dir, file))
	slug := strings.TrimSuffix(clean, ".md")
	switch {
	case !strings.HasSuffix(file, ".md"):
		return dest
	case d.set.has(slug) || (slug == "README" && d.set.Index == "README.md"):
		if slug == "README" {
			slug = ""
		}
		return d.set.Href(slug) + frag
	case d.public.has(slug):
		return d.sites.URL(sites.Home, d.public.Href(slug)) + frag
	case d.legal.has(slug):
		return d.sites.URL(sites.Home, d.legal.Href(slug)) + frag
	case slug == "README":
		return d.sites.URL(sites.Docs, "/") + frag
	default: // another repository file, relative to the docs folder
		return repoURL + path.Clean(path.Join("docs", clean)) + frag
	}
}

// plansMarkdown builds the plans table from the platform's configuration.
func plansMarkdown(c *plans.Catalog) string {
	if c == nil || len(c.Names()) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("| Plan | Apps | CPU per app | Memory per app | Disk per app | Custom domains per app | Idle sleep | Stopped when unused | Volume | PostgreSQL | Valkey cache |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, n := range c.Names() {
		p, _ := c.Get(n)
		domains, sleep := strconv.Itoa(p.MaxCustomDomains), "never"
		if p.SleepAfterMinutes > 0 {
			sleep = fmt.Sprintf("after %d min", p.SleepAfterMinutes)
		}
		unused := "never"
		if p.AbandonAfterDays > 0 {
			unused = "after " + strconv.FormatFloat(p.AbandonAfterDays, 'f', -1, 64) + " days"
		}
		volume, db, cache := "no", "no", "no"
		if p.VolumeSize != "" {
			volume = humanSize(p.VolumeSize)
		}
		if p.DBSize != "" {
			db = fmt.Sprintf("%s, %d connections", humanSize(p.DBSize), p.DBConnections)
		}
		if p.CacheMemory != "" {
			cache = humanSize(p.CacheMemory)
		}
		fmt.Fprintf(&b, "| %s | %d | %s vCPU | %s | %s | %s | %s | %s | %s | %s | %s |\n", n, p.MaxApps,
			strconv.FormatFloat(p.CPUs, 'f', -1, 64), humanSize(p.Memory), humanSize(p.Disk), domains, sleep, unused, volume, db, cache)
	}
	return b.String()
}

// humanSize turns Docker sizes ("256m", "1g", "5G") into "256 MB", "1 GB".
func humanSize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	units := map[byte]string{'k': "KB", 'm': "MB", 'g': "GB", 't': "TB"}
	last := s[len(s)-1] | 0x20 // lower case
	if u, ok := units[last]; ok {
		return s[:len(s)-1] + " " + u
	}
	return s
}

func (w *Web) initDocs() error {
	var table string
	if w.Svc != nil {
		table = plansMarkdown(w.Svc.Plans)
	}
	var err error
	if w.docs, err = renderDocs(publicDocs(), w.sites, table, nil); err != nil {
		return err
	}
	if w.adminDocs, err = renderDocs(adminDocs(), w.sites, table, nil); err != nil {
		return err
	}
	w.legalDocs, err = renderDocs(legalDocs(), w.sites, "", legalVars(w.Operator))
	return err
}

// legalVars fills the policies' placeholders with the operator's details.
func legalVars(o config.Operator) map[string]string {
	return map[string]string{
		"%OPERATOR_NAME%":          o.Name,
		"%OPERATOR_COUNTRY%":       o.Country,
		"%OPERATOR_EMAIL%":         o.Email,
		"%OPERATOR_DATA_LOCATION%": o.DataLocation,
		"%LEGAL_VERSION%":          docs.LegalVersion,
		"%EFFECTIVE_DATE%":         docs.LegalEffectiveDate,
	}
}

// serveLegal renders a policy page (public, on the home site).
func (w *Web) serveLegal(rw http.ResponseWriter, r *http.Request, slug string) {
	view, title := w.docsView(legalDocs(), w.legalDocs, slug, rw)
	w.render(rw, r, "docs", title, nil, view)
}

// serveDocs renders a page of the public guides; u is the signed-in student
// in the dashboard, or nil on the home site.
func (w *Web) serveDocs(rw http.ResponseWriter, r *http.Request, set docSet, pages map[string]*docPage, slug string, u *auth.User) {
	view, title := w.docsView(set, pages, slug, rw)
	w.render(rw, r, "docs", title, u, view)
}

// serveAdminDocs renders a page of the admin guide for a signed-in admin.
func (w *Web) serveAdminDocs(rw http.ResponseWriter, r *http.Request, slug string, ad *auth.Admin) {
	view, title := w.docsView(adminDocs(), w.adminDocs, slug, rw)
	w.renderAdmin(rw, r, "docs", title, ad, view)
}

// docsView picks the page to show (writing a 404 status for unknown pages)
// and returns the template data and the page title.
func (w *Web) docsView(set docSet, pages map[string]*docPage, slug string, rw http.ResponseWriter) (docsView, string) {
	home, title := set.Href(""), "Documentation"
	switch set.Name {
	case "admin":
		home, title = set.Href(docs.AdminNav[0].Pages[0].Slug), "Admin guide"
	case "legal":
		home, title = set.Href(docs.LegalNav[0].Pages[0].Slug), "Legal"
	}
	p, ok := pages[slug]
	if !ok {
		rw.WriteHeader(http.StatusNotFound)
		p = &docPage{Slug: "-", Title: "Page not found", HTML: template.HTML(`<h1>Page not found</h1><p>This page doesn't exist. Choose a page in the menu.</p>`)}
	}
	pageTitle := p.Title
	if p.Slug != "" {
		pageTitle += " · " + title
	}
	return docsView{Page: p, Nav: set.Nav, Home: home, Title: title, Href: set.Href}, pageTitle
}
