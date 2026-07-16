// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package command

import (
	"context"

	task "github.com/bborbe/agent/command/task"
	"github.com/bborbe/cqrs/base"
	cdb "github.com/bborbe/cqrs/cdb"
	"github.com/bborbe/github-dark-factory-watcher/pkg"
	libkv "github.com/bborbe/kv"
	libtime "github.com/bborbe/time"
)

// RunTrigger re-exports the private runTrigger for the external test package.
// The _test.go suffix keeps this file out of production builds.
var RunTrigger = runTrigger

// Compile-time guard: keep the public surface tightly aligned with the internal
// helper. If runTrigger's signature ever drifts, this file fails to build and
// the test breakage is local.
var _ = func(
	ctx context.Context,
	tx libkv.Tx,
	obj cdb.CommandObject,
	ghClient pkg.GitHubClient,
	createSender task.CreateCommandSender,
	taskCfg pkg.TaskConfig,
	metrics pkg.Metrics,
	currentDateTime libtime.CurrentDateTimeGetter,
) (*base.EventID, base.Event, error) {
	return runTrigger(
		ctx, tx, obj,
		ghClient, createSender, taskCfg, metrics, currentDateTime,
	)
}
