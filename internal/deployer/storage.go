// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package deployer

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/fodydev/fodycloud/internal/plans"
)

// Storage of an app outside its containers: a persistent volume mounted at
// /data, and a Valkey cache on a private network shared only with the app.
// These carry LabelServiceOf, not LabelApp, so deployments, sleep and
// metrics (which select the app's containers by LabelApp) leave them alone.

// LabelServiceOf marks the volume, network and cache container of an app.
const LabelServiceOf = "fodycloud.service-of"

// DataDir is where the persistent volume is mounted in the app's container.
const DataDir = "/data"

// ValkeyHost is the cache's name on the app's private network.
const ValkeyHost = "valkey"

func VolumeName(app string) string  { return "fc-vol-" + app }
func NetworkName(app string) string { return "fc-net-" + app }
func ValkeyName(app string) string  { return "fc-valkey-" + app }

// CreateVolume creates the app's persistent volume, limited to size by an
// XFS project quota when the host supports it (DiskQuota).
func (d *Deployer) CreateVolume(ctx context.Context, app, size string) error {
	args := []string{"volume", "create", "--driver", "local", "--label", LabelServiceOf + "=" + app}
	if d.DiskQuota {
		args = append(args, "--opt", "size="+size)
	}
	_, err := d.Docker.Run(ctx, append(args, VolumeName(app))...)
	return err
}

// RemoveVolume deletes the app's volume and its files. The app's containers
// must be removed first.
func (d *Deployer) RemoveVolume(ctx context.Context, app string) error {
	_, err := d.Docker.Run(ctx, "volume", "rm", "--force", VolumeName(app))
	return err
}

// VolumeUsage measures the bytes used in the app's volume, with the probe
// image (read-only, no network).
func (d *Deployer) VolumeUsage(ctx context.Context, app string) (int64, error) {
	out, err := d.Docker.Run(ctx, "run", "--rm", "--network", "none", "--cap-drop", "ALL", "--read-only",
		"--security-opt", "no-new-privileges:true", "--memory", "32m", "--log-driver", "none",
		"--mount", "type=volume,src="+VolumeName(app)+",dst=/v,readonly", d.ProbeImage, "du", "-sk", "/v")
	if err != nil {
		return 0, err
	}
	f := strings.Fields(out)
	if len(f) == 0 {
		return 0, fmt.Errorf("du: unexpected output %q", out)
	}
	kb, err := strconv.ParseInt(f[0], 10, 64)
	return kb << 10, err
}

// StartValkey creates the app's private network and its cache container:
// in memory only (no persistence), maxmemory from the plan, evicting the
// least recently used keys when full. The network is internal: the cache
// has no internet access, and only the app's containers join it.
func (d *Deployer) StartValkey(ctx context.Context, app, password string, p plans.Plan) error {
	net := NetworkName(app)
	if _, err := d.Docker.Run(ctx, "network", "inspect", net); err != nil {
		if _, err := d.Docker.Run(ctx, "network", "create", "--internal", "--label", LabelServiceOf+"="+app, net); err != nil {
			return err
		}
	}
	maxmem, limit, err := valkeyMemory(p.CacheMemory)
	if err != nil {
		return err
	}
	d.Docker.Run(ctx, "rm", "-f", ValkeyName(app)) //nolint:errcheck // leftover
	_, err = d.Docker.Run(ctx, "run", "--detach", "--name", ValkeyName(app),
		"--label", LabelServiceOf+"="+app,
		"--network", net, "--network-alias", ValkeyHost,
		"--restart", "unless-stopped",
		"--user", "65534:65534", // the image's entrypoint then skips its root-only steps
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--read-only", "--tmpfs", "/data:size=1m",
		"--memory", strconv.FormatInt(limit, 10), "--cpus", "0.25", "--pids-limit", "64",
		"--log-driver", "local", "--log-opt", "max-size=1m",
		d.ValkeyImage, "valkey-server",
		"--requirepass", password,
		"--maxmemory", strconv.FormatInt(maxmem, 10), "--maxmemory-policy", "allkeys-lru",
		"--save", "", "--appendonly", "no")
	return err
}

// valkeyMemory returns the cache's maxmemory and its container's memory
// limit (maxmemory plus room for the server itself).
func valkeyMemory(size string) (maxmem, limit int64, err error) {
	maxmem, err = plans.ParseSize(size)
	if err != nil {
		return 0, 0, err
	}
	return maxmem, maxmem + 16<<20, nil
}

// ResizeValkey applies a new plan's cache size to a running cache.
func (d *Deployer) ResizeValkey(ctx context.Context, app, password string, p plans.Plan) error {
	maxmem, limit, err := valkeyMemory(p.CacheMemory)
	if err != nil {
		return err
	}
	// Raise the container's limit before maxmemory, lower it after.
	cur, _ := d.Docker.Run(ctx, "inspect", "--format", "{{.HostConfig.Memory}}", ValkeyName(app))
	update := func() error {
		_, err := d.Docker.Run(ctx, "update", "--memory", strconv.FormatInt(limit, 10), "--memory-swap", strconv.FormatInt(limit, 10), ValkeyName(app))
		return err
	}
	if n, _ := strconv.ParseInt(cur, 10, 64); n < limit {
		if err := update(); err != nil {
			return err
		}
	}
	if _, err := d.Docker.Run(ctx, "exec", "--env", "REDISCLI_AUTH="+password, ValkeyName(app),
		"valkey-cli", "config", "set", "maxmemory", strconv.FormatInt(maxmem, 10)); err != nil {
		return err
	}
	return update()
}

// StopValkey stops the app's cache (suspended accounts); StartValkey
// recreates it.
func (d *Deployer) StopValkey(ctx context.Context, app string) error {
	_, err := d.Docker.Run(ctx, "rm", "-f", ValkeyName(app))
	return err
}

// RemoveValkey deletes the app's cache and its private network. The app's
// containers must be removed (or restarted without the network) first.
func (d *Deployer) RemoveValkey(ctx context.Context, app string) error {
	if _, err := d.Docker.Run(ctx, "rm", "-f", ValkeyName(app)); err != nil {
		return err
	}
	out, err := d.Docker.Run(ctx, "network", "inspect", "--format", "{{range .Containers}}{{.Name}} {{end}}", NetworkName(app))
	if err != nil {
		return nil // already gone
	}
	// The app's running container is still attached until its restart.
	for _, c := range strings.Fields(out) {
		d.Docker.Run(ctx, "network", "disconnect", "--force", NetworkName(app), c) //nolint:errcheck
	}
	_, err = d.Docker.Run(ctx, "network", "rm", NetworkName(app))
	return err
}

// ValkeyRunning reports whether the app's cache container runs.
func (d *Deployer) ValkeyRunning(ctx context.Context, app string) bool {
	out, err := d.Docker.Run(ctx, "inspect", "--format", "{{.State.Running}}", ValkeyName(app))
	return err == nil && out == "true"
}

// VolumeMissing reports whether the app has no volume (any more).
func (d *Deployer) VolumeMissing(ctx context.Context, app string) bool {
	_, err := d.Docker.Run(ctx, "volume", "inspect", VolumeName(app))
	return err != nil
}
