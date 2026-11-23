// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package deployer runs student applications as resource-limited containers
// on the shared apps network, routed by Traefik labels.
package deployer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fodydev/fodycloud/internal/dockercli"
	"github.com/fodydev/fodycloud/internal/plans"
)

// Label keys set on every app container.
const (
	LabelApp        = "fodycloud.app"
	LabelUser       = "fodycloud.user"
	LabelDeployment = "fodycloud.deployment"
)

// Deployer manages app containers.
type Deployer struct {
	Docker             *dockercli.Docker
	Network            string
	AppsDomain         string
	AppsCertResolver   string
	CustomCertResolver string
	AppPort            int
	DiskQuota          bool
	// StartupGrace is how long a new container must stay up before the old one
	// is removed.
	StartupGrace time.Duration
	// ProbeImage runs the HTTP health checks (busybox nc), in the network
	// namespace of the app's container.
	ProbeImage string
	// HealthTimeout is how long an app with a health-check path has to
	// answer it with a 2xx or 3xx status (default 60 s).
	HealthTimeout time.Duration
	// ValkeyImage runs the apps' caches (storage.go).
	ValkeyImage string
	// SwitchDelay is how long Traefik gets to route to a new container once
	// it is marked ready, before the previous one is removed (default 3 s,
	// above Traefik's 2 s provider throttle).
	SwitchDelay time.Duration
}

// readyMarker is the file that marks a container ready for traffic. Its Docker
// health check passes only once the file exists, and Traefik ignores
// containers that are "starting" or "unhealthy". So a new deployment gets no
// visitors until it has passed its checks, and a failed one never does.
// The file stays in the container's writable layer, so a restart (crash, wake
// from sleep, stop and start) is healthy again within a second.
const readyMarker = "/.fodycloud-ready"

// healthArgs are the docker run flags of the ready gate. The check uses only
// a shell builtin, so it works in any image with /bin/sh (all Railpack images).
func healthArgs() []string {
	return []string{
		"--health-cmd", "[ -e " + readyMarker + " ]",
		"--health-start-period", "24h", "--health-start-interval", "1s",
		"--health-interval", "1h", "--health-retries", "3",
	}
}

// Spec describes the desired state of one app.
type Spec struct {
	AppName       string
	UserID        int64
	DeploymentID  int64
	Image         string
	Plan          plans.Plan
	Env           map[string]string
	CustomDomains []string
	HealthPath    string // HTTP path to check before going live; "" = no check
	Volume        bool   // mount the app's persistent volume at /data
	Cache         bool   // join the app's private network with its Valkey cache
}

// ContainerName returns the container name of a deployment.
func ContainerName(app string, deploymentID int64) string {
	return fmt.Sprintf("fc-app-%s-%d", app, deploymentID)
}

// Hostname returns the free subdomain of an app.
func (d *Deployer) Hostname(app string) string { return app + "." + d.AppsDomain }

// Labels returns the Docker labels (ownership + Traefik routing) of an app container.
func (d *Deployer) Labels(s Spec) map[string]string {
	r := "app-" + s.AppName
	l := map[string]string{
		LabelApp:                 s.AppName,
		LabelUser:                strconv.FormatInt(s.UserID, 10),
		LabelDeployment:          strconv.FormatInt(s.DeploymentID, 10),
		"traefik.enable":         "true",
		"traefik.docker.network": d.Network,
		"traefik.http.services." + r + ".loadbalancer.server.port": strconv.Itoa(d.AppPort),
		"traefik.http.routers." + r + ".rule":                      hostRule([]string{d.Hostname(s.AppName)}),
		"traefik.http.routers." + r + ".entrypoints":               "websecure",
		"traefik.http.routers." + r + ".tls":                       "true",
		"traefik.http.routers." + r + ".service":                   r,
	}
	if d.AppsCertResolver != "" {
		l["traefik.http.routers."+r+".tls.certresolver"] = d.AppsCertResolver
	}
	if len(s.CustomDomains) > 0 {
		c := r + "-custom"
		l["traefik.http.routers."+c+".rule"] = hostRule(s.CustomDomains)
		l["traefik.http.routers."+c+".entrypoints"] = "websecure"
		l["traefik.http.routers."+c+".tls"] = "true"
		l["traefik.http.routers."+c+".service"] = r
		if d.CustomCertResolver != "" {
			l["traefik.http.routers."+c+".tls.certresolver"] = d.CustomCertResolver
		}
	}
	return l
}

func hostRule(hosts []string) string {
	parts := make([]string, len(hosts))
	for i, h := range hosts {
		parts[i] = "Host(`" + h + "`)"
	}
	return strings.Join(parts, " || ")
}

// LimitArgs translates a plan into docker run/update flags.
func (d *Deployer) LimitArgs(p plans.Plan, forUpdate bool) []string {
	args := []string{
		"--cpus", strconv.FormatFloat(p.CPUs, 'f', -1, 64),
		"--memory", p.Memory,
		"--memory-swap", p.Memory, // no swap beyond the memory limit
		"--pids-limit", strconv.FormatInt(p.Pids, 10),
	}
	if !forUpdate && d.DiskQuota {
		args = append(args, "--storage-opt", "size="+p.Disk)
	}
	return args
}

// Deploy starts a new container for the deployment, waits until it is stable,
// marks it ready for traffic, then removes the app's previous containers
// (blue/green: both are briefly served by the same Traefik service, but never
// before the new one has passed its checks).
func (d *Deployer) Deploy(ctx context.Context, s Spec, log io.Writer) error {
	name := ContainerName(s.AppName, s.DeploymentID)
	d.Docker.Run(ctx, "rm", "-f", name) //nolint:errcheck // leftover from an interrupted attempt

	args := []string{"run", "--detach", "--name", name,
		"--network", d.Network,
		"--hostname", s.AppName,
		"--restart", "unless-stopped",
		"--cap-drop", "ALL",
		// FrankenPHP (PHP apps) carries the file capability
		// cap_net_bind_service: without it in the bounding set, the kernel
		// refuses to run the binary. It only allows ports below 1024, in the
		// app's own network namespace.
		"--cap-add", "NET_BIND_SERVICE",
		"--security-opt", "no-new-privileges:true",
		"--log-driver", "local", "--log-opt", "max-size=5m", "--log-opt", "max-file=2",
		"--env", "PORT=" + strconv.Itoa(d.AppPort),
	}
	args = append(args, healthArgs()...)
	if s.Cache {
		args = append(args, "--network", NetworkName(s.AppName))
	}
	if s.Volume {
		args = append(args, "--mount", "type=volume,src="+VolumeName(s.AppName)+",dst="+DataDir)
	}
	args = append(args, d.LimitArgs(s.Plan, false)...)
	keys := make([]string, 0, len(s.Env))
	for k := range s.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--env", k+"="+s.Env[k])
	}
	labels := d.Labels(s)
	lkeys := make([]string, 0, len(labels))
	for k := range labels {
		lkeys = append(lkeys, k)
	}
	sort.Strings(lkeys)
	for _, k := range lkeys {
		args = append(args, "--label", k+"="+labels[k])
	}
	args = append(args, s.Image)

	fmt.Fprintf(log, "==> Starting container %s (cpus=%g memory=%s pids=%d disk=%s)\n",
		name, s.Plan.CPUs, s.Plan.Memory, s.Plan.Pids, diskInfo(d.DiskQuota, s.Plan.Disk))
	if _, err := d.Docker.Run(ctx, args...); err != nil {
		return err
	}

	if s.HealthPath != "" {
		if err := d.checkHealth(ctx, name, s, log); err != nil {
			d.Docker.Run(ctx, "rm", "-f", name) //nolint:errcheck
			return err
		}
	} else {
		grace := d.StartupGrace
		if grace == 0 {
			grace = 8 * time.Second
		}
		fmt.Fprintf(log, "==> Waiting %s for the app to stay up\n", grace)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(grace):
		}
		if err := d.checkAlive(ctx, name, log); err != nil {
			return err
		}
	}

	if err := d.markReady(ctx, name, log); err != nil {
		d.Docker.Run(ctx, "rm", "-f", name) //nolint:errcheck
		return err
	}

	// Remove the previous containers of this app.
	for _, c := range d.containers(ctx, s.AppName, true) {
		if c != name {
			fmt.Fprintf(log, "==> Removing previous container %s\n", c)
			d.Docker.Run(ctx, "rm", "-f", c) //nolint:errcheck
		}
	}
	fmt.Fprintf(log, "==> Live at https://%s\n", d.Hostname(s.AppName))
	return nil
}

// markReady creates the ready marker, waits for Docker to report the
// container healthy, then gives Traefik time to route to it.
func (d *Deployer) markReady(ctx context.Context, name string, log io.Writer) error {
	fmt.Fprintln(log, "==> Sending traffic to the new version")
	if _, err := d.Docker.Run(ctx, "exec", "--user", "0", name, "/bin/sh", "-c", ": > "+readyMarker); err != nil {
		return fmt.Errorf("couldn't mark the new version ready: %w", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		out, err := d.Docker.Run(ctx, "inspect", "--format", "{{.State.Health.Status}}", name)
		if err == nil && strings.TrimSpace(out) == "healthy" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the new version wasn't marked ready within 30s (status %q)", strings.TrimSpace(out))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	delay := d.SwitchDelay
	if delay == 0 {
		delay = 3 * time.Second
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
	}
	return nil
}

// checkAlive fails, and removes the container, if the app's first process
// is no longer running. A restart policy can make a crashing app look
// "running" at any given instant, so a deploy only succeeds if the first
// process is still alive.
func (d *Deployer) checkAlive(ctx context.Context, name string, log io.Writer) error {
	st, err := d.state(ctx, name)
	if err != nil {
		return err
	}
	if st.Healthy() {
		return nil
	}
	logs, _ := d.Docker.Combined(ctx, "logs", "--tail", "50", name)
	fmt.Fprintf(log, "    container exited (code %d). Last output:\n%s\n", st.ExitCode, logs)
	d.Docker.Run(ctx, "rm", "-f", name) //nolint:errcheck
	how := fmt.Sprintf("exited with code %d", st.ExitCode)
	if st.RestartCount > 0 { // the exit code now belongs to the restarted process
		how = fmt.Sprintf("crashed and was restarted %d time(s)", st.RestartCount)
	}
	return fmt.Errorf("the app %s during startup (see its output above); "+
		"the run command must keep a server running and listening on $PORT (%d)", how, d.AppPort)
}

// checkHealth waits until the new container answers its health-check path
// with a 2xx or 3xx status, while its first process stays alive. The probe
// runs in the container's own network namespace (so it reaches the app on
// 127.0.0.1 without joining the apps network), with no privileges, and sends
// a plain HTTP/1.0 request: redirects are not followed.
func (d *Deployer) checkHealth(ctx context.Context, name string, s Spec, log io.Writer) error {
	timeout := d.HealthTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	fmt.Fprintf(log, "==> Checking http://%s%s (up to %s)\n", d.Hostname(s.AppName), s.HealthPath, timeout)
	req := "GET " + s.HealthPath + " HTTP/1.0\r\nHost: " + d.Hostname(s.AppName) + "\r\nUser-Agent: FodyCloud-HealthCheck\r\n\r\n"
	start := time.Now()
	deadline := start.Add(timeout)
	last := "no answer"
	for {
		if err := d.checkAlive(ctx, name, log); err != nil {
			return err
		}
		pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		out, err := d.Docker.RunInput(pctx, req, "run", "-i", "--rm", "--network", "container:"+name,
			"--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--read-only",
			"--memory", "32m", "--pids-limit", "16", "--log-driver", "none",
			d.ProbeImage, "nc", "-w", "5", "127.0.0.1", strconv.Itoa(d.AppPort))
		cancel()
		if code, ok := ParseStatus(out); err == nil && ok {
			if code >= 200 && code < 400 {
				fmt.Fprintf(log, "==> Health check passed (%d) after %s\n", code, time.Since(start).Round(time.Second))
				return d.checkAlive(ctx, name, log)
			}
			last = strconv.Itoa(code)
		} else {
			last = "no HTTP answer (is the app listening on $PORT?)"
		}
		fmt.Fprintf(log, "    GET %s → %s\n", s.HealthPath, last)
		if time.Now().Add(2 * time.Second).After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	logs, _ := d.Docker.Combined(ctx, "logs", "--tail", "30", name)
	if logs != "" {
		fmt.Fprintf(log, "    Last output of the app:\n%s\n", logs)
	}
	return fmt.Errorf("the app didn't answer %s with a 2xx or 3xx status within %s (last: %s)", s.HealthPath, timeout, last)
}

// ParseStatus reads the status code of an HTTP response's first line
// ("HTTP/1.1 200 OK").
func ParseStatus(resp string) (int, bool) {
	line, _, _ := strings.Cut(resp, "\n")
	f := strings.Fields(line)
	if len(f) < 2 || !strings.HasPrefix(f[0], "HTTP/") {
		return 0, false
	}
	code, err := strconv.Atoi(f[1])
	if err != nil || code < 100 || code > 599 {
		return 0, false
	}
	return code, true
}

type state struct {
	Running      bool
	Restarting   bool
	ExitCode     int
	RestartCount int
}

// Healthy reports whether the container's first process is still running.
func (st state) Healthy() bool { return st.Running && !st.Restarting && st.RestartCount == 0 }

func (d *Deployer) state(ctx context.Context, name string) (state, error) {
	out, err := d.Docker.Run(ctx, "inspect", "--format", `{"State":{{json .State}},"RestartCount":{{.RestartCount}}}`, name)
	if err != nil {
		return state{}, err
	}
	var raw struct {
		State        state
		RestartCount int
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return state{}, err
	}
	raw.State.RestartCount = raw.RestartCount
	return raw.State, nil
}

func (d *Deployer) containers(ctx context.Context, app string, all bool) []string {
	args := []string{"ps", "--format", "{{.Names}}", "--filter", "label=" + LabelApp + "=" + app}
	if all {
		args = append(args, "--all")
	}
	out, err := d.Docker.Run(ctx, args...)
	if err != nil {
		return nil
	}
	return dockercli.Lines(out)
}

// Stop stops every container of the app.
func (d *Deployer) Stop(ctx context.Context, app string) error {
	for _, c := range d.containers(ctx, app, false) {
		if _, err := d.Docker.Run(ctx, "stop", "--time", "10", c); err != nil {
			return err
		}
	}
	return nil
}

// Start starts the app's (stopped) container. It returns false if there is
// no container to start, in which case the caller redeploys.
func (d *Deployer) Start(ctx context.Context, app string) (bool, error) {
	cs := d.containers(ctx, app, true)
	if len(cs) == 0 {
		return false, nil
	}
	for _, c := range cs {
		if _, err := d.Docker.Run(ctx, "start", c); err != nil {
			return true, err
		}
	}
	return true, nil
}

// Remove deletes the app's containers and images.
func (d *Deployer) Remove(ctx context.Context, app string) error {
	for _, c := range d.containers(ctx, app, true) {
		if _, err := d.Docker.Run(ctx, "rm", "-f", c); err != nil {
			return err
		}
	}
	out, _ := d.Docker.Run(ctx, "images", "--quiet", "--filter", "label="+LabelApp+"="+app)
	for _, img := range dockercli.Lines(out) {
		d.Docker.Run(ctx, "rmi", "-f", img) //nolint:errcheck
	}
	return nil
}

// ApplyLimits updates CPU/memory/PID limits of the app's running containers
// in place. Disk quotas can only change on the next deployment.
func (d *Deployer) ApplyLimits(ctx context.Context, app string, p plans.Plan) error {
	for _, c := range d.containers(ctx, app, true) {
		args := append([]string{"update"}, d.LimitArgs(p, true)...)
		if _, err := d.Docker.Run(ctx, append(args, c)...); err != nil {
			return err
		}
	}
	return nil
}

// Logs returns the last lines of the app's runtime output.
func (d *Deployer) Logs(ctx context.Context, app string, tail int) (string, error) {
	cs := d.containers(ctx, app, true)
	if len(cs) == 0 {
		return "", nil
	}
	return d.Docker.Combined(ctx, "logs", "--timestamps", "--tail", strconv.Itoa(tail), cs[0])
}

// Running reports whether the app has a running container.
func (d *Deployer) Running(ctx context.Context, app string) bool {
	return len(d.containers(ctx, app, false)) > 0
}

func diskInfo(quota bool, disk string) string {
	if quota {
		return disk
	}
	return "unlimited (no quota support)"
}
