// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/fodydev/fodycloud/internal/plans"
	"github.com/fodydev/fodycloud/internal/stacks"
)

// Job is a claimed deployment for the worker.
type Job struct {
	DeploymentID int64
	Kind         string
	Image        string
	AppID        int64
	AppName      string
	UserID       int64
	RepoURL      string
	Branch       string
	Stack        stacks.Stack
	HealthPath   string
}

// ClaimJob atomically takes the oldest queued deployment, or returns nil.
func (s *Service) ClaimJob(ctx context.Context) (*Job, error) {
	var j Job
	err := s.DB.QueryRow(ctx, `UPDATE deployments d SET status='building', started_at=now()
		FROM apps a WHERE a.id=d.app_id AND d.id = (
			SELECT id FROM deployments WHERE status='queued' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING d.id, d.kind, d.image, a.id, a.name, a.user_id, a.repo_url, a.branch,
			a.stack_language, a.stack_framework, a.stack_version, a.build_command, a.start_command, a.publish_dir, a.root_dir, a.health_path`).
		Scan(&j.DeploymentID, &j.Kind, &j.Image, &j.AppID, &j.AppName, &j.UserID, &j.RepoURL, &j.Branch,
			&j.Stack.Language, &j.Stack.Framework, &j.Stack.Version, &j.Stack.BuildCommand, &j.Stack.StartCommand, &j.Stack.PublishDir, &j.Stack.RootDir, &j.HealthPath)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &j, err
}

// JobConfig returns what the deployer needs for a job.
func (s *Service) JobConfig(ctx context.Context, j *Job) (plans.Plan, map[string]string, []string, error) {
	p, err := s.PlanFor(ctx, j.UserID)
	if err != nil {
		return p, nil, nil, err
	}
	env, err := s.appEnv(ctx, j.AppID)
	if err != nil {
		return p, nil, nil, err
	}
	// The storage's connection variables, at build time too (frameworks
	// read them while building, e.g. Laravel's config:cache).
	storage, err := s.storageEnv(ctx, j.AppID)
	if err != nil {
		return p, nil, nil, err
	}
	for k, v := range storage {
		env[k] = v
	}
	doms, err := s.domains(ctx, j.AppID, true)
	return p, env, doms, err
}

// AppendLog appends output to a deployment log (the last 1 MB is kept) and
// pushes it to the log's live followers.
func (s *Service) AppendLog(ctx context.Context, deploymentID int64, chunk string) error {
	var size int64
	err := s.DB.QueryRow(ctx, `UPDATE deployments SET log = right(log || $2, 1048576), log_size = log_size + char_length($2)
		WHERE id=$1 RETURNING log_size`, deploymentID, chunk).Scan(&size)
	if err != nil {
		return err
	}
	s.hub().publish(deploymentID, LogEvent{Text: chunk, End: size})
	return nil
}

// SetJobStatus updates the status of a running job.
func (s *Service) SetJobStatus(ctx context.Context, deploymentID int64, status string) error {
	_, err := s.DB.Exec(ctx, `UPDATE deployments SET status=$2 WHERE id=$1`, deploymentID, status)
	if err == nil {
		s.hub().publish(deploymentID, LogEvent{Status: status})
	}
	return err
}

// SetJobBuilt records the built image and commit.
func (s *Service) SetJobBuilt(ctx context.Context, deploymentID int64, image, commit string) error {
	_, err := s.DB.Exec(ctx, `UPDATE deployments SET image=$2, commit_sha=$3 WHERE id=$1`, deploymentID, image, commit)
	return err
}

// JobCancelled reports whether the app was deleted or the job cancelled meanwhile.
func (s *Service) JobCancelled(ctx context.Context, deploymentID int64) bool {
	var status string
	err := s.DB.QueryRow(ctx, `SELECT status FROM deployments WHERE id=$1`, deploymentID).Scan(&status)
	return err != nil || status == "cancelled"
}

// FinishJob records the outcome of a job.
func (s *Service) FinishJob(ctx context.Context, j *Job, jobErr error) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if jobErr == nil {
		if _, err := tx.Exec(ctx, `UPDATE deployments SET status='superseded' WHERE app_id=$1 AND status='running' AND id<>$2`, j.AppID, j.DeploymentID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE deployments SET status='running', finished_at=now() WHERE id=$1`, j.DeploymentID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE apps SET status='running', current_deployment_id=$2, updated_at=now() WHERE id=$1`, j.AppID, j.DeploymentID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `UPDATE deployments SET status='failed', error=$2, finished_at=now() WHERE id=$1`, j.DeploymentID, jobErr.Error()); err != nil {
			return err
		}
		// Keep serving the previous deployment if there is one.
		if _, err := tx.Exec(ctx, `UPDATE apps SET status = CASE WHEN current_deployment_id IS NULL THEN 'failed' ELSE status END,
			updated_at=now() WHERE id=$1`, j.AppID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	status := "running"
	if jobErr != nil {
		status = "failed"
	}
	s.hub().publish(j.DeploymentID, LogEvent{Status: status})
	return nil
}

// FailInterrupted marks jobs left running by a previous process as failed.
func (s *Service) FailInterrupted(ctx context.Context) error {
	_, err := s.DB.Exec(ctx, `UPDATE deployments SET status='failed', error='interrupted by a control-plane restart', finished_at=now()
		WHERE status IN ('building','deploying')`)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `UPDATE apps SET status='failed' WHERE status='building' AND current_deployment_id IS NULL
		AND NOT EXISTS (SELECT 1 FROM deployments d WHERE d.app_id=apps.id AND d.status='queued')`)
	if err != nil {
		return err
	}
	// An interrupted analysis lets the student choose the stack by hand.
	_, err = s.DB.Exec(ctx, `UPDATE apps SET status='review' WHERE status='detecting'
		AND NOT EXISTS (SELECT 1 FROM deployments d WHERE d.app_id=apps.id AND d.status='queued')`)
	return err
}
