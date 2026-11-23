// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static
var staticFS embed.FS

func (w *Web) staticHandler() http.Handler {
	static, _ := fs.Sub(staticFS, "static")
	return http.StripPrefix("/static/", cacheStatic(http.FileServerFS(static)))
}

// assetVersions maps each static file to a short hash of its content, so a
// new release busts the one-day browser cache.
var assetVersions = func() map[string]string {
	m := map[string]string{}
	fs.WalkDir(staticFS, "static", func(p string, d fs.DirEntry, err error) error { //nolint:errcheck
		if err == nil && !d.IsDir() {
			b, _ := fs.ReadFile(staticFS, p)
			sum := sha256.Sum256(b)
			m[strings.TrimPrefix(p, "static/")] = hex.EncodeToString(sum[:4])
		}
		return nil
	})
	return m
}()

func cacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Cache-Control", "public, max-age=86400")
		h.ServeHTTP(rw, r)
	})
}
