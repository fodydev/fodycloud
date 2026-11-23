// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package worker processes queued deployments (build + deploy) and runs the
// periodic subscription-expiry job.
package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fodydev/fodycloud/internal/builder"
	"github.com/fodydev/fodycloud/internal/deployer"
	"github.com/fodydev/fodycloud/internal/repo"
	"github.com/fodydev/fodycloud/internal/service"
	"github.com/fodydev/fodycloud/internal/stacks"
)

// Worker runs deployments with bounded concurrency.
type Worker struct {
	Svc          *service.Service
	Builder      *builder.Builder
	Deployer     *deployer.Deployer
	Concurrency  int
	BuildTimeout time.Duration
	Log          *slog.Logger
}

// Run processes jobs until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	if err := w.Svc.FailInterrupted(ctx); err != nil {
		w.Log.Error("recover interrupted jobs", "err", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < max(1, w.Concurrency); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				job, err := w.Svc.ClaimJob(ctx)
				if err != nil {
					w.Log.Error("claim job", "err", err)
				}
				if job == nil {
					select {
					case <-ctx.Done():
					case <-time.After(2 * time.Second):
					}
					continue
				}
				w.process(ctx, job)
			}
		}()
	}
	wg.Wait()
}

// RunExpiry checks subscription expiry, plan reminders and storage usage
// (database size limits) every interval.
func (w *Worker) RunExpiry(ctx context.Context, interval time.Duration) {
	for {
		if n, err := w.Svc.ExpireSubscriptions(ctx); err != nil {
			w.Log.Error("expire subscriptions", "err", err)
		} else if n > 0 {
			w.Log.Info("subscriptions expired", "count", n)
		}
		if n, err := w.Svc.SendReminders(ctx); err != nil {
			w.Log.Error("plan reminders", "err", err)
		} else if n > 0 {
			w.Log.Info("plan reminders sent", "count", n)
		}
		if err := w.Svc.CheckStorage(ctx); err != nil {
			w.Log.Error("check storage", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (w *Worker) process(ctx context.Context, j *service.Job) {
	log := newLogSink(w.Svc, j.DeploymentID)
	defer log.Close()
	w.Log.Info("deployment started", "app", j.AppName, "deployment", j.DeploymentID, "kind", j.Kind)

	jobCtx, cancel := context.WithTimeout(ctx, w.BuildTimeout+5*time.Minute)
	defer cancel()
	start := time.Now()
	took := func() string { return service.FormatDuration(time.Since(start)) }
	if j.Kind == "detect" {
		w.detect(jobCtx, ctx, j, log)
		return
	}
	err := w.run(jobCtx, j, log)
	if w.Svc.JobCancelled(ctx, j.DeploymentID) {
		w.Log.Info("deployment cancelled", "app", j.AppName, "deployment", j.DeploymentID)
		return
	}
	if err != nil {
		fmt.Fprintf(log, "\n==> FAILED after %s: %v\n", took(), err)
	} else {
		fmt.Fprintf(log, "\n==> Deployment succeeded in %s\n", took())
	}
	log.Close()
	if ferr := w.Svc.FinishJob(context.WithoutCancel(ctx), j, err); ferr != nil {
		w.Log.Error("finish job", "err", ferr)
	}
	if err == nil { // the previous container is gone: a removed volume can go too
		w.Svc.CleanupStorage(context.WithoutCancel(ctx), j.AppID, j.AppName)
	}
	w.Log.Info("deployment finished", "app", j.AppName, "deployment", j.DeploymentID, "err", err)
}

// detect analyses a repository and leaves the result for the student to review.
func (w *Worker) detect(jobCtx, ctx context.Context, j *service.Job, log *logSink) {
	start := time.Now()
	r, err := repo.Parse(j.RepoURL)
	var d stacks.Detection
	if err == nil {
		d, err = w.Builder.Detect(jobCtx, builder.Spec{AppID: j.AppID, AppName: j.AppName, DeploymentID: j.DeploymentID, Repo: r, Branch: j.Branch, Stack: j.Stack}, log)
	}
	if err == nil {
		fmt.Fprintf(log, "==> Analysis finished in %s\n", service.FormatDuration(time.Since(start)))
	}
	log.Close()
	if w.Svc.JobCancelled(ctx, j.DeploymentID) {
		return
	}
	if ferr := w.Svc.FinishDetect(context.WithoutCancel(ctx), j, d, err); ferr != nil {
		w.Log.Error("finish detect", "err", ferr)
	}
	w.Log.Info("analysis finished", "app", j.AppName, "language", d.Language, "framework", d.Framework, "err", err)
}

func (w *Worker) run(ctx context.Context, j *service.Job, log *logSink) error {
	image := j.Image
	if image == "" {
		r, err := repo.Parse(j.RepoURL)
		if err != nil {
			return err
		}
		if w.Svc.RepoBanned(ctx, r.CloneURL) {
			return errors.New("this repository is not allowed on FodyCloud")
		}
		buildCtx, cancel := context.WithTimeout(ctx, w.BuildTimeout)
		defer cancel()
		buildStart := time.Now()
		_, env, _, err := w.Svc.JobConfig(ctx, j)
		if err != nil {
			return err
		}
		img, commit, err := w.Builder.Build(buildCtx, builder.Spec{
			AppID: j.AppID, AppName: j.AppName, DeploymentID: j.DeploymentID, Repo: r, Branch: j.Branch,
			Stack: j.Stack, Env: env,
		}, log)
		if errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("build timed out after %s", w.BuildTimeout)
		}
		if err != nil {
			return err
		}
		if err := w.Svc.SetJobBuilt(ctx, j.DeploymentID, img, commit); err != nil {
			return err
		}
		fmt.Fprintf(log, "==> Build finished in %s\n", service.FormatDuration(time.Since(buildStart)))
		image = img
	} else {
		fmt.Fprintf(log, "==> Reusing image %s (%s)\n", image, j.Kind)
	}
	if w.Svc.JobCancelled(ctx, j.DeploymentID) {
		return errors.New("cancelled")
	}
	if err := w.Svc.SetJobStatus(ctx, j.DeploymentID, "deploying"); err != nil {
		return err
	}
	plan, env, domains, err := w.Svc.JobConfig(ctx, j)
	if err != nil {
		return err
	}
	volume, cache, err := w.Svc.JobStorage(ctx, j, plan)
	if err != nil {
		return err
	}
	deployStart := time.Now()
	if err := w.Deployer.Deploy(ctx, deployer.Spec{
		AppName: j.AppName, UserID: j.UserID, DeploymentID: j.DeploymentID,
		Image: image, Plan: plan, Env: env, CustomDomains: domains, HealthPath: j.HealthPath,
		Volume: volume, Cache: cache,
	}, log); err != nil {
		return err
	}
	fmt.Fprintf(log, "==> App started in %s\n", service.FormatDuration(time.Since(deployStart)))
	return nil
}

// logSink buffers job output and flushes it to the database every second, so
// clients can follow builds live.
type logSink struct {
	svc    *service.Service
	id     int64
	mu     sync.Mutex
	buf    bytes.Buffer
	done   chan struct{}
	closed bool
	wg     sync.WaitGroup
}

func newLogSink(svc *service.Service, id int64) *logSink {
	l := &logSink{svc: svc, id: id, done: make(chan struct{})}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				l.flush()
			case <-l.done:
				l.flush()
				return
			}
		}
	}()
	return l
}

func (l *logSink) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logSink) flush() {
	l.mu.Lock()
	chunk := l.buf.String()
	l.buf.Reset()
	l.mu.Unlock()
	if chunk != "" {
		l.svc.AppendLog(context.Background(), l.id, chunk) //nolint:errcheck
	}
}

// Close flushes the remaining output; safe to call more than once.
func (l *logSink) Close() {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	l.closed = true
	l.mu.Unlock()
	close(l.done)
	l.wg.Wait()
}
