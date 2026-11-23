// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fodydev/fodycloud/internal/auth"
)

var hostnameRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// DomainStatus describes a custom domain.
type DomainStatus struct {
	Hostname     string `json:"hostname"`
	Verified     bool   `json:"verified"`
	Instructions string `json:"instructions"`
}

// AddDomain attaches a custom domain, within the plan's max_custom_domains
// per app.
func (s *Service) AddDomain(ctx context.Context, u *auth.User, name, host string) (*DomainStatus, error) {
	if err := requireWrite(u); err != nil {
		return nil, err
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	info, err := s.PlanInfo(ctx, u)
	if err != nil {
		return nil, err
	}
	if info.Limits.MaxCustomDomains <= 0 {
		return nil, &QuotaError{Msg: fmt.Sprintf("custom domains are not included in the %s plan", info.Plan)}
	}
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if !hostnameRe.MatchString(host) || len(host) > 253 {
		return nil, invalid("invalid hostname")
	}
	base := strings.TrimPrefix(s.Deployer.AppsDomain, "apps.")
	if host == base || strings.HasSuffix(host, "."+base) {
		return nil, invalid("subdomains of %s cannot be used as custom domains", base)
	}
	// Count and insert under a lock on the app, so two requests can't both
	// take the last free slot.
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM domains WHERE app_id=(SELECT id FROM apps WHERE id=$1 FOR UPDATE)`, a.ID).Scan(&count); err != nil {
		return nil, err
	}
	if err := domainLimit(info.Plan, info.Limits.MaxCustomDomains, count); err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO domains(app_id, hostname) VALUES ($1,$2) ON CONFLICT (hostname) DO NOTHING`, a.ID, host)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, invalid("%s is already attached to an app", host)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.DomainStatus(ctx, u, name, host)
}

// domainLimit refuses a new domain when the app already has the plan's
// maximum.
func domainLimit(plan string, max, count int) error {
	if count >= max {
		return &QuotaError{Msg: fmt.Sprintf("the %s plan allows %d custom domains per app; remove one first", plan, max)}
	}
	return nil
}

// RemoveDomain detaches a custom domain from an app. A verified domain was
// routed to the running app, which is restarted so Traefik drops the route.
func (s *Service) RemoveDomain(ctx context.Context, u *auth.User, name, host string) error {
	if err := requireWrite(u); err != nil {
		return err
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return err
	}
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	var verified bool
	err = s.DB.QueryRow(ctx, `DELETE FROM domains WHERE app_id=$1 AND hostname=$2 RETURNING verified`, a.ID, host).Scan(&verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if verified && a.Status == "running" {
		_, err = s.Restart(ctx, u, name)
	}
	return err
}

// DomainStatus checks DNS for a custom domain and redeploys the app once the
// domain points to FodyCloud, so Traefik starts routing it.
func (s *Service) DomainStatus(ctx context.Context, u *auth.User, name, host string) (*DomainStatus, error) {
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	var id int64
	var verified bool
	err = s.DB.QueryRow(ctx, `SELECT id, verified FROM domains WHERE app_id=$1 AND hostname=$2`, a.ID, host).Scan(&id, &verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	target := s.Deployer.Hostname(a.Name)
	st := &DomainStatus{Hostname: host, Verified: verified,
		Instructions: fmt.Sprintf("Create a CNAME record %s → %s", host, target)}
	if s.PublicIP != "" {
		st.Instructions += fmt.Sprintf(" (or an A record %s → %s for an apex domain)", host, s.PublicIP)
	}
	if verified {
		return st, nil
	}
	if s.pointsHere(ctx, host, target) {
		if _, err := s.DB.Exec(ctx, `UPDATE domains SET verified=true WHERE id=$1`, id); err != nil {
			return nil, err
		}
		st.Verified = true
		if a.Status == "running" {
			if _, err := s.Restart(ctx, u, name); err != nil {
				return nil, err
			}
		}
	}
	return st, nil
}

func (s *Service) pointsHere(ctx context.Context, host, target string) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var r net.Resolver
	if cname, err := r.LookupCNAME(ctx, host); err == nil && strings.TrimSuffix(cname, ".") == target {
		return true
	}
	if s.PublicIP == "" {
		return false
	}
	ips, err := r.LookupHost(ctx, host)
	return err == nil && slices.Contains(ips, s.PublicIP)
}

func (s *Service) appDomains(ctx context.Context, appID int64) ([]string, error) {
	return s.domains(ctx, appID, false)
}

func (s *Service) domains(ctx context.Context, appID int64, verifiedOnly bool) ([]string, error) {
	rows, err := s.DB.Query(ctx, `SELECT hostname FROM domains WHERE app_id=$1 AND (verified OR NOT $2) ORDER BY hostname`, appID, verifiedOnly)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
