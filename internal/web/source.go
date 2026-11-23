// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"net/http"
	"os"
	"regexp"
)

// The AGPL-3.0 (section 13) requires offering every user of the platform the
// source of the version that runs it. <domain>/source links to the public
// repository and serves the archive of the exact source the image was built
// from, so the offer stays true for modified versions too.

// safeVersion keeps the version usable in a file name.
var safeVersion = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// sourceInfo is what the source page shows.
type sourceInfo struct {
	Version    string
	Repository string
	Size       int64 // archive size in bytes; 0 = no archive (development build)
}

func (w *Web) sourceInfo() sourceInfo {
	info := sourceInfo{Version: w.Version, Repository: w.SourceURL}
	if w.SourceArchive != "" {
		if st, err := os.Stat(w.SourceArchive); err == nil && st.Mode().IsRegular() {
			info.Size = st.Size()
		}
	}
	return info
}

// sourceRoutes registers the source page and the archive on the home site.
func (w *Web) sourceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /source", func(rw http.ResponseWriter, r *http.Request) {
		w.render(rw, r, "source", "Source code", nil, w.sourceInfo())
	})
	mux.HandleFunc("GET /source.tar.gz", w.serveSourceArchive)
}

func (w *Web) serveSourceArchive(rw http.ResponseWriter, r *http.Request) {
	f, err := os.Open(w.SourceArchive)
	if w.SourceArchive == "" || err != nil {
		w.notFound(rw, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		w.notFound(rw, r)
		return
	}
	name := "fodycloud-source.tar.gz"
	if safeVersion.MatchString(w.Version) {
		name = "fodycloud-" + w.Version + "-source.tar.gz"
	}
	rw.Header().Set("Content-Type", "application/gzip")
	rw.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	rw.Header().Set("Cache-Control", "no-cache") // changes with each deploy of the platform
	http.ServeContent(rw, r, name, st.ModTime(), f)
}
