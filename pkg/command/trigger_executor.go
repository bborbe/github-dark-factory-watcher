// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package command

import (
	"context"
	"strconv"

	task "github.com/bborbe/agent/command/task"
	"github.com/bborbe/cqrs/base"
	cdb "github.com/bborbe/cqrs/cdb"
	"github.com/bborbe/errors"
	"github.com/bborbe/github-dark-factory-watcher/pkg"
	"github.com/bborbe/github-dark-factory-watcher/pkg/darkfactory"
	libkv "github.com/bborbe/kv"
	libtime "github.com/bborbe/time"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/prurl"
)

// NewTriggerCommandExecutor creates a cdb.CommandObjectExecutorTx that consumes
// TriggerCommand messages and drives the single-PR dark-factory-implement
// pipeline: GitHub fetch → candidate gate (darkfactory.Evaluate) → downstream
// publish.
//
// Exit-path mapping:
//   - malformed / invalid payload             → cdb.ErrCommandObjectSkipped
//   - unparseable URL                         → cdb.ErrCommandObjectSkipped
//   - non-candidate PR (result.Keep == false) → cdb.ErrCommandObjectSkipped
//   - GitHub 5xx / network error              → wrapped error (transient, retried)
//   - Evaluate infrastructure error           → wrapped error (transient, retried)
//   - downstream CreateTaskCommand send error → wrapped error (transient, retried)
//   - success                                 → nil, nil, nil
//
// SendResultEnabled is false (fire-and-forget, no result topic). Outcomes are
// counted on pkg.Metrics: IncPublished("create"/"error") on the publish path
// and IncFilterSkipped(reason) on the non-candidate skip path (same labels the
// poll-loop watcher uses).
func NewTriggerCommandExecutor(
	ghClient pkg.GitHubClient,
	createSender task.CreateCommandSender,
	taskCfg pkg.TaskConfig,
	metrics pkg.Metrics,
	currentDateTime libtime.CurrentDateTimeGetter,
) cdb.CommandObjectExecutorTx {
	return cdb.CommandObjectExecutorTxFunc(
		TriggerCommandOperation,
		false, // SendResultEnabled = false
		func(ctx context.Context, tx libkv.Tx, commandObject cdb.CommandObject) (*base.EventID, base.Event, error) {
			return runTrigger(
				ctx, tx, commandObject,
				ghClient, createSender, taskCfg, metrics, currentDateTime,
			)
		},
	)
}

// runTrigger is the work-loop for a single TriggerCommand. Splitting it out from
// the constructor keeps the closure short and makes the function directly
// testable from the package's external _test.go (the constructor returns an
// interface, not a closure).
//
// cmd.Validate is invoked here (inside unmarshalAndValidate) as defense-in-depth:
// the sender already validates before publishing, but a buggy client that
// bypasses the HTTP handler could otherwise inject garbage.
func runTrigger(
	ctx context.Context,
	_ libkv.Tx,
	commandObject cdb.CommandObject,
	ghClient pkg.GitHubClient,
	createSender task.CreateCommandSender,
	taskCfg pkg.TaskConfig,
	metrics pkg.Metrics,
	currentDateTime libtime.CurrentDateTimeGetter,
) (*base.EventID, base.Event, error) {
	cmd, err := unmarshalAndValidate(ctx, commandObject)
	if err != nil {
		return nil, nil, err
	}
	prInfo, err := prurl.ParsePRURL(ctx, cmd.URL)
	if err != nil {
		return nil, nil, errors.Wrapf(
			ctx,
			cdb.ErrCommandObjectSkipped,
			"parse url %q: %v",
			cmd.URL,
			err,
		)
	}
	details, err := ghClient.GetPRDetails(ctx, prInfo.Owner, prInfo.Repo, prInfo.Number)
	if err != nil {
		// Transient: GitHub 5xx / network error. Framework emits Failure on the
		// result topic, Kafka redelivers.
		return nil, nil, errors.Wrapf(ctx, err, "get PR details for %s", cmd.URL)
	}
	result, err := darkfactory.Evaluate(ctx, ghClient, darkfactory.Input{
		Owner:   prInfo.Owner,
		Repo:    prInfo.Repo,
		Number:  prInfo.Number,
		HeadSHA: details.HeadSHA,
		IsDraft: details.IsDraft,
		State:   details.State,
	})
	if err != nil {
		// Transient: GitHub read error while evaluating the candidate.
		return nil, nil, errors.Wrapf(ctx, err, "evaluate candidate for %s", cmd.URL)
	}
	if !result.Keep {
		// Deliberate: the PR is not a dark-factory candidate at head. Non-retryable.
		metrics.IncFilterSkipped(result.Reason)
		glog.V(2).Infof(
			"trigger executor: skipped pr=%s/%s#%d reason=%s",
			prInfo.Owner, prInfo.Repo, prInfo.Number, result.Reason,
		)
		return nil, nil, errors.Wrapf(
			ctx,
			cdb.ErrCommandObjectSkipped,
			"candidate rejected pr=%s/%s#%d reason=%s",
			prInfo.Owner, prInfo.Repo, prInfo.Number, result.Reason,
		)
	}
	return publishCreateCommand(
		ctx, prInfo, cmd, details, result, createSender, taskCfg, metrics, currentDateTime,
	)
}

// unmarshalAndValidate decodes the CommandObject payload into a typed
// TriggerCommand and runs Validate as defense-in-depth. Any failure here is a
// deliberate, non-retryable skip.
func unmarshalAndValidate(
	ctx context.Context,
	commandObject cdb.CommandObject,
) (TriggerCommand, error) {
	var cmd TriggerCommand
	if err := commandObject.Command.Data.MarshalInto(ctx, &cmd); err != nil {
		return cmd, errors.Wrapf(
			ctx,
			cdb.ErrCommandObjectSkipped,
			"malformed TriggerCommand: %v",
			err,
		)
	}
	if err := cmd.Validate(ctx); err != nil {
		return cmd, errors.Wrapf(
			ctx,
			cdb.ErrCommandObjectSkipped,
			"validate TriggerCommand: %v",
			err,
		)
	}
	return cmd, nil
}

// publishCreateCommand builds the downstream CreateTaskCommand and publishes it
// via createSender. Returns (nil, nil, wrappedErr) on transient Kafka send
// failure and (nil, nil, nil) on success.
//
// When cmd.Force is true, the published TaskIdentifier is derived from
// DeriveTaskIDForce with a time-derived nonce so the agent controller's
// vault-file dedup does not skip the publish. The nonce is intentionally not
// logged — it leaks no security-sensitive data and adds noise without operator
// benefit.
func publishCreateCommand(
	ctx context.Context,
	prInfo *prurl.PRInfo,
	cmd TriggerCommand,
	details pkg.PRDetails,
	result darkfactory.Result,
	createSender task.CreateCommandSender,
	taskCfg pkg.TaskConfig,
	metrics pkg.Metrics,
	currentDateTime libtime.CurrentDateTimeGetter,
) (*base.EventID, base.Event, error) {
	pr := pkg.PullRequest{
		Number:    prInfo.Number,
		Owner:     prInfo.Owner,
		Repo:      prInfo.Repo,
		Title:     details.Title,
		HTMLURL:   cmd.URL,
		IsDraft:   details.IsDraft,
		UpdatedAt: details.UpdatedAt,
	}
	var taskIDStr string
	if cmd.Force {
		nonce := strconv.FormatInt(currentDateTime.Now().UnixMicro(), 10)
		taskIDStr = pkg.DeriveTaskIDForce(
			prInfo.Owner, prInfo.Repo, prInfo.Number, details.HeadSHA, nonce,
		).String()
	} else {
		taskIDStr = pkg.DeriveTaskID(
			prInfo.Owner, prInfo.Repo, prInfo.Number, details.HeadSHA,
		).String()
	}

	createCmd := pkg.BuildCreateCommand(pr, details, taskIDStr, taskCfg, result.MatchedSpecs)
	if err := createSender.SendCommand(ctx, createCmd); err != nil {
		// Transient: downstream Kafka send error. Framework emits Failure, Kafka
		// redelivers. Downstream is idempotent via derived task_id.
		metrics.IncPublished("error")
		glog.Errorf("trigger executor: send create-task failed pr=%s err=%v", cmd.URL, err)
		return nil, nil, errors.Wrapf(ctx, err, "send create task command for %s", cmd.URL)
	}
	metrics.IncPublished("create")
	glog.V(2).Infof(
		"trigger executor: published task_id=%s pr=%s/%s#%d sha=%s force=%v",
		taskIDStr, prInfo.Owner, prInfo.Repo, prInfo.Number, details.HeadSHA, cmd.Force,
	)
	return nil, nil, nil
}
