---
status: completed
spec: [001-cluster-build-arm]
summary: Added BUILD_TARGET dispatch to Makefile.docker with a cluster build arm, caller-supplied VERSION guard, parse-time local-only refusals, and a CHANGELOG Unreleased entry.
execution_id: 7d9b909d-e8d6-5375-8f61-3e1c5b0ff377-exec-001-spec-001-makefile-dispatch
dark-factory-version: dev
created: "2026-11-09T08:24:19Z"
queued: "2026-11-09T08:37:18Z"
started: "2026-11-09T08:37:20Z"
completed: "2026-11-09T08:45:45Z"
branch: dark-factory/cluster-build-arm
---

<summary>
- `make build` gains a second build arm selected by `BUILD_TARGET`, defaulting to the existing local Docker build.
- A cluster value routes the build to the in-cluster runner and prints the exact image reference first.
- The cluster arm refuses to run unless the caller supplied `VERSION`; the newest-release default is rejected.
- The version-tag and `go mod vendor` guards now run only for the local arm.
- `upload`, `clean` and `buca` stay local-only and are refused at parse time for a cluster target.
- Both arms compose the same Docker Hub repository from the existing `DOCKER_REGISTRY` and `SERVICE`.
- No new build knobs are introduced: the clone URL and push-secret name are fixed literals.
- The local arm's command and tag are byte-for-byte unchanged.
</summary>

<objective>
Make `Makefile.docker` dispatch between two build arms. With `BUILD_TARGET=local` (the default) `make build` behaves exactly as today. With any other `BUILD_TARGET` it prints `$(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION)` and then invokes `hack/build-cluster.sh` with the repository URL and push-secret name as literals, while refusing to run without a caller-supplied `VERSION` and refusing the local-only `upload`/`clean`/`buca` goals at parse time.
</objective>

<context>
Read these files before editing:
- `/workspace/Makefile` — include order and where `SERVICE` is assigned (after the includes).
- `/workspace/Makefile.docker` — the file you are rewriting.
- `/workspace/Makefile.precommit` — `ROOTDIR` and the `tools.env` include.
- `/workspace/Makefile.variables` — `ROOTDIR` default.
- `/workspace/docs/dod.md` — definition of done.
- `/workspace/specs/in-progress/001-cluster-build-arm.md` — Desired Behaviors 1-5 and 8, Constraints, Failure Modes.

Reference docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-makefile-commands.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-dockerfile-guide.md`

This prompt is the first of three; prompts 2 and 3 follow it. Do not create `hack/` here — prompt 2 does that and this prompt's dry runs never execute the runner.
</context>

<requirements>
1. Replace the contents of `/workspace/Makefile.docker` with the content below. It has been validated with GNU make: local dry run prints `docker build` and no `hack/build-cluster.sh`; a cluster dry run prints the composed reference and the runner; an unset `VERSION` on the cluster arm aborts at parse time naming `VERSION`; `upload`/`clean`/`buca` abort at parse time for a cluster target; the local version guard still refuses an untagged build.

2. The guard must be a parse-time `$(error …)`, not a recipe-level `exit 1`: `make -n` must observe the refusal. The `VERSION` guard must sit AFTER the existing file-default assignment and must reject `$(origin VERSION)` values `file` and `undefined` (both), because the default always supplies a value and an undefined variable is a different origin.

3. `IMAGE` and the recipes must expand `$(SERVICE)` lazily: `SERVICE` is assigned in `Makefile` after `Makefile.docker` is included, so never assign `IMAGE` with `:=` at include time. Expand `$(SERVICE)` directly inside the recipe.

4. Do NOT add `BUILD_REPO_URL`, `BUILD_PUSH_SECRET`, `BUILD_TIMEOUT`, `BUILD_DOCKERFILE`, `ALLOW_DEFAULT_VERSION`, or any other knob. The repository URL and the push-secret name are literals in the dispatch. `BUILD_TARGET` and `VERSION` are the only inputs this change adds.

5. Keep `check-version-tag`, `check-go-mod`, `upload`, `clean`, `apply` and `buca` recipes otherwise unchanged. The only change to the guards is that they are prerequisites of `build` only when `BUILD_TARGET` is `local`.

Exact target content for `/workspace/Makefile.docker` (tabs, not spaces, in recipe lines):

```makefile
DOCKER_REGISTRY ?= docker.io
BUILD_TARGET ?= local

ifeq ($(VERSION),)
	VERSION := $(shell git describe --tags `git rev-list --tags --max-count=1`)
endif

# The cluster arm must never push a tag nobody chose. This guard sits AFTER the
# file default above so it judges the value that default produced, and rejects
# both `file` (the default fired) and `undefined` (no default could be found).
# Parse-time $(error): `make -n` must observe the refusal.
ifneq ($(BUILD_TARGET),local)
ifeq ($(filter $(origin VERSION),file undefined),$(origin VERSION))
$(error BUILD_TARGET=$(BUILD_TARGET) requires an explicit VERSION; pass VERSION=<tag> on the command line or in the environment)
endif
endif

# upload/clean/buca publish or remove a locally-built image and have no meaning
# for a cluster build. Parse-time $(error) so `make -n` observes it. Keyed on all
# three: `buca: build upload clean apply`, so keying on buca alone would leave
# `make upload BUILD_TARGET=...` live.
ifneq ($(BUILD_TARGET),local)
ifneq ($(filter upload clean buca,$(MAKECMDGOALS)),)
$(error upload, clean and buca are local-only and do not support BUILD_TARGET=$(BUILD_TARGET))
endif
endif

ifeq ($(BUILD_TARGET),local)
BUILD_PREREQS := check-version-tag check-go-mod
else
BUILD_PREREQS :=
endif

.PHONY: build
build: $(BUILD_PREREQS)
ifeq ($(BUILD_TARGET),local)
	@if [ -z "$(VERSION)" ]; then echo "ERROR: no VERSION set and no git tag found — set VERSION=vX.Y.Z or create a tag" >&2; exit 1; fi
	DOCKER_BUILDKIT=1 \
	docker build \
	--pull \
	--no-cache \
	--rm=true \
	--platform=linux/amd64 \
	--build-arg DOCKER_REGISTRY=$(DOCKER_REGISTRY) \
	--build-arg BUILD_GIT_COMMIT=$$(git rev-parse --short HEAD) \
	--build-arg BUILD_DATE=$$(date -u +%Y-%m-%dT%H:%M:%SZ) \
	-t $(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION) \
	-f Dockerfile .
else
	@echo $(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION)
	BUILD_TARGET='$(BUILD_TARGET)' \
	REPO_URL='https://github.com/bborbe/github-dark-factory-watcher.git' \
	REF='$(REF)' \
	TAG='$(VERSION)' \
	IMAGE='$(DOCKER_REGISTRY)/bborbe/$(SERVICE)' \
	PUSH_SECRET='buildkit-push' \
	hack/build-cluster.sh
endif

.PHONY: check-version-tag
check-version-tag:
	@if [ -n "$(ALLOW_UNTAGGED_BUILD)" ]; then \
		echo "ALLOW_UNTAGGED_BUILD set — skipping version/tag check"; \
	else \
		head_tag=$$(git describe --tags --exact-match HEAD 2>/dev/null); \
		if [ "$$head_tag" != "$(VERSION)" ]; then \
			echo "ERROR: refusing to build $(VERSION) from this tree." >&2; \
			echo "  HEAD is at tag: $${head_tag:-<untagged>}" >&2; \
			echo "  building as:    $(VERSION)" >&2; \
			echo "  An image stamped vX.Y.Z must be built from the vX.Y.Z tag." >&2; \
			echo "  Fix: git checkout $(VERSION)   (or set ALLOW_UNTAGGED_BUILD=1 for a scratch build)" >&2; \
			exit 1; \
		fi; \
	fi
.PHONY: check-go-mod
check-go-mod:
	@if [ -f "go.mod" ]; then \
		echo "go.mod found, running go mod vendor..."; \
		go mod vendor; \
	else \
		echo "go.mod not found, skipping go mod vendor."; \
	fi

.PHONY: upload
upload:
	docker push $(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION)

.PHONY: clean
clean:
	docker rmi $(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION) || true
	docker image prune -f || true
	docker builder prune --all -f || true
	rm -rf vendor

# Publish-only: no k8s in this repo. Deployment lives in the maintainer Helm
# chart (mirror + keel-pattern). apply is a no-op placeholder.
.PHONY: apply
apply:
	@echo "skip apply — publish-only"

# Operator-run publish to Docker Hub (requires `docker login`). CI runs
# `make precommit`, never `buca`, so the unconditional push is safe.
.PHONY: buca
buca: build upload clean apply
```

6. Do not change `/workspace/Makefile` or any other Makefile.
</requirements>

<constraints>
- Do not run `git`: `.dark-factory.yaml` sets `workflow: worktree`, so `.git` is masked inside this container. Commits and pushes happen host-side. Use filesystem checks, not git commands.
- Pass `ROOTDIR=/workspace` on every `make` invocation; without it `include $(ROOTDIR)/tools.env` resolves to `/tools.env` and every target dies at parse time.
- The local arm's command and tag must not change: same `docker build` flags, same `-t $(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION)`.
- `check-version-tag` must still fire for a bare local build; narrowing it by `BUILD_TARGET` is the only permitted change.
- The local-only refusal must be a parse-time `$(error …)`, not a recipe-level `exit 1`.
- Do not delete existing `upload`, `clean`, `apply`, `buca`, `check-version-tag` or `check-go-mod` targets.
- Existing tests must still pass.
- Do not commit — dark-factory handles git.
</constraints>

<verification>
Run from `/workspace` (never use git):

```bash
cd /workspace

# BUILD_TARGET and the runner invocation exist.
grep -c '^BUILD_TARGET' Makefile.docker                       # >= 1
grep -cE '^[[:space:]]*hack/build-cluster\.sh' Makefile.docker # >= 1

# Local dry run: docker build recipe, no cluster runner.
local_out="$(ROOTDIR=/workspace make -n build 2>/dev/null)"
printf '%s\n' "$local_out" | grep -q 'docker build'
if printf '%s\n' "$local_out" | grep -q 'hack/build-cluster.sh'; then echo 'local arm leaked cluster runner'; exit 1; fi

# Cluster dry run: composed reference on one line, then the runner.
cluster_out="$(ROOTDIR=/workspace make -n build BUILD_TARGET=nukeprod REF=0123456789abcdef0123456789abcdef01234567 VERSION=probe)"
printf '%s\n' "$cluster_out" | grep -q 'docker.io/bborbe/github-dark-factory-watcher:probe'
printf '%s\n' "$cluster_out" | grep -q 'hack/build-cluster.sh'

# Cluster arm refuses a VERSION it did not receive (parse-time, visible to -n).
if ROOTDIR=/workspace make -n build BUILD_TARGET=nukeprod REF=0123456789abcdef0123456789abcdef01234567 >/tmp/nover.out 2>&1; then echo 'expected refusal without VERSION'; exit 1; fi
grep -q 'VERSION' /tmp/nover.out

# Local buca still prints the push and the remove.
ROOTDIR=/workspace make -n buca 2>/dev/null | grep -cE 'docker push|docker rmi'  # >= 2

# upload/clean/buca are refused at parse time for a cluster target.
for goal in buca upload clean; do
  if ROOTDIR=/workspace make -n "$goal" BUILD_TARGET=nukeprod REF=0123456789abcdef0123456789abcdef01234567 VERSION=probe >/tmp/refuse.out 2>&1; then
    echo "expected $goal to be refused for a cluster target"; exit 1
  fi
  grep -q 'local-only' /tmp/refuse.out
done

# The local version guard still fires on an untagged HEAD (git is masked here).
if ROOTDIR=/workspace make build VERSION=probe >/tmp/guard.out 2>&1; then echo 'expected untagged local build to refuse'; exit 1; fi
grep -q 'ERROR: refusing to build' /tmp/guard.out
grep -q 'ALLOW_UNTAGGED_BUILD=1' /tmp/guard.out

# Definition of done.
ROOTDIR=/workspace make precommit
```

<!-- AC1 wording says the untagged guard "exits 1". GNU make reports the failing
     recipe as `Error 1` but exits non-zero (2). The recipe itself exits 1. The
     assertion above checks non-zero plus both error lines, which is the
     observable the AC's evidence records. -->
</verification>
