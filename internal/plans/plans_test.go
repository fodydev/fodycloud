// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package plans

import (
	"testing"
	"time"
)

const sample = `{"plans":{
 "free":{"cpus":0.25,"memory":"256m","disk":"1G","pids":128,"max_apps":2,"max_custom_domains":0},
 "student":{"cpus":1,"memory":"1g","disk":"5G","pids":512,"max_apps":5,"max_custom_domains":5}},
 "apps_domain":"apps.x.test"}`

func TestParseAndNames(t *testing.T) {
	c, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Names(); len(got) != 2 || got[0] != "free" || got[1] != "student" {
		t.Fatalf("Names() = %v", got)
	}
	p, ok := c.Get("student")
	if !ok || p.Name != "student" || p.MaxApps != 5 || p.MaxCustomDomains != 5 {
		t.Fatalf("Get(student) = %+v", p)
	}
	if b, _ := p.MemoryBytes(); b != 1<<30 {
		t.Fatalf("memory = %d", b)
	}
}

func TestParseRejects(t *testing.T) {
	for _, raw := range []string{
		`{"plans":{"student":{"cpus":1,"memory":"1g","disk":"1g","pids":1,"max_apps":1}}}`, // no free plan
		`{"plans":{"free":{"cpus":0,"memory":"1g","disk":"1g","pids":1,"max_apps":1}}}`,    // zero cpus
		`{"plans":{"free":{"cpus":1,"memory":"lots","disk":"1g","pids":1,"max_apps":1}}}`,  // bad size
	} {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("Parse accepted %s", raw)
		}
	}
}

func TestEffective(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	cases := []struct {
		assigned string
		until    *time.Time
		want     string
	}{
		{"", nil, "free"},
		{"student", nil, "student"},     // no expiry
		{"student", &future, "student"}, // still valid
		{"student", &past, "free"},      // expired
		{"student", &now, "free"},       // expires exactly now
	}
	for _, c := range cases {
		if got := Effective(c.assigned, c.until, now); got != c.want {
			t.Errorf("Effective(%q, %v) = %q, want %q", c.assigned, c.until, got, c.want)
		}
	}
}

func TestParseSize(t *testing.T) {
	ok := map[string]int64{"256m": 256 << 20, "1g": 1 << 30, "5G": 5 << 30, "512k": 512 << 10, "1024": 1024, "1.5g": 3 << 29}
	for in, want := range ok {
		if got, err := ParseSize(in); err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "g", "-1g", "abc", "1x"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q) accepted", in)
		}
	}
}

func TestStorageFields(t *testing.T) {
	c, err := Parse([]byte(`{"plans":{"free":{"cpus":0.25,"memory":"256m","disk":"1G","pids":128,"max_apps":2,"db_size":"100M","db_connections":5,"cache_memory":"32m"},
		"student":{"cpus":1,"memory":"1g","disk":"5G","pids":512,"max_apps":5,"volume_size":"1G","db_size":"1G","db_connections":20,"cache_memory":"128m"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	free, _ := c.Get("free")
	student, _ := c.Get("student")
	if free.Has("volume") || !free.Has("postgres") || !free.Has("valkey") || !student.Has("volume") || free.Has("other") {
		t.Errorf("Has: free %+v student %+v", free, student)
	}
	for name, bad := range map[string]string{
		"invalid size":           `"volume_size":"lots"`,
		"database without conns": `"db_size":"1G"`,
	} {
		raw := `{"plans":{"free":{"cpus":0.25,"memory":"256m","disk":"1G","pids":128,"max_apps":2,` + bad + `}}}`
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
