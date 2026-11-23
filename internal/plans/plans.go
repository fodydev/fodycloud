// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package plans loads plan limits from plans.json (rendered by Terraform) and
// resolves which plan applies to a user.
package plans

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Default is the plan every user has without an (active) subscription.
const Default = "free"

// Plan holds the resource limits of one subscription plan.
type Plan struct {
	Name    string  `json:"-"`
	CPUs    float64 `json:"cpus"`
	Memory  string  `json:"memory"`
	Disk    string  `json:"disk"`
	Pids    int64   `json:"pids"`
	MaxApps int     `json:"max_apps"`
	// MaxCustomDomains is how many custom domains each app may have (0 = none).
	MaxCustomDomains int `json:"max_custom_domains"`
	// SleepAfterMinutes stops an app after this many idle minutes; it wakes on
	// the next request. 0 disables sleeping.
	SleepAfterMinutes int `json:"sleep_after_minutes"`
	// AbandonAfterDays warns, then stops, apps without visits or deploys for
	// this many days (see service.CheckAbandoned). 0 disables it.
	AbandonAfterDays float64 `json:"abandon_after_days"`

	// Storage and databases ("" or 0 = not included in the plan).
	VolumeSize    string `json:"volume_size"`    // persistent volume mounted at /data
	DBSize        string `json:"db_size"`        // managed PostgreSQL database
	DBConnections int    `json:"db_connections"` // connections per database
	CacheMemory   string `json:"cache_memory"`   // managed Valkey (cache), maxmemory
}

// Has reports whether the plan includes a storage service ("volume",
// "postgres" or "valkey").
func (p Plan) Has(kind string) bool {
	switch kind {
	case "volume":
		return p.VolumeSize != ""
	case "postgres":
		return p.DBSize != ""
	case "valkey":
		return p.CacheMemory != ""
	}
	return false
}

// MemoryBytes returns the memory limit in bytes.
func (p Plan) MemoryBytes() (int64, error) { return ParseSize(p.Memory) }

// Catalog is the set of known plans.
type Catalog struct {
	plans map[string]Plan
}

type file struct {
	Plans map[string]Plan `json:"plans"`
}

// Load reads plans.json.
func Load(path string) (*Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

// Parse parses the content of plans.json and validates every plan.
func Parse(raw []byte) (*Catalog, error) {
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("plans: %w", err)
	}
	if _, ok := f.Plans[Default]; !ok {
		return nil, fmt.Errorf("plans: the %q plan is required", Default)
	}
	for name, p := range f.Plans {
		p.Name = name
		if p.CPUs <= 0 || p.Pids <= 0 || p.MaxApps < 0 || p.SleepAfterMinutes < 0 {
			return nil, fmt.Errorf("plans: %s: cpus and pids must be positive, max_apps and sleep_after_minutes not negative", name)
		}
		if _, err := ParseSize(p.Memory); err != nil {
			return nil, fmt.Errorf("plans: %s: memory: %w", name, err)
		}
		if _, err := ParseSize(p.Disk); err != nil {
			return nil, fmt.Errorf("plans: %s: disk: %w", name, err)
		}
		for field, v := range map[string]string{"volume_size": p.VolumeSize, "db_size": p.DBSize, "cache_memory": p.CacheMemory} {
			if v == "" {
				continue
			}
			if n, err := ParseSize(v); err != nil || n <= 0 {
				return nil, fmt.Errorf("plans: %s: %s: invalid size %q", name, field, v)
			}
		}
		if p.DBSize != "" && p.DBConnections <= 0 {
			return nil, fmt.Errorf("plans: %s: db_connections must be positive when db_size is set", name)
		}
		f.Plans[name] = p
	}
	return &Catalog{plans: f.Plans}, nil
}

// Get returns a plan by name.
func (c *Catalog) Get(name string) (Plan, bool) {
	p, ok := c.plans[name]
	return p, ok
}

// Names returns the plan names sorted by CPU allocation (smallest first).
func (c *Catalog) Names() []string {
	names := make([]string, 0, len(c.plans))
	for n := range c.plans {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := c.plans[names[i]], c.plans[names[j]]
		if a.CPUs != b.CPUs {
			return a.CPUs < b.CPUs
		}
		return names[i] < names[j]
	})
	return names
}

// Effective returns the plan that applies right now to a subscription:
// the assigned plan while it is valid, otherwise the default plan.
// validUntil == nil means the subscription never expires.
func Effective(assigned string, validUntil *time.Time, now time.Time) string {
	if assigned == "" {
		return Default
	}
	if validUntil != nil && !now.Before(*validUntil) {
		return Default
	}
	return assigned
}

// ParseSize parses Docker-style sizes such as "256m", "1g", "5G", "512k" or a
// plain number of bytes.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'k':
		mult = 1 << 10
	case 'm':
		mult = 1 << 20
	case 'g':
		mult = 1 << 30
	case 't':
		mult = 1 << 40
	case 'b':
		// plain bytes with explicit unit
	default:
		if s[len(s)-1] < '0' || s[len(s)-1] > '9' {
			return 0, fmt.Errorf("invalid size %q", s)
		}
		s += "b"
	}
	n, err := strconv.ParseFloat(s[:len(s)-1], 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return int64(n * float64(mult)), nil
}
