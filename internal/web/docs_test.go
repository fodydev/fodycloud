// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"github.com/fodydev/fodycloud/internal/config"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/fodydev/fodycloud/docs"
	"github.com/fodydev/fodycloud/internal/sites"
)

func TestDocsNavCoversEveryFile(t *testing.T) {
	inNav := map[string]bool{"README.md": true}
	for _, nav := range [][]docs.Section{docs.Nav, docs.AdminNav, docs.LegalNav} {
		for _, s := range nav {
			for _, p := range s.Pages {
				if _, err := fs.Stat(docs.FS, p.Slug+".md"); err != nil {
					t.Errorf("nav entry %s has no file", p.Slug)
				}
				inNav[p.Slug+".md"] = true
			}
		}
	}
	fs.WalkDir(docs.FS, ".", func(p string, d fs.DirEntry, err error) error { //nolint:errcheck
		if err == nil && strings.HasSuffix(p, ".md") && !inNav[p] {
			t.Errorf("%s is not in docs.Nav, docs.AdminNav or docs.LegalNav", p)
		}
		return nil
	})
}

var (
	hrefRe = regexp.MustCompile(`href="([^"]+)"`)
	idRe   = regexp.MustCompile(`id="([^"]+)"`)
)

type renderedSet struct {
	set   docSet
	pages map[string]*docPage
	byURL map[string]string          // URL path → slug
	ids   map[string]map[string]bool // slug → heading ids
}

func renderSet(t *testing.T, set docSet, st sites.Sites) renderedSet {
	t.Helper()
	pages, err := renderDocs(set, st, "", legalVars(testOperator))
	if err != nil {
		t.Fatal(err)
	}
	rs := renderedSet{set: set, pages: pages, byURL: map[string]string{}, ids: map[string]map[string]bool{}}
	for slug, p := range pages {
		rs.byURL[set.Href(slug)] = slug
		rs.ids[slug] = map[string]bool{}
		for _, m := range idRe.FindAllStringSubmatch(string(p.HTML), -1) {
			rs.ids[slug][m[1]] = true
		}
	}
	return rs
}

// Every link must reach an existing page and #anchor: on the same site, on
// the home site's /docs (from the admin guide), or a repository file that exists.
// A renamed heading or page breaks this test, not readers.
func TestDocsLinksResolve(t *testing.T) {
	st := sites.For("fodycloud.example")
	public := renderSet(t, publicDocs(), st)
	admin := renderSet(t, adminDocs(), st)
	legal := renderSet(t, legalDocs(), st)
	docsOrigin := strings.TrimSuffix(st.URL(sites.Home, "/"), "/") // public guides: <domain>/docs/…
	check := func(from renderedSet, slug, href string) {
		target, frag, _ := strings.Cut(href, "#")
		in := from
		switch {
		case strings.HasPrefix(target, repoURL):
			if _, err := os.Stat("../../" + strings.TrimPrefix(target, repoURL)); err != nil {
				t.Errorf("%s: link %q points to a missing repository file", slug, href)
			}
			return
		case target == docsOrigin+"/source": // a page of the home site, not a document (source.go)
			return
		case strings.HasPrefix(target, docsOrigin+"/legal/"):
			in, target = legal, strings.TrimPrefix(target, docsOrigin)
		case strings.HasPrefix(target, docsOrigin):
			in, target = public, strings.TrimPrefix(target, docsOrigin)
		case strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:"):
			return
		case strings.HasSuffix(target, ".md"):
			t.Errorf("%s: link %q was not rewritten", slug, href)
			return
		}
		dest := slug
		if target != "" {
			var ok bool
			if dest, ok = in.byURL[target]; !ok {
				t.Errorf("%s: link %q points to a missing page", slug, href)
				return
			}
		}
		if frag != "" && !in.ids[dest][frag] {
			t.Errorf("%s: link %q points to a missing heading", slug, href)
		}
	}
	for _, rs := range []renderedSet{public, admin, legal} {
		for slug, p := range rs.pages {
			for _, m := range hrefRe.FindAllStringSubmatch(string(p.HTML), -1) {
				check(rs, slug, m[1])
			}
		}
	}
	if len(admin.pages) != len(docs.AdminNav[0].Pages) {
		t.Errorf("admin guide has %d pages", len(admin.pages))
	}
}

func TestDocsPlaceholdersAndSafety(t *testing.T) {
	st := sites.For("cloud.univ.cm")
	pages, err := renderDocs(publicDocs(), st, "| Plan | Apps |\n|---|---|\n| campus | 7 |\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	for slug, p := range pages {
		if strings.Contains(string(p.HTML), docs.PlaceholderDomain) {
			t.Errorf("%s: placeholder domain left", slug)
		}
		if strings.Contains(string(p.HTML), "<script") || strings.Contains(string(p.HTML), "<!-- fodycloud") {
			t.Errorf("%s: raw HTML rendered", slug)
		}
	}
	plansPage := string(pages["user/plans-and-limits"].HTML)
	if !strings.Contains(plansPage, "<td>campus</td>") || strings.Contains(plansPage, "<td>research</td>") {
		t.Errorf("live plans table not used:\n%s", plansPage)
	}
	if !strings.Contains(string(pages["developer/overview"].HTML), "https://api.cloud.univ.cm/v1") {
		t.Error("domain not replaced in examples")
	}
	if pages["user/getting-started"].Next == nil || pages["user/getting-started"].Prev != nil {
		t.Error("prev/next links wrong")
	}
	if !strings.Contains(string(pages["user/getting-started"].HTML), `href="/docs/user/stacks"`) {
		t.Error("links between public guides must be /docs paths (they work on the home site and the dashboard)")
	}
	adminPages, err := renderDocs(adminDocs(), st, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	ov := string(adminPages["admin/overview"].HTML)
	if !strings.Contains(ov, `href="/docs/plans"`) || !strings.Contains(ov, `href="https://cloud.univ.cm/docs/user/getting-started"`) {
		t.Errorf("admin guide links wrong:\n%s", ov)
	}
}

func TestHumanSize(t *testing.T) {
	if got := humanSize("256m") + "," + humanSize("5G"); got != "256 MB,5 GB" {
		t.Errorf("humanSize: %s", got)
	}
}

// The online guides are for students, developers and the admin panel; server
// commands (the admin CLI) belong in the README, which isn't served.
func TestDocsNeverMentionTheAdminCLI(t *testing.T) {
	st := sites.For("fodycloud.example")
	for _, set := range []docSet{publicDocs(), adminDocs()} {
		pages, err := renderDocs(set, st, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		for slug, p := range pages {
			text := strings.ToLower(string(p.HTML))
			for _, banned := range []string{"fodycloud admin", "docker compose", "admin cli", "command line"} {
				if strings.Contains(text, banned) {
					t.Errorf("%s mentions %q", slug, banned)
				}
			}
		}
	}
}

var testOperator = config.Operator{Name: "Example University", Country: "Cameroon", Email: "privacy@univ.example", DataLocation: "Yaoundé, Cameroon"}

// The policies name the platform's operator: every placeholder must be filled.
func TestLegalPagesFilled(t *testing.T) {
	pages, err := renderDocs(legalDocs(), sites.For("cloud.univ.cm"), "", legalVars(testOperator))
	if err != nil {
		t.Fatal(err)
	}
	left := regexp.MustCompile(`%[A-Z_]+%`)
	for _, slug := range []string{"legal/terms", "legal/acceptable-use", "legal/privacy"} {
		p, ok := pages[slug]
		if !ok {
			t.Fatalf("%s not rendered", slug)
		}
		html := string(p.HTML)
		if m := left.FindString(html); m != "" {
			t.Errorf("%s: placeholder %s left", slug, m)
		}
		if !strings.Contains(html, "privacy@univ.example") || !strings.Contains(html, docs.LegalVersion) {
			t.Errorf("%s: operator contact or version missing", slug)
		}
	}
	if !strings.Contains(string(pages["legal/terms"].HTML), "Example University") ||
		!strings.Contains(string(pages["legal/privacy"].HTML), "Yaoundé, Cameroon") {
		t.Error("operator name or data location missing")
	}
}
