// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package repo

import "testing"

func TestParse(t *testing.T) {
	ok := map[string]string{
		"https://github.com/owner/project":        "https://github.com/owner/project.git",
		"https://github.com/owner/project.git":    "https://github.com/owner/project.git",
		"https://GitHub.com/owner/project/":       "https://github.com/owner/project.git",
		"https://gitlab.com/group/sub/project":    "https://gitlab.com/group/sub/project.git",
		"https://codeberg.org/user/repo":          "https://codeberg.org/user/repo.git",
		"  https://github.com/o/my.repo_name-2  ": "https://github.com/o/my.repo_name-2.git",
	}
	for in, want := range ok {
		r, err := Parse(in)
		if err != nil || r.CloneURL != want {
			t.Errorf("Parse(%q) = %q, %v; want %q", in, r.CloneURL, err, want)
		}
	}
	bad := []string{
		"http://github.com/owner/project",             // not https
		"git@github.com:owner/project.git",            // ssh
		"ssh://git@github.com/owner/project",          // ssh
		"https://user:token@github.com/owner/project", // credentials
		"https://bitbucket.org/owner/project",         // unsupported provider
		"https://github.com.evil.com/owner/project",   // lookalike host
		"https://github.com:8443/owner/project",       // custom port
		"https://github.com/owner",                    // missing repo
		"https://github.com/owner/project/tree/main",  // extra path on GitHub
		"https://github.com/../etc/passwd",            // traversal
		"https://github.com/owner/project?x=1",        // query
		"file:///etc/passwd",                          // local file
		"https://github.com/-owner/project",           // option-like segment
		"",
	}
	for _, in := range bad {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) accepted, want error", in)
		}
	}
}

func TestValidateBranch(t *testing.T) {
	for _, b := range []string{"main", "feature/x", "v1.2", "release-2026_11"} {
		if err := ValidateBranch(b); err != nil {
			t.Errorf("ValidateBranch(%q) = %v", b, err)
		}
	}
	for _, b := range []string{"", "-x", "--upload-pack=evil", "a..b", "/abs", "a b", "a;rm"} {
		if err := ValidateBranch(b); err == nil {
			t.Errorf("ValidateBranch(%q) accepted", b)
		}
	}
}
