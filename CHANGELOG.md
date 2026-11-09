# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v0.7.0

- feat: Add a cluster build arm to `make build`, selected by `BUILD_TARGET` (default `local`); the cluster arm prints the composed image reference and runs `hack/build-cluster.sh`, requires a caller-supplied `VERSION`, and refuses the local-only `upload`, `clean` and `buca` goals at parse time
- feat: Add the pinned `hack/` build-runner files (`build-cluster.sh`, `cluster-alias.sh`, `build-job.yaml`, `build-cluster-bootstrap.sh`, `dockerconfig.json.tpl`) the cluster arm invokes, copied byte-identical from their pinned upstream commits, and switch the shared `Dockerfile` from `-mod=vendor` to `-mod=mod` so an image builds from a fresh clone with no `vendor/` directory

## v0.6.2

- chore: restate the do-not-merge contract in the gate workflow

## v0.6.1

- chore(dark-factory): worktree workflow with PR delivery

## v0.6.0

- feat: derive `target_vault` per repo from a new `REPO_VAULT_MAP` (`repo-glob=vault`, first match wins, unmapped repos fall back to `openclaw`), so a personal- or trading-vault PR is no longer claimed by the openclaw controller and skipped by its own

## v0.5.2

- fix: stop vetoing a PR whose prompts are queued. `Evaluate` no longer skips on `prompts/in-progress/*.md`; a queued prompt is the executor's inbox ("approved, awaiting execution"), so it now keeps the candidate and reports `Result.PromptsQueued` instead. That veto deadlocked every PR whose prompts were queued — nothing had run, and nothing ever would. Self-trigger suppression moves to the task identifier: `DeriveTaskID` gains a `promptsQueued` parameter and drops the head SHA from the key while prompts are queued (`<owner>/<repo>#<n>@queued`), so the agent's own in-progress → completed commits dedup instead of minting a fresh task per commit and spawning a second agent run on the same branch. A PR that has never triggered holds no cursor entry, so its queued prompts publish; once the queue drains the SHA returns to the key and force-push re-triggers as before. Publishing a queued PR also records an already-triggered marker in a new `Cursor.QueuedMarkers` field, keyed by `owner/repo#number`. It lives outside the map each poll rebuilds from the search batch on purpose: a PR older than the watermark, or one skipped on a transient error, is absent from a batch and would otherwise lose its entry, after which the drained commit publishes a second run under a fresh identifier. With the marker persisted, neither the commit that completes the last prompt nor the follow-up commit that marks the spec completed re-arms a run — those arrive separately, so the drained window spans more than one SHA and a SHA-keyed dedup cannot span it. The marker is dropped on `no_spec_in_diff` or `not_open`; a PR merged before either observation keeps its entry (the search is `is:pr is:open`, so a closed PR leaves the batch), so the map grows by one small entry per PR that ever had a queued-phase publish — accepted, because surviving a missing batch is what the field is for. Two counters make the new behaviour visible: `published_total{status="create_queued"}` for a queued-phase publish (the trigger this change restores) and `filter_skipped_total{reason="queued_marker"}` for the suppression it performs.

Three deliberate consequences, documented in the README: one trigger per PR per spec lifecycle (a second prompt batch after the drain needs `/trigger force: true`); a force-push does not re-trigger while a PR is inside a queued phase; and deploying over a cursor from an earlier version publishes one task per in-flight PR whose prompts are queued, because the old watcher skipped those PRs and a skipped PR is dropped from the rebuilt cursor — drain in-flight PRs before deploying if a duplicate run matters.

`ReasonPromptsInFlight` and the `prompts_in_flight` metric label are retired; `hasInFlightPrompts` is renamed `hasQueuedPrompts` and now runs after the spec-in-diff check, since it is only consulted on the Keep path.

## v0.5.1

- chore: update github.com/bborbe/agent to v0.138.0, github.com/bborbe/cqrs to v0.6.15, github.com/bborbe/errors to v1.6.6, github.com/bborbe/http to v1.26.32, github.com/bborbe/kafka to v1.29.11, github.com/bborbe/kv to v1.21.28, github.com/bborbe/log to v1.7.6, github.com/bborbe/maintainer to v0.51.0, github.com/bborbe/run to v1.11.10, github.com/bborbe/sentry to v1.10.7, github.com/bborbe/service to v1.11.2, github.com/bborbe/time to v1.27.25, github.com/bborbe/validation to v1.5.8, github.com/onsi/ginkgo/v2 to v2.34.0

## v0.5.0

- feat: log one `github non-2xx` line per non-2xx GitHub API response at the HTTP boundary, via a new `non2xxLoggingTransport` installed on the injected `*http.Client` in `NewGitHubClient` (no transport existed before). Each line carries `method`, `host`, `path`, `status`, `resource` (X-RateLimit-Resource), `remaining` (X-RateLimit-Remaining), `retry_after` and a body snippet truncated to 200 runes with `…`; a missing header or empty body is logged as the literal `absent`. Emitted at `glog.V(2)`, which the deployed `-v=2` shows. Without it a 429/403 cannot be split into primary-quota exhaustion and secondary rate limiting — go-github does not log at its HTTP boundary. The full URL and request headers are never logged; the response body is restored for the caller

## v0.4.4

- chore: update Go to 1.27.3 and github.com/bborbe/agent to v0.112.0, github.com/bborbe/kafka to v1.28.4, github.com/bborbe/maintainer to v0.50.12, github.com/bborbe/sentry to v1.10.5, github.com/bborbe/service to v1.11.0, github.com/bborbe/time to v1.27.17, github.com/bborbe/validation to v1.5.5

## v0.4.3

- chore: update github.com/bborbe/agent to v0.102.0, github.com/bborbe/http to v1.26.30, github.com/bborbe/kafka to v1.27.1, github.com/bborbe/kv to v1.21.18, github.com/bborbe/maintainer to v0.50.11, github.com/bborbe/service to v1.10.16, github.com/bborbe/validation to v1.5.4

## v0.4.2

- chore: update github.com/bborbe/maintainer to v0.50.9, github.com/bborbe/sentry to v1.10.4

## v0.4.1

- chore: update Go to 1.27.2 and github.com/bborbe/agent to v0.100.0, github.com/bborbe/cqrs to v0.6.12, github.com/bborbe/errors to v1.6.3, github.com/bborbe/http to v1.26.28, github.com/bborbe/kafka to v1.27.0, github.com/bborbe/kv to v1.21.17, github.com/bborbe/log to v1.7.4, github.com/bborbe/maintainer to v0.50.8, github.com/bborbe/run to v1.11.2, github.com/bborbe/sentry to v1.10.3, github.com/bborbe/service to v1.10.15, github.com/bborbe/time to v1.27.16, github.com/bborbe/validation to v1.5.3, github.com/onsi/ginkgo/v2 to v2.33.1, github.com/onsi/gomega to v1.44.0, github.com/prometheus/client_golang to v1.25.0

## v0.4.0

- fix: bump `osv-scanner` to v2.6.0 and `golang.org/x/net` to v0.60.0 so the Linux vulnerability gates stop failing. v2.3.1 pins `golang.org/x/tools` v0.38.0, whose SSA builder aborts with `unexpected expr: *ast.KeyValueExpr` on the promoted-field composite-literal key Go 1.27 permits in the Linux stdlib, so a repo on the old pin passes locally on darwin and fails only in Linux CI. `x/net` v0.58.0 carries `GO-2026-6603/6610/6611/6612/6617`, which fail both `vulncheck` and `trivy`.

## v0.3.6

- chore: update github.com/bborbe/kv to v1.21.15, github.com/bborbe/run to v1.10.3, github.com/bborbe/sentry to v1.10.2, github.com/bborbe/validation to v1.5.1

## v0.3.5

- chore: update Go to 1.27.1 and github.com/bborbe/agent to v0.87.6, github.com/bborbe/errors to v1.6.1, github.com/bborbe/http to v1.26.26, github.com/bborbe/kafka to v1.25.16, github.com/bborbe/kv to v1.21.14, github.com/bborbe/maintainer to v0.50.7, github.com/bborbe/service to v1.10.13, github.com/bborbe/time to v1.27.14, github.com/bborbe/validation to v1.5.0

## v0.3.4

- fix: `make build` refuses to stamp a version onto a tree that is not that version's tag (`check-version-tag`, escape hatch `ALLOW_UNTAGGED_BUILD=1`). `VERSION` defaults to the newest tag repo-wide, so an operator-run build from an untagged or older tree silently republishes under the newest tag. The guard compares `git describe --exact-match HEAD` against `$(VERSION)` and exits non-zero on mismatch.

## v0.3.3

- chore: update github.com/bborbe/agent to v0.86.0, github.com/bborbe/cqrs to v0.6.10, github.com/bborbe/kafka to v1.25.11, github.com/bborbe/kv to v1.21.13, github.com/bborbe/maintainer to v0.50.5, github.com/bborbe/run to v1.10.2, github.com/bborbe/sentry to v1.10.1, github.com/bborbe/service to v1.10.11, github.com/bborbe/time to v1.27.12

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
