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
   whole lifecycle, so an existence-only check would false-trigger every unrelated draft PR;
4. no `prompts/in-progress/*.md` remain at the head ref (self-trigger suppression —
   the agent's own in-flight commits keep the spec approved-not-completed while it works).

For each kept PR it publishes a `dark-factory-implement` task keyed by a
deterministic UUID5 `task_identifier` derived from `(owner, repo, number, head_sha)`.
The cursor at `/data/cursor.json` (last-seen update time + `task_id → head_sha` map)
is persisted between polls so a restart does not re-trigger every known PR, and a
force-push (new head SHA) re-submits a fresh task.

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
