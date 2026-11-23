// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package sse

import "testing"

func TestEncode(t *testing.T) {
	cases := []struct{ name, id, data, want string }{
		{"log", "12", "one\ntwo\n", "event: log\nid: 12\ndata: one\ndata: two\ndata: \n\n"},
		{"log", "", "a\r\nb\rc", "event: log\ndata: a\ndata: b\ndata: c\n\n"},
		{"", "", "x", "data: x\n\n"},
	}
	for _, c := range cases {
		if got := Encode(c.name, c.id, c.data); got != c.want {
			t.Errorf("Encode(%q, %q, %q) = %q, want %q", c.name, c.id, c.data, got, c.want)
		}
	}
}
