# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

- feat: add a CQRS `/trigger` command consumer so an operator can force a specific draft PR into the dark-factory-implement pipeline immediately (skip the poll) and re-run an already-processed PR (`force=true`). Re-adds an in-pod command subsystem (mirrors the pr-review/releaser/build watchers): `TriggerCommand{url,force}` on schema `maintainer-githubdarkfactory-v1` (bumped `maintainer` → v0.47.0 for `GithubDarkFactoryV1SchemaID`), consumed by a 3rd `run.Func` whose executor runs `GetPRDetails → darkfactory.Evaluate` (same candidate gate as the poll) → `DeriveTaskID`/`DeriveTaskIDForce` (salted nonce for force) → `BuildCreateCommand` → Kafka. Non-candidate PRs are skipped (`ErrCommandObjectSkipped`); transient GitHub/Kafka errors retry. Adds `pkg/memdb.go` offset store + a thin `/trigger` HTTP handler for parity.

## v0.1.0

- feat: initial poll-only watcher — scans a GitHub scope for open DRAFT PRs that carry an approved-not-completed dark-factory spec in the PR diff (`.dark-factory.yaml` present, `specs/in-progress/*.md` ∩ diff with `approved:` set and `completed:` unset, and no `prompts/in-progress/*.md` in flight) and emits one `dark-factory-implement` CreateTaskCommand per (PR, head SHA) to Kafka for the `github-dark-factory-agent`. Reuses the fleet watcher skeleton (persistent cursor, REPO_ALLOWLIST scope filter, UUID5 `task_identifier`, Prometheus metrics, GitHub App auth, Kafka task sender) with a new random task-id namespace and a network-backed candidate evaluator; drops the pr-review trust/override/`/trigger`-consumer machinery.
