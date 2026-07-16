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
	"github.com/bborbe/github-dark-factory-watcher/pkg/command"
	"github.com/bborbe/github-dark-factory-watcher/pkg/filter"
	libkafka "github.com/bborbe/kafka"
	libkv "github.com/bborbe/kv"
	"github.com/bborbe/log"
	"github.com/bborbe/run"
	libtime "github.com/bborbe/time"

	lib "github.com/bborbe/maintainer"
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

// CreateCommandConsumer wires a run.Func that consumes TriggerCommand messages
// from the github-dark-factory watcher's request topic and runs them through the
// single-PR implement pipeline (GitHub fetch → candidate gate → publish).
//
// currentDateTime is the injected libtime.CurrentDateTimeGetter passed through
// to the trigger executor so it can derive a time-salted task identifier when
// the TriggerCommand has Force=true. The non-force path does not consult the
// clock.
//
// The function is pure composition: no business logic, no conditionals. It uses
// cdb.RunCommandConsumerTxDefault (auto-wraps the transaction) per the
// go-cqrs/auto-tx-wrapper-no-manual-wrap rule — do NOT manually wrap the
// executor with kv.NewTransactionMiddleware.
func CreateCommandConsumer(
	saramaClientProvider libkafka.SaramaClientProvider,
	syncProducer libkafka.SyncProducer,
	db libkv.DB,
	ghClient pkg.GitHubClient,
	createSender task.CreateCommandSender,
	taskCfg pkg.TaskConfig,
	metrics pkg.Metrics,
	topicPrefix base.TopicPrefix,
	currentDateTime libtime.CurrentDateTimeGetter,
) run.Func {
	executors := cdb.CommandObjectExecutorTxs{
		command.NewTriggerCommandExecutor(
			ghClient,
			createSender,
			taskCfg,
			metrics,
			currentDateTime,
		),
	}
	return cdb.RunCommandConsumerTxDefault(
		saramaClientProvider,
		syncProducer,
		db,
		lib.GithubDarkFactoryV1SchemaID,
		topicPrefix,
		false, // ignoreUnsupported
		executors,
	)
}
