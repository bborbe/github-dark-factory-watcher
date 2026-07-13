// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the github-dark-factory-watcher binary.
package factory

import (
	"context"
	"net/http"

	task "github.com/bborbe/agent/command/task"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/cqrs/cdb"
	"github.com/bborbe/github-dark-factory-watcher/pkg"
	"github.com/bborbe/github-dark-factory-watcher/pkg/filter"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/maintainer/githubapp"
)

// CreateGitHubAppClient creates an HTTP client authenticated as a GitHub App installation.
func CreateGitHubAppClient(
	ctx context.Context,
	appID int64,
	installationID int64,
	pemKey []byte,
) (*http.Client, error) {
	cfg := githubapp.Config{
		AppID:          appID,
		InstallationID: installationID,
		PEM:            pemKey,
	}
	return githubapp.NewClient(ctx, cfg)
}

// CreateKafkaSender constructs a typed create-task command sender backed by a Kafka sync producer.
func CreateKafkaSender(
	syncProducer libkafka.SyncProducer,
	topicPrefix base.TopicPrefix,
) task.CreateCommandSender {
	sender := cdb.NewCommandObjectSender(syncProducer, topicPrefix, log.DefaultSamplerFactory)
	return task.NewCreateCommandSender(sender, "")
}

// CreateWatcher wires all dependencies and returns a ready-to-use Watcher.
func CreateWatcher(
	ghClient pkg.GitHubClient,
	createSender task.CreateCommandSender,
	metrics pkg.Metrics,
	cursorPath string,
	startTime libtime.DateTime,
	scope string,
	scopeFilter filter.TaskCreationFilter,
	cfg pkg.TaskConfig,
) pkg.Watcher {
	return pkg.NewWatcher(
		ghClient,
		createSender,
		metrics,
		cursorPath,
		startTime,
		scope,
		scopeFilter,
		cfg,
	)
}
