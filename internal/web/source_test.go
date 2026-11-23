// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fodydev/fodycloud/internal/service"
)

func sourceRoot(t *testing.T, archive, version string) http.Handler {
	t.Helper()
	w := &Web{Svc: &service.Service{}, Log: slog.Default(), BaseDomain: "fc.test", SecureCookies: true,
		Version: version, SourceURL: "https://forge.example/fork", SourceArchive: archive}
	if err := w.Init(); err != nil {
		t.Fatal(err)
	}
	return w.Root(http.NotFoundHandler(), http.NotFoundHandler())
}

func get(h http.Handler, url string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
	return rec
}

// The AGPL-3.0 offer: the page and the exact archive, on the home site.
func TestSourceArchive(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "source.tar.gz")
	if err := os.WriteFile(archive, []byte("tarball"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := sourceRoot(t, archive, "abc123def456")

	page := get(root, "https://fc.test/source")
	for _, want := range []string{"abc123def456", `href="https://fc.test/source.tar.gz"`, `href="https://forge.example/fork"`, "Affero"} {
		if page.Code != 200 || !strings.Contains(page.Body.String(), want) {
			t.Errorf("source page: %d, lacks %q", page.Code, want)
		}
	}

	dl := get(root, "https://fc.test/source.tar.gz")
	if dl.Code != 200 || dl.Body.String() != "tarball" {
		t.Fatalf("archive: %d %q", dl.Code, dl.Body.String())
	}
	if cd := dl.Header().Get("Content-Disposition"); cd != `attachment; filename="fodycloud-abc123def456-source.tar.gz"` {
		t.Errorf("Content-Disposition %q", cd)
	}
	if ct := dl.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Errorf("Content-Type %q", ct)
	}
}

// A version that isn't safe in a header falls back to a fixed file name.
func TestSourceArchiveOddVersion(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "source.tar.gz")
	if err := os.WriteFile(archive, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dl := get(sourceRoot(t, archive, `bad"version`), "https://fc.test/source.tar.gz")
	if cd := dl.Header().Get("Content-Disposition"); cd != `attachment; filename="fodycloud-source.tar.gz"` {
		t.Errorf("Content-Disposition %q", cd)
	}
}

// Development builds have no archive: the page says so, the download is a 404.
func TestSourceWithoutArchive(t *testing.T) {
	root := sourceRoot(t, filepath.Join(t.TempDir(), "missing.tar.gz"), "dev")
	page := get(root, "https://fc.test/source")
	if page.Code != 200 || !strings.Contains(page.Body.String(), "development build") || strings.Contains(page.Body.String(), "source.tar.gz") {
		t.Errorf("source page without archive: %d", page.Code)
	}
	if dl := get(root, "https://fc.test/source.tar.gz"); dl.Code != 404 {
		t.Errorf("missing archive: %d, want 404", dl.Code)
	}
	if dl := get(sourceRoot(t, "", "dev"), "https://fc.test/source.tar.gz"); dl.Code != 404 {
		t.Errorf("no archive setting: %d, want 404", dl.Code)
	}
}

// Every site's pages, wake pages included, link to the source.
func TestFooterLinksToSource(t *testing.T) {
	root := sourceRoot(t, "", "dev")
	for _, url := range []string{"https://fc.test/", "https://fc.test/docs", "https://dashboard.fc.test/login",
		"https://admin.fc.test/", "https://api.fc.test/", "https://mcp.fc.test/", "https://fc.test/nope"} {
		if body := get(root, url).Body.String(); !strings.Contains(body, `href="https://fc.test/source"`) {
			t.Errorf("%s lacks the source link", url)
		}
	}
}
