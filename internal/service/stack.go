// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"encoding/json"

	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/stacks"
)

// ConfirmStack records the student's choice of language and framework (the
// detected one, or another) and queues a build. Changing the stack later
// goes through the same call and triggers a rebuild.
func (s *Service) ConfirmStack(ctx context.Context, u *auth.User, name string, st stacks.Stack) (*Deployment, error) {
	if err := s.canBuild(u); err != nil {
		return nil, err
	}
	if err := st.Normalize(); err != nil {
		return nil, invalid("%s", err.Error())
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	if err := s.checkRepoAllowed(ctx, a); err != nil {
		return nil, err
	}
	if a.Status == "detecting" {
		return nil, invalid("the repository is still being analysed; wait for the result before confirming")
	}
	if _, err := s.DB.Exec(ctx, `UPDATE apps SET stack_language=$2, stack_framework=$3, stack_version=$4, build_command=$5,
		start_command=$6, publish_dir=$7, root_dir=$8, stack_confirmed=true, updated_at=now() WHERE id=$1`,
		a.ID, st.Language, st.Framework, st.Version, st.BuildCommand, st.StartCommand, st.PublishDir, st.RootDir); err != nil {
		return nil, err
	}
	return s.enqueue(ctx, a.ID, "build", "")
}

// Analyse queues a new detection of the repository (e.g. after the student
// pushed a fix); it doesn't change the confirmed stack. rootDir, when not
// nil, sets the folder to analyse ("" = the repository root, then likely
// subfolders); it can only change before the stack is confirmed (afterwards
// the working directory is part of the stack).
func (s *Service) Analyse(ctx context.Context, u *auth.User, name string, rootDir *string) (*Deployment, error) {
	if err := requireWrite(u); err != nil {
		return nil, err
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	if rootDir != nil {
		dir, err := stacks.CleanRootDir(*rootDir)
		if err != nil {
			return nil, invalid("%s", err.Error())
		}
		if dir != a.Stack.RootDir {
			if a.StackConfirmed {
				return nil, invalid("the app's stack is confirmed: change its working directory in the stack settings")
			}
			if _, err := s.DB.Exec(ctx, `UPDATE apps SET root_dir=$2, updated_at=now() WHERE id=$1`, a.ID, dir); err != nil {
				return nil, err
			}
		}
	}
	return s.enqueue(ctx, a.ID, "detect", "")
}

// FinishDetect stores the result of an analysis and asks for review.
func (s *Service) FinishDetect(ctx context.Context, j *Job, d stacks.Detection, jobErr error) error {
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	status, msg := "done", ""
	if jobErr != nil {
		status, msg = "failed", jobErr.Error()
	}
	if _, err := s.DB.Exec(ctx, `UPDATE deployments SET status=$2, error=$3, finished_at=now() WHERE id=$1`, j.DeploymentID, status, msg); err != nil {
		return err
	}
	s.hub().publish(j.DeploymentID, LogEvent{Status: status})
	// Unconfirmed apps wait for review; confirmed apps keep their status.
	_, err = s.DB.Exec(ctx, `UPDATE apps SET detection=$2,
		status = CASE WHEN stack_confirmed THEN (CASE WHEN status='detecting' THEN 'review' ELSE status END) ELSE 'review' END,
		updated_at=now() WHERE id=$1`, j.AppID, raw)
	return err
}
