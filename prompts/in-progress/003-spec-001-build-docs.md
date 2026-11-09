---
status: approved
spec: [001-cluster-build-arm]
created: "2026-11-09T11:42:00Z"
queued: "2026-11-09T11:49:21Z"
branch: feat/cluster-build-arm
---

<summary>
- The README gains a new `Build + Deploy` section documenting both build arms.
- It shows the exact cluster invocation and states that the cluster arm requires a caller-supplied tag.
- It states that both arms publish to the same Docker Hub repository and that `buca` stays local-only.
- It records the two upstream repositories and their pinned commit SHAs that the copied runner came from, so a later maintainer can tell a deliberate copy from drift.
- The existing `Development` section keeps its three local commands and is left unchanged.
- The CHANGELOG already carries the `Unreleased` bullet from the earlier prompt; it is verified, not duplicated.
- The documented build path now matches what the Makefiles and Dockerfile actually do.
</summary>

<objective>
Add a `## Build + Deploy` section to `README.md` that documents both build arms, the required caller-supplied tag on the cluster arm, the single shared Docker Hub repository, the local-only `buca`, and the provenance of the copied `hack/` runner; and confirm the `CHANGELOG.md` `## Unreleased` entry is present with a bullet.
</objective>

<context>
Read these files before making changes:
- `/workspace/README.md` — the new section goes between the end of `## Development` and `## License`. `## Development` currently holds the three local commands and must not be rewritten.
- `/workspace/CHANGELOG.md` — `## Unreleased` already exists with a bullet added by prompt 1.
- `/workspace/specs/in-progress/001-cluster-build-arm.md` — Desired Behavior 7 and Acceptance Criterion 6 (the grep evidence).
- `/workspace/docs/dod.md` — definition of done.

Reference docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/documentation-guide.md`

This is prompt 3 of the spec and is docs-only. Prompt 1 (the dispatch) is merged at `614be7c` and prompt 2 (the `hack/` runner and module-mode Dockerfile) is the sibling change. Do not modify any Makefile, the Dockerfile, `.dark-factory.yaml`, `.maintainer.yaml` or anything under `hack/`.
</context>

<requirements>
1. Insert a new `## Build + Deploy` section into `/workspace/README.md` immediately after the `## Development` code block and before `## License`. Add exactly one `## Build + Deploy` heading. Leave the `## Development` section and its three commands unchanged.

2. The section content must be exactly this (the outer fence here is four backticks; write the content with the three-backtick fences shown inside):

````markdown
## Build + Deploy

`make build` has two arms, selected by `BUILD_TARGET`.

The local arm is the default. It builds the image on this machine and tags
`docker.io/bborbe/github-dark-factory-watcher:<tag>`:

```bash
make build
```

The cluster arm builds a named commit as a Kubernetes Job and pushes the same
repository:

```bash
make build BUILD_TARGET=<cluster> REF=<sha> VERSION=<tag>
```

`<cluster>` is one of the `hack/cluster-alias.sh` aliases (`sun`, `nukedev`,
`nukeprod`) or a kubectl context. `VERSION` is required on the cluster arm — a
tag nobody chose is never pushed. `REF` must be a full 40-character SHA that
exists on `origin`, because the Job clones the repository.

Both arms compose `docker.io/bborbe/github-dark-factory-watcher` from the same
`DOCKER_REGISTRY` and `SERVICE` variables, so the two cannot drift. `buca`
stays local-only: it builds, pushes and removes an image on this machine and is
refused for a cluster `BUILD_TARGET`.

`hack/` holds the copied build runner. It is not maintained here:

- `build-cluster.sh`, `cluster-alias.sh` and `build-job.yaml` are copied from
  [`bborbe/agent-task-executor`](https://github.com/bborbe/agent-task-executor)
  at `91a7e64e160f30be8af0aea64635e1e11bc86731`.
- `build-cluster-bootstrap.sh` and `dockerconfig.json.tpl` are copied from
  [`bborbe/go-skeleton`](https://github.com/bborbe/go-skeleton) at
  `eb050aa20bb6194d8af7e47baf8880df789bd1ca`.

Treat a local edit to those files as drift: re-copy from the pinned commits
instead of patching them in place.
````

3. The section must contain, verbatim:
   - the invocation `make build BUILD_TARGET=<cluster> REF=<sha> VERSION=<tag>`
   - the reference `docker.io/bborbe/github-dark-factory-watcher`
   - the word `buca`
   - `agent-task-executor` and the SHA `91a7e64e160f30be8af0aea64635e1e11bc86731`
   - `go-skeleton` and the SHA `eb050aa20bb6194d8af7e47baf8880df789bd1ca`
   Do not paraphrase these tokens; the acceptance criteria grep for them literally.

4. `CHANGELOG.md`: verify `## Unreleased` exists with at least one `- ` bullet. It already contains the prompt-1 bullet beginning `- feat: Add a cluster build arm ...`. Leave that bullet unchanged. If (and only if) the heading or the bullet is absent, add one bullet under `## Unreleased` with a conventional prefix (`feat:` for this change) and a specific description. Never add a second bullet for the same change.

5. Do not move, delete or rewrite the CHANGELOG preamble (`# Changelog` title through the SemVer bullets). The `## Unreleased` section stays directly above the newest `## vX.Y.Z` section.

6. Do not add `## Build + Deploy` anywhere other than the one placement described, and do not add a table of contents or reorder existing README sections.
</requirements>

<constraints>
- Do not run `git`: the container has `.git` masked. Use filesystem checks only.
- Pass `ROOTDIR=/workspace` on every `make` invocation; without it `include $(ROOTDIR)/tools.env` resolves to `/tools.env` and every target dies at parse time.
- This is a documentation-only change: no Makefile, Dockerfile, `hack/`, `.dark-factory.yaml` or `.maintainer.yaml` edits.
- `## Development` keeps its three local commands; the new section is separate.
- The CHANGELOG keeps a flat bullet list under `## Unreleased`, with a conventional prefix and no `### Added`/`### Fixed` sub-headings.
- Existing tests must still pass.
- Do not commit — dark-factory handles git.
</constraints>

<verification>
Run from `/workspace` (never use git):

```bash
cd /workspace

# README evidence required by AC6.
test "$(grep -c 'BUILD_TARGET=<cluster> REF=<sha> VERSION=<tag>' README.md)" -ge 1
test "$(grep -c 'docker.io/bborbe/github-dark-factory-watcher' README.md)" -ge 1
test "$(grep -c '^## Build + Deploy$' README.md)" -eq 1
test "$(grep -c 'agent-task-executor' README.md)" -ge 1
test "$(grep -c 'go-skeleton' README.md)" -ge 1
test "$(grep -c '91a7e64e160f30be8af0aea64635e1e11bc86731' README.md)" -ge 1
test "$(grep -c 'eb050aa20bb6194d8af7e47baf8880df789bd1ca' README.md)" -ge 1
test "$(grep -c 'buca' README.md)" -ge 1

# CHANGELOG evidence required by AC6: heading once, at least one real bullet under it.
test "$(grep -c '^## Unreleased$' CHANGELOG.md)" -eq 1
test "$(awk '/^## Unreleased$/{f=1;next} /^## /{f=0} f' CHANGELOG.md | grep -c '^- ')" -ge 1

# Development section survived with its three commands.
test "$(grep -c '^## Development$' README.md)" -eq 1
grep -q 'make precommit' README.md

# No stale BUILD_TARGET documentation outside the allow-listed files.
test "$(grep -rn 'BUILD_TARGET' --exclude-dir=vendor --exclude-dir=prompts --exclude-dir=.git . | grep -vc 'Makefile.docker\|README.md\|CHANGELOG.md\|specs/\|hack/')" -eq 0

# Definition of done.
ROOTDIR=/workspace make precommit
```
</verification>
