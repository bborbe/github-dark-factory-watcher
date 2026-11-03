# github-dark-factory-watcher

Polls the GitHub Search API for open **DRAFT** pull requests that carry an
**approved-not-completed dark-factory spec in the PR diff** and publishes a
`CreateTaskCommand` to Kafka for each new or force-pushed candidate PR, so the
[`github-dark-factory-agent`](https://github.com/bborbe/github-dark-factory-agent)
picks it up and drives the dark-factory implement lifecycle automatically.

## How It Works

The watcher runs a `user:<scope>` GitHub Search query on a configurable interval
and, for every open PR since the cursor watermark, evaluates a network-backed
candidate predicate. A PR is kept only when ALL hold at its head SHA:

1. the PR is **open** and a **draft** (re-confirmed via `pulls/N` to dodge search-index lag);
2. `.dark-factory.yaml` is present at the head ref;
3. at least one spec under `specs/in-progress/*.md` is **touched by the PR diff**
   AND is approved-but-not-completed (frontmatter `approved:` set, `completed:` unset).
   The diff-scoping is essential — specs persist in `specs/in-progress/` across their
   whole lifecycle, so an existence-only check would false-trigger every unrelated draft PR.

Queued prompts under `prompts/in-progress/*.md` are **not** a veto. They mean
"approved, awaiting execution" — the executor's inbox — so a PR holding them is a
candidate. Treating them as a self-trigger signal is what deadlocked every PR whose
prompts were queued: nothing had run yet, and nothing ever would.

For each kept PR it publishes a `dark-factory-implement` task keyed by a
deterministic UUID5 `task_identifier` derived from `(owner, repo, number, head_sha)`
— except while prompts are queued, where the key is `(owner, repo, number)` and the
head SHA is dropped. That is what carries self-trigger suppression now: the agent's
own commits move prompts `in-progress → completed`, advancing the head SHA after each
one, and a SHA-keyed identifier would mint a fresh task on every commit and spawn a
second agent run on the same branch. Pinning the key for the whole queued phase makes
those commits a controller no-op, while a PR that has never triggered has no cursor
entry, so its queued prompts finally run.

Publishing a queued PR also records an already-triggered marker in the cursor, keyed
by `owner/repo#number` and held in its own field — deliberately **not** in the map that
each poll rebuilds from the search batch, because a PR older than the watermark, or one
skipped on a transient error, is absent from a batch and would otherwise lose its entry.
The marker therefore survives the drained window, so neither the commit that completes
the last prompt nor the follow-up commit that marks the spec completed re-arms a run —
those arrive separately, and the window spans more than one SHA, which a SHA-keyed
dedup cannot span. It is dropped when the spec reaches completed (`no_spec_in_diff`) or
the PR closes (`not_open`).

Three consequences follow, all deliberate:

- **One trigger per PR per spec lifecycle.** A second prompt batch queued after the
  drain is deduped by the preserved marker. Use `/trigger` with `force: true`, which
  salts the identifier through `DeriveTaskIDForce`.
- **A force-push does not re-trigger while a PR is inside a queued phase** (queued or
  drained-but-not-completed). Again, `/trigger force: true` is the recovery route. A
  force-push on a PR whose spec has completed re-triggers as before, via the normal
  SHA-keyed path.
- **Deploying over a cursor from an earlier version publishes one task per in-flight
  PR whose prompts are queued.** The old watcher skipped those PRs, and a skipped PR
  is dropped from the rebuilt cursor, so there is no prior entry to dedup against. For
  a PR that is genuinely deadlocked that is the intended effect; for one an agent is
  already working it produces a duplicate run. Drain in-flight PRs before deploying if
  that matters.

The cursor at `/data/cursor.json` (last-seen update time + `task_id → head_sha` map)
is persisted between polls so a restart does not re-trigger every known PR.

## Emitted Task Contract

Frontmatter (per the Agent Task File Contract):

```
task_type: dark-factory-implement
assignee: github-dark-factory-agent
phase: planning
status: in_progress
stage: <dev|prod>
task_identifier: <UUID5 of owner/repo#number@head_sha>
title: Implement <owner>/<repo> PR #<n> at <sha[:7]>
repo: <owner>/<repo>
clone_url: https://github.com/<owner>/<repo>.git
ref: <full head SHA>
pr_number: <n>
branch: <head ref name>
```

The body is an operator-readable header only (PR URL, branch, HEAD, approved-spec
count). The agent's data source is the clone of the draft PR branch, not the body.

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `APP_ID` / `INSTALLATION_ID` / `PEM_KEY` | yes | — | GitHub App credentials (all three or none) |
| `KAFKA_BROKERS` | yes | — | Comma-separated Kafka broker list |
| `STAGE` | yes | — | Deployment stage (`dev` or `prod`) |
| `LISTEN` | no | `:9090` | HTTP listen address (`/healthz`, `/readiness`, `/metrics`, `/check`) |
| `POLL_INTERVAL` | no | `5m` | Poll interval (Go duration string) |
| `REPO_SCOPE` | no | `bborbe` | GitHub user or org to search for PRs |
| `REPO_ALLOWLIST` | no | — | Comma-separated host-qualified repo allowlist (`host/owner/repo`); empty means allow-all |
| `BACKFILL_DURATION` | no | `720h` (30d) | On cold start, backdate the initial cursor by this; empty disables |
| `TASK_SUFFIX` | no | — | Optional ` - <suffix>` appended to task filenames (per-stage disambiguator) |
| `TARGET_VAULT` | no | — | Vault slug stamped on every command (matched against the controller's `VAULT_NAME`) |
| `TOPIC_PREFIX` | no | — | Kafka topic prefix for CQRS topic construction (`develop`/`master`) |
| `MAX_SLUG_LEN` | no | `80` | Max length of the slugified PR-title segment in vault filenames |
| `MAX_TITLE_LEN` | no | `200` | Max length of the whole vault task filename (safety cap) |
| `SENTRY_DSN` / `SENTRY_PROXY` | no | — | Sentry error tracking |

### `REPO_ALLOWLIST` syntax

Entries are comma-separated. A leading `!` marks an exclusion. A target is allowed iff
`(includes is empty OR any include matches) AND (no exclude matches)`; excludes always
override includes. Wildcards (`github.com/bborbe/*`) are supported.

## HTTP Endpoints

| Path | Method | Purpose |
|---|---|---|
| `/healthz` | GET | Liveness probe |
| `/readiness` | GET | Readiness probe |
| `/metrics` | GET | Prometheus metrics |
| `/check` | POST | Run a poll cycle in the background; returns 200 immediately |
| `/setloglevel/{level}` | GET | Temporarily raise the glog verbosity |

## Metrics

| Metric | Labels | Purpose |
|---|---|---|
| `github_dark_factory_watcher_poll_cycles_total` | `result` (success / github_error / rate_limited) | poll health |
| `github_dark_factory_watcher_published_total` | `status` (create / skipped / error) | task emission rate |
| `github_dark_factory_watcher_repos_scanned_total` | — | sanity check on the scope filter |
| `github_dark_factory_watcher_filter_skipped_total` | `reason` | per-reason skip visibility |

## Development

```bash
make test          # run unit tests
make generate      # regenerate counterfeiter mocks
make precommit     # format + lint + test + security checks
```

## License

BSD 2-Clause License. See [LICENSE](LICENSE).
