// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/deployer"
	"github.com/fodydev/fodycloud/internal/plans"
)

// Storage and databases of an app, at most one of each kind:
//   - volume: a persistent volume mounted at /data (deployer.CreateVolume);
//   - postgres: a database on the shared apps-postgres server (appsdb);
//   - valkey: an in-memory cache on a network private to the app.
//
// Their limits come from the plan. Adding or removing one restarts the app,
// which gets the mount or the connection variables.

// ServiceKinds are the kinds of storage an app can add.
var ServiceKinds = []string{"volume", "postgres", "valkey"}

// serviceLabels name each kind for people.
var serviceLabels = map[string]string{"volume": "a persistent volume", "postgres": "a PostgreSQL database", "valkey": "a Valkey cache"}

// ReservedEnv are the variables FodyCloud sets for an app's storage; students
// can't set them.
var ReservedEnv = []string{"DATABASE_URL", "PGHOST", "PGPORT", "PGUSER", "PGPASSWORD", "PGDATABASE", "REDIS_URL", "FODYCLOUD_DATA_DIR"}

// AppService is one storage service of an app, for its owner.
type AppService struct {
	Kind       string     `json:"kind"`
	URL        string     `json:"url,omitempty"`  // connection URL (postgres, valkey); contains the password
	Path       string     `json:"path,omitempty"` // mount path (volume)
	Limit      string     `json:"limit"`          // from the plan
	UsedBytes  int64      `json:"used_bytes"`     // last measurement
	MeasuredAt *time.Time `json:"measured_at,omitempty"`
	ReadOnly   bool       `json:"read_only,omitempty"` // postgres over its size limit
	Blocked    bool       `json:"blocked,omitempty"`   // postgres far over its limit: connections refused
	CreatedAt  time.Time  `json:"created_at"`
}

type serviceRow struct {
	appID      int64
	kind       string
	secret     string
	size       int64
	measuredAt *time.Time
	readOnly   *time.Time
	blocked    *time.Time
	createdAt  time.Time
}

func (s *Service) serviceRows(ctx context.Context, appID int64) ([]serviceRow, error) {
	rows, err := s.DB.Query(ctx, `SELECT app_id, kind, secret, size_bytes, measured_at, read_only_since, blocked_since, created_at
		FROM app_services WHERE app_id=$1 ORDER BY array_position(ARRAY['volume','postgres','valkey'], kind)`, appID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanServiceRow)
}

func scanServiceRow(r pgx.CollectableRow) (serviceRow, error) {
	var v serviceRow
	return v, r.Scan(&v.appID, &v.kind, &v.secret, &v.size, &v.measuredAt, &v.readOnly, &v.blocked, &v.createdAt)
}

func newSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func validKind(kind string) error {
	if !slices.Contains(ServiceKinds, kind) {
		return invalid("unknown storage %q (available: volume, postgres, valkey)", kind)
	}
	return nil
}

// planLimit is the plan's limit for a kind, for display.
func planLimit(p plans.Plan, kind string) string {
	switch kind {
	case "volume":
		return p.VolumeSize
	case "postgres":
		return p.DBSize
	}
	return p.CacheMemory
}

// Services lists the app's storage with connection URLs (the owner's own
// secrets).
func (s *Service) Services(ctx context.Context, u *auth.User, name string) ([]AppService, error) {
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	info, err := s.PlanInfo(ctx, u)
	if err != nil {
		return nil, err
	}
	rows, err := s.serviceRows(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	out := make([]AppService, 0, len(rows))
	for _, r := range rows {
		v := AppService{Kind: r.kind, Limit: planLimit(info.Limits, r.kind), UsedBytes: r.size, MeasuredAt: r.measuredAt,
			ReadOnly: r.readOnly != nil, Blocked: r.blocked != nil, CreatedAt: r.createdAt}
		switch r.kind {
		case "volume":
			v.Path = deployer.DataDir
		case "postgres":
			v.URL = s.AppsDB.URL(a.ID, r.secret)
		case "valkey":
			v.URL = valkeyURL(r.secret)
		}
		out = append(out, v)
	}
	return out, nil
}

func valkeyURL(secret string) string {
	return "redis://default:" + secret + "@" + deployer.ValkeyHost + ":6379/0"
}

// AddService adds a storage service the plan includes, then restarts the app
// so it gets the mount or its connection variables.
func (s *Service) AddService(ctx context.Context, u *auth.User, name, kind string) (*AppService, error) {
	if err := requireWrite(u); err != nil {
		return nil, err
	}
	if err := validKind(kind); err != nil {
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
	if !info.Limits.Has(kind) {
		return nil, &QuotaError{Msg: fmt.Sprintf("the %s plan doesn't include %s", info.Plan, serviceLabels[kind])}
	}
	if kind == "postgres" && !s.AppsDB.Enabled() {
		return nil, invalid("managed databases are not available on this platform")
	}
	secret := ""
	if kind != "volume" {
		if secret, err = newSecret(); err != nil {
			return nil, err
		}
	}
	tag, err := s.DB.Exec(ctx, `INSERT INTO app_services(app_id, kind, secret) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, a.ID, kind, secret)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, invalid("this app already has %s", serviceLabels[kind])
	}
	if err := s.provision(ctx, a.ID, a.Name, kind, secret, info.Limits); err != nil {
		s.DB.Exec(ctx, `DELETE FROM app_services WHERE app_id=$1 AND kind=$2`, a.ID, kind) //nolint:errcheck
		return nil, fmt.Errorf("add %s: %w", kind, err)
	}
	s.Log.Info("storage added", "app", a.Name, "kind", kind)
	if err := s.restartForStorage(ctx, u, a); err != nil {
		return nil, err
	}
	all, err := s.Services(ctx, u, name)
	if err != nil {
		return nil, err
	}
	for _, v := range all {
		if v.Kind == kind {
			return &v, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Service) provision(ctx context.Context, appID int64, app, kind, secret string, p plans.Plan) error {
	switch kind {
	case "volume":
		return s.Deployer.CreateVolume(ctx, app, p.VolumeSize)
	case "postgres":
		return s.AppsDB.Create(ctx, appID, secret, p.DBConnections)
	default:
		return s.Deployer.StartValkey(ctx, app, secret, p)
	}
}

// restartForStorage redeploys the app's current image so it picks up a new
// or removed service (nothing to do before its first deployment).
func (s *Service) restartForStorage(ctx context.Context, u *auth.User, a *App) error {
	if a.Deployment == nil || a.Deployment.image == "" {
		return nil
	}
	_, err := s.enqueue(ctx, a.ID, "restart", a.Deployment.image)
	return err
}

// RemoveService deletes a storage service and its data, for good. confirm
// must repeat the app's name.
func (s *Service) RemoveService(ctx context.Context, u *auth.User, name, kind, confirm string) error {
	if err := requireWrite(u); err != nil {
		return err
	}
	if err := validKind(kind); err != nil {
		return err
	}
	if confirm != name {
		return invalid("confirmation does not match the app name")
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return err
	}
	tag, err := s.DB.Exec(ctx, `DELETE FROM app_services WHERE app_id=$1 AND kind=$2`, a.ID, kind)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	switch kind {
	case "postgres":
		err = s.AppsDB.Drop(ctx, a.ID)
	case "valkey":
		err = s.Deployer.RemoveValkey(ctx, a.Name)
	case "volume":
		// In use by the running container: removed after the restart below
		// replaces it (CleanupStorage), or now if nothing uses it.
		if rmErr := s.Deployer.RemoveVolume(ctx, a.Name); rmErr != nil {
			s.Log.Info("volume removed after the restart", "app", a.Name)
		}
	}
	if err != nil {
		return err
	}
	s.Log.Info("storage removed", "app", a.Name, "kind", kind)
	return s.restartForStorage(ctx, u, a)
}

// JobStorage prepares an app's storage for a deployment: it says whether to
// mount the volume and join the cache network, and makes sure the cache runs
// (it is stopped while the account is suspended). The connection variables
// come with JobConfig.
func (s *Service) JobStorage(ctx context.Context, j *Job, p plans.Plan) (volume, cache bool, err error) {
	rows, err := s.serviceRows(ctx, j.AppID)
	if err != nil {
		return false, false, err
	}
	for _, r := range rows {
		switch r.kind {
		case "volume":
			volume = true
		case "valkey":
			cache = true
			if !s.Deployer.ValkeyRunning(ctx, j.AppName) {
				if p.CacheMemory == "" { // plan lost its cache: keep the size it had
					p.CacheMemory = "32m"
				}
				if err := s.Deployer.StartValkey(ctx, j.AppName, r.secret, p); err != nil {
					return false, false, err
				}
			}
		}
	}
	return volume, cache, nil
}

// ensureCache starts the app's cache if it has one and it isn't running
// (stopped while the account was suspended).
func (s *Service) ensureCache(ctx context.Context, appID int64, app string, p plans.Plan) error {
	var secret string
	err := s.DB.QueryRow(ctx, `SELECT secret FROM app_services WHERE app_id=$1 AND kind='valkey'`, appID).Scan(&secret)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && s.Deployer.ValkeyRunning(ctx, app)) {
		return nil
	}
	if err != nil {
		return err
	}
	if p.CacheMemory == "" {
		p.CacheMemory = "32m"
	}
	return s.Deployer.StartValkey(ctx, app, secret, p)
}

// storageEnv returns the variables for an app's storage services.
func (s *Service) storageEnv(ctx context.Context, appID int64) (map[string]string, error) {
	rows, err := s.serviceRows(ctx, appID)
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	for _, r := range rows {
		switch r.kind {
		case "volume":
			env["FODYCLOUD_DATA_DIR"] = deployer.DataDir
		case "postgres":
			for k, v := range s.AppsDB.Env(appID, r.secret) {
				env[k] = v
			}
		case "valkey":
			env["REDIS_URL"] = valkeyURL(r.secret)
		}
	}
	return env, nil
}

// CleanupStorage deletes what an app's services left behind once nothing
// uses it any more: a removed volume (after the restart that unmounted it).
func (s *Service) CleanupStorage(ctx context.Context, appID int64, app string) {
	var has bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_services WHERE app_id=$1 AND kind='volume')`, appID).Scan(&has); err != nil || has {
		return
	}
	s.Deployer.RemoveVolume(ctx, app) //nolint:errcheck // absent, or still in use until the next deployment
}

// dropStorage deletes all of an app's storage (the app is being deleted; its
// containers are already removed).
func (s *Service) dropStorage(ctx context.Context, appID int64, app string) error {
	var errs []error
	errs = append(errs, s.Deployer.RemoveValkey(ctx, app))
	if err := s.Deployer.RemoveVolume(ctx, app); err != nil && !s.Deployer.VolumeMissing(ctx, app) {
		errs = append(errs, err)
	}
	var hasDB bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_services WHERE app_id=$1 AND kind='postgres')`, appID).Scan(&hasDB); err != nil {
		errs = append(errs, err)
	} else if hasDB {
		errs = append(errs, s.AppsDB.Drop(ctx, appID))
	}
	return errors.Join(errs...)
}

// applyStoragePlan applies a plan's cache size and connection limit to an
// app's services (a volume keeps its size: it can't shrink).
func (s *Service) applyStoragePlan(ctx context.Context, appID int64, app string, p plans.Plan) error {
	rows, err := s.serviceRows(ctx, appID)
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range rows {
		switch {
		case r.kind == "valkey" && p.CacheMemory != "" && s.Deployer.ValkeyRunning(ctx, app):
			errs = append(errs, s.Deployer.ResizeValkey(ctx, app, r.secret, p))
		case r.kind == "postgres" && p.DBConnections > 0 && r.blocked == nil:
			errs = append(errs, s.AppsDB.SetConnectionLimit(ctx, appID, p.DBConnections))
		}
	}
	return errors.Join(errs...)
}

// dbState is what a database's size calls for: read-only above the plan's
// limit; connections refused when a database that was already read-only
// grew past 120% (the app can turn the read-only setting off, so growing
// further means it did). A first pass over the limit, even a large one
// (a bulk import), only makes it read-only. A blocked database opens again
// once it fits the limit (e.g. a bigger plan).
func dbState(size, limit int64, wasReadOnly, blocked bool) (readOnly, block bool) {
	block = (wasReadOnly && size > limit*12/10) || (blocked && size > limit)
	return size > limit, block
}

// CheckStorage measures every app's volume and database, and enforces the
// database size limits: read-only when full (the student is emailed), back
// to writable once it fits, and blocked when far over (the admins are told).
func (s *Service) CheckStorage(ctx context.Context) error {
	rows, err := s.DB.Query(ctx, `SELECT v.app_id, v.kind, v.secret, v.size_bytes, v.measured_at, v.read_only_since, v.blocked_since, v.created_at
		FROM app_services v WHERE v.kind IN ('volume','postgres') ORDER BY v.app_id`)
	if err != nil {
		return err
	}
	list, err := pgx.CollectRows(rows, scanServiceRow)
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range list {
		var name, email string
		var userID int64
		if err := s.DB.QueryRow(ctx, `SELECT a.name, u.email, u.id FROM apps a JOIN users u ON u.id=a.user_id WHERE a.id=$1`, r.appID).Scan(&name, &email, &userID); err != nil {
			continue // deleted meanwhile
		}
		if r.kind == "volume" {
			size, err := s.Deployer.VolumeUsage(ctx, name)
			if err == nil {
				_, err = s.DB.Exec(ctx, `UPDATE app_services SET size_bytes=$3, measured_at=now() WHERE app_id=$1 AND kind=$2`, r.appID, r.kind, size)
			}
			errs = append(errs, err)
			continue
		}
		errs = append(errs, s.checkDatabase(ctx, r, name, email, userID))
	}
	return errors.Join(errs...)
}

func (s *Service) checkDatabase(ctx context.Context, r serviceRow, app, email string, userID int64) error {
	size, err := s.AppsDB.Size(ctx, r.appID)
	if err != nil {
		return err
	}
	p, err := s.PlanFor(ctx, userID)
	if err != nil {
		return err
	}
	limit, _ := plans.ParseSize(p.DBSize)
	if p.DBSize == "" { // the plan no longer includes databases: keep what fits the default
		limit, _ = plans.ParseSize("100M")
	}
	readOnly, block := dbState(size, limit, r.readOnly != nil, r.blocked != nil)
	if _, err := s.DB.Exec(ctx, `UPDATE app_services SET size_bytes=$2, measured_at=now() WHERE app_id=$1 AND kind='postgres'`, r.appID, size); err != nil {
		return err
	}
	data := map[string]any{"App": app, "Size": HumanBytes(size), "Limit": HumanBytes(limit)}
	switch {
	case block && r.blocked == nil:
		if err := s.AppsDB.SetConnectionLimit(ctx, r.appID, 0); err != nil {
			return err
		}
		s.DB.Exec(ctx, `UPDATE app_services SET blocked_since=now() WHERE app_id=$1 AND kind='postgres'`, r.appID) //nolint:errcheck
		s.NotifyAdmins(ctx, "Database blocked: "+app, fmt.Sprintf("The database of %s (%s) grew to %s, more than 120%% of its %s limit, while read-only. Its connections are refused until it fits its plan.", app, email, HumanBytes(size), HumanBytes(limit)))
		s.Log.Warn("database blocked", "app", app, "size", size)
	case !block && r.blocked != nil:
		if err := s.AppsDB.SetConnectionLimit(ctx, r.appID, max(p.DBConnections, 1)); err != nil {
			return err
		}
		s.DB.Exec(ctx, `UPDATE app_services SET blocked_since=NULL WHERE app_id=$1 AND kind='postgres'`, r.appID) //nolint:errcheck
	}
	switch {
	case readOnly && r.readOnly == nil:
		if err := s.AppsDB.SetReadOnly(ctx, r.appID, true); err != nil {
			return err
		}
		s.DB.Exec(ctx, `UPDATE app_services SET read_only_since=now() WHERE app_id=$1 AND kind='postgres'`, r.appID) //nolint:errcheck
		s.mail(ctx, email, "db-full", data)
		s.Log.Info("database read-only", "app", app, "size", size)
	case !readOnly && r.readOnly != nil:
		if err := s.AppsDB.SetReadOnly(ctx, r.appID, false); err != nil {
			return err
		}
		s.DB.Exec(ctx, `UPDATE app_services SET read_only_since=NULL WHERE app_id=$1 AND kind='postgres'`, r.appID) //nolint:errcheck
		s.mail(ctx, email, "db-writable", data)
		s.Log.Info("database writable again", "app", app, "size", size)
	}
	return nil
}

// StorageUse sums the storage of one kind across apps, for admins.
type StorageUse struct {
	Kind      string `json:"kind"`
	Count     int    `json:"count"`
	UsedBytes int64  `json:"used_bytes"`
	ReadOnly  int    `json:"read_only"` // databases over their limit
	Blocked   int    `json:"blocked"`
}

// StorageTotals sums the apps' volumes, databases and caches.
func (s *Service) StorageTotals(ctx context.Context, admin *auth.Admin) ([]StorageUse, error) {
	if err := requireAdmin(admin); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT kind, count(*), coalesce(sum(size_bytes),0)::bigint,
		count(read_only_since), count(blocked_since) FROM app_services GROUP BY kind
		ORDER BY array_position(ARRAY['volume','postgres','valkey'], kind)`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[StorageUse])
}

// HumanBytes writes a size for people: 512 KB, 20 MB, 1.5 GB.
func HumanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", n>>20)
	default:
		return fmt.Sprintf("%d KB", n>>10)
	}
}
