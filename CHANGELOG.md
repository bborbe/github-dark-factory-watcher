# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v0.3.2

- fix: align the Dockerfile Go pin with `go.mod` — `FROM golang:1.26.6` against `go 1.27.0` made the image build fail at `RUN go build` with `go: go.mod requires go >= 1.27.0 (running go 1.26.6; GOTOOLCHAIN=local)`. The service ran in prod on a stale image that could no longer be rebuilt. A scan of all 18 bborbe split repos on 2026-08-30 found this was the only mismatch; every other repo agrees at `1.27.0`. It drifted alone because this repo is missing from the weekly rebuild runbook's maintainer list, so no run ever built it, and the runbook's Go-pin consistency check only scans the trading repo.

## v0.3.1

- chore: update Go to 1.27.0 and github.com/bborbe/agent to v0.84.0, github.com/bborbe/cqrs to v0.6.9, github.com/bborbe/errors to v1.6.0, github.com/bborbe/http to v1.26.25, github.com/bborbe/kafka to v1.25.9, github.com/bborbe/kv to v1.21.12, github.com/bborbe/log to v1.6.25, github.com/bborbe/maintainer to v0.50.3, github.com/bborbe/run to v1.10.0, github.com/bborbe/sentry to v1.10.0, github.com/bborbe/service to v1.10.10, github.com/bborbe/time to v1.27.11, github.com/bborbe/validation to v1.4.23, github.com/onsi/gomega to v1.43.0

## v0.3.0

- feat: opt into `autoMerge.trivial` for mechanically-trivial update PRs

## v0.2.2

- chore: Bump errcheck to v1.20.0 and golangci-lint to v2.13.1 for Go 1.27 support
## v0.2.1

- update Go to 1.26.6 and update dependencies

## v0.2.0

- feat: add a CQRS `/trigger` command consumer so an operator can force a specific draft PR into the dark-factory-implement pipeline immediately (skip the poll) and re-run an already-processed PR (`force=true`). Re-adds an in-pod command subsystem (mirrors the pr-review/releaser/build watchers): `TriggerCommand{url,force}` on schema `maintainer-githubdarkfactory-v1` (bumped `maintainer` → v0.47.0 for `GithubDarkFactoryV1SchemaID`), consumed by a 3rd `run.Func` whose executor runs `GetPRDetails → darkfactory.Evaluate` (same candidate gate as the poll) → `DeriveTaskID`/`DeriveTaskIDForce` (salted nonce for force) → `BuildCreateCommand` → Kafka. Non-candidate PRs are skipped (`ErrCommandObjectSkipped`); transient GitHub/Kafka errors retry.

## v0.1.0

- feat: initial poll-only watcher — scans a GitHub scope for open DRAFT PRs that carry an approved-not-completed dark-factory spec in the PR diff (`.dark-factory.yaml` present, `specs/in-progress/*.md` ∩ diff with `approved:` set and `completed:` unset, and no `prompts/in-progress/*.md` in flight) and emits one `dark-factory-implement` CreateTaskCommand per (PR, head SHA) to Kafka for the `github-dark-factory-agent`. Reuses the fleet watcher skeleton (persistent cursor, REPO_ALLOWLIST scope filter, UUID5 `task_identifier`, Prometheus metrics, GitHub App auth, Kafka task sender) with a new random task-id namespace and a network-backed candidate evaluator; drops the pr-review trust/override/`/trigger`-consumer machinery.
