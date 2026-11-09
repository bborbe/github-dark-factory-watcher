---
status: completed
approved: "2026-11-09T07:52:25Z"
generating: "2026-11-09T11:36:01Z"
prompted: "2026-11-09T11:42:21Z"
verifying: "2026-11-09T11:43:01Z"
completed: "2026-11-09T11:57:33Z"
branch: feat/cluster-build-arm
---

## Summary

- This repo's image can currently be produced on one machine only: the Mac.
- Its siblings already build their image inside a Kubernetes cluster, unattended, from any committed ref.
- Add that second build path here as an arm beside the existing local one, leaving the local command and tag as they are.
- The cluster arm builds a named commit as a Kubernetes Job and pushes the result to the same Docker Hub repository the local arm already tags, so a release no longer needs this machine.
- Two supporting edits make the arm reachable: the image must build from a fresh clone, which ships without its vendored dependency tree; and the repo must carry the dark-factory directory layout, which it has none of today and which this PR adds directly because no generated prompt can.

## Problem

`github-dark-factory-watcher` runs on nukedev and nukeprod, and `bborbe/nuke` mirrors its image through `dark-factory/Makefile:66` (`github-dark-factory-watcher:$(WATCHER_TAG)`). Nothing can produce that image except `docker build` on this Mac. A release cut while the operator is away from this machine cannot ship, and the fleet has a hole shaped exactly like this repo.

The arm is not a straight copy. Four properties of this repo change what has to be written:

- **The repository name is already the one the cluster credential can push.** `Makefile.docker:1` sets `DOCKER_REGISTRY ?= docker.io` and `Makefile.docker:19` tags `$(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION)`; `Makefile:5` sets `SERVICE = github-dark-factory-watcher`. Both arms therefore compose `docker.io/bborbe/github-dark-factory-watcher`, the Docker Hub repository the `buildkit-push` credential authenticates against. The pilot needed a separate `CLUSTER_IMAGE` because its local arm pushed the private mirror; **no such split is needed here** — a repo-local fact, not a template rule.
- **`build` is chained to a version guard.** `Makefile.docker:8` reads `build: check-version-tag check-go-mod`, and `check-version-tag` exits 1 unless HEAD's exact tag equals `$(VERSION)`. A scratch-tag cluster build dies at that guard before any Job is created. The guard must be wrapped for the local arm only, and must keep firing for a bare local build.
- **The image build resolves dependencies from `vendor/`.** `Dockerfile:7` runs `go build -mod=vendor`, and `.gitignore` excludes `/vendor/`, so a fresh clone has no such directory and the build fails before it starts. The Job clones; it does not see a developer's working tree.
- **`VERSION` has a file default, not a caller contract.** `Makefile.docker:3-5` assigns `VERSION` from the latest tag when the caller sets none. The cluster arm must refuse that value, because a tag nobody chose must never be pushed. The guard belongs after that assignment and must reject `$(origin VERSION)` values `file` and `undefined`.
- **The repo has none of the directories the prompt lifecycle writes into.** Git creates a directory in a clone only for a path it tracks, so `specs/in-progress/` exists (this spec is tracked there) while `prompts/`, `prompts/in-progress/`, `prompts/completed/` and `specs/completed/` have no tracked file at all, and no `.keep` exists anywhere. Measured twice on 2026-11-09: the first implement run (task `43694d77`) generated and executed prompt 1, then died at `save prompt metadata: write file: open prompts/in-progress/001-spec-001-makefile-dispatch.md: no such file or directory`; the second run (task `adfa6564`) died at the *same* call for the hygiene prompt that was meant to fix it. The metadata write happens at the end of a prompt, so **no generated prompt can create the directory it writes into** — the layout has to be in the tree before the lifecycle's first prompt. This PR adds it directly for that reason.

## Goal

`make build` on this repo produces the image locally exactly as it does today, and `make build BUILD_TARGET=<cluster> REF=<sha> VERSION=<tag>` produces it inside that cluster as a Kubernetes Job, pushing `docker.io/bborbe/github-dark-factory-watcher:<tag>`. Both arms work from the same checkout, and the cluster arm works from a fresh clone with no local preparation.

## Non-goals

- Changing the local arm's command or its tag. Both stay as they are. The shared Dockerfile does move to module resolution, because the cluster arm cannot build from a clone otherwise; the local arm runs the same Dockerfile and is unaffected by how dependencies are fetched.
- Changing anything in `.dark-factory.yaml` beyond the delivery shape. The file exists at `origin/master` (added in `v0.6.1`) with `workflow: worktree` + `pr: true`; this PR changes those two keys to the pilot's known-good `workflow: direct` + `pr: false` and leaves every other key as it is. Measured reason: `worktree` branches from `origin/<defaultBranch>`, so on a PR branch that already carries commits the run diverges from `origin/feat/cluster-build-arm` and the delivery step dies at `git fetch … (non-fast-forward)`; `pr: true` then tries to open a second PR, which the agent token refuses (`Resource not accessible by integration (createPullRequest)`). `direct` commits to the branch the task checked out, which is the PR branch.
- Changing `.maintainer.yaml`. Its `release.autoRelease: true` is the maintainer watcher's release flow, not dark-factory's.
- Touching `bborbe/nuke`'s `MIRROR_IMAGES`. The entry exists and is tag-pinned; a new tag needs a line there, which is the pin-bump step's work.
- Building at release or tag time. This spec adds the capability; *when* a build is triggered is a separate concern.
- Extracting the shared build runner into a common upstream template. The files are copied, not shared.
- Extra build knobs beyond `BUILD_TARGET` and `VERSION`. The image repository is not a knob this spec adds: both arms compose it from the pre-existing `DOCKER_REGISTRY` make variable and `SERVICE`, so the two cannot drift from each other. `DOCKER_REGISTRY` is itself overridable on both arms — that is the local arm's existing behaviour and this spec neither changes nor narrows it. The copy source also carries `ALLOW_DEFAULT_VERSION`, `BUILD_TIMEOUT`, `BUILD_REPO_URL`, `BUILD_DOCKERFILE` and `BUILD_PUSH_SECRET`. `ALLOW_DEFAULT_VERSION` in particular would reintroduce an opt-out on the refusal this spec ships, so it is not copied. The repository URL and the push-secret name are not knobs either: the dispatch passes both as literals, because a configurable clone source on a build that mounts a push credential is a hazard with no use case here.
- Multi-arch images. `linux/amd64` only, matching the cluster nodes.
- Building the image in GitHub Actions. `.github/workflows/ci.yml` runs `make precommit` only, holds no registry push credential, and the org's cluster build lane is where images are cut — adding a second, credential-bearing build path is the thing this spec exists to avoid.
- Running `hack/build-cluster-bootstrap.sh`. The bootstrap pair is copied for parity with the goal's recorded arm-source decision and is never executed here — the `build` namespace and its `buildkit-push` secret already exist in the target cluster. It is not dead code to delete; it is the provisioning path the next cluster needs.

## Acceptance Criteria

- [ ] **The local arm's command and tag are unchanged, and its version guard still fires** — *container:* with no `BUILD_TARGET` set, `ROOTDIR=/workspace make -n build` prints the `docker build` recipe and no reference to `hack/build-cluster.sh`; and on an untagged HEAD `ROOTDIR=/workspace make build VERSION=<scratch-tag>` exits 1 with `ERROR: refusing to build` plus the `ALLOW_UNTAGGED_BUILD=1` hint, which is reachable in-container because the guard is a prerequisite and fails before `docker build` runs. *Operator:* a real `make build` at a tagged HEAD exits 0 and `docker image inspect docker.io/bborbe/github-dark-factory-watcher:<tag>` exits 0; the tag-matching *discrimination* — a tagged HEAD passes, an untagged one refuses — is asserted only here, because the container run below proves the refusal path fires, not that it discriminates. Evidence: the dry-run output, the guard run's exit code with both error lines, the build's exit code and the inspected image id.
- [ ] **Post-Deploy (Rung-2):** the cluster arm builds and pushes from a fresh clone — `make build BUILD_TARGET=nukeprod REF=<full 40-character sha> VERSION=<scratch-tag>` exits 0, and `crane digest docker.io/bborbe/github-dark-factory-watcher:<scratch-tag>` resolves to a digest. `<scratch-tag>` must be a tag never built on this host, or the local-inspect half below passes for the wrong reason. `nukeprod` is the target for this verification, while sun is host-down (Assumptions). When sun returns, the same AC runs against `sun` and `deploy_check:` must follow the cluster the evidence row names — swap `kubectlnukeprod` for `kubectlsun`; the two wrappers are otherwise identical. Evidence: the exit code, the Job name, the build pod's `spec.nodeName`, the build log line naming this repo's `Dockerfile`, the resolved digest, and `docker image inspect` on the same reference failing on this Mac with `No such image` — a stopped daemon or a malformed reference does not satisfy this.
  - `deploy_check:` `kubectlnukeprod -n build get secret buildkit-push -o jsonpath='{.metadata.name}'`
  - `deploy_target:` `buildkit-push`
  - This check gates the credential *prerequisite*, not deployment freshness — this AC's observable is a fresh push from a Job, not a redeployed image, so there is no image tag to compare. Do not "fix" it into one.
- [ ] **Both arms publish to the repository the deploy consumes** — the cluster branch prints `docker.io/bborbe/github-dark-factory-watcher:<tag>` on one line before it calls the runner, so `ROOTDIR=/workspace make -n build BUILD_TARGET=nukeprod REF=<sha> VERSION=probe` shows that exact string, and it equals the reference the local arm tags. Evidence: the printed line, and the digest above resolving at exactly that repository. The `MIRROR_IMAGES` entry that carries this repository into the private registry is checked on the operator ladder, not here — `nuke/` is not mounted in the container.
- [ ] **`hack/` holds the five files, byte-identical to their pinned sources, and the cluster branch invokes the runner** — `ls -1 hack/` lists `build-cluster.sh`, `build-cluster-bootstrap.sh`, `build-job.yaml`, `cluster-alias.sh` and `dockerconfig.json.tpl`; `shasum -a 256 hack/<f>` matches its pinned digest (below); and `grep -cE '^[[:space:]]*hack/build-cluster\.sh' Makefile.docker` returns ≥ 1 on an invocation line rather than a comment. Evidence: the five names, the five digests, and the count. The listing is a filesystem read, not `git ls-tree`: the worktree *is* the committed ref, and a filesystem check holds under either workflow the config might name. The digests are what separate a copied runner from an empty `touch`ed file — `ls` and `bash -n` both pass on zero bytes.
  - `build-cluster.sh` (agent-task-executor@91a7e64) `976d4fb2be48fd25db09aa5b72a994ac7ae92269e1f75d1ade60eceef1c9b6b5`
  - `cluster-alias.sh` (agent-task-executor@91a7e64) `292922f77702353bbf9166a3c2fd0d406d0c94246a90b93852c3cf3bea118448`
  - `build-job.yaml` (agent-task-executor@91a7e64) `e2a7d9d28dc01154ae184d681ff5798c5d256597f500adf1017eaa15089f81a9`
  - `build-cluster-bootstrap.sh` (go-skeleton@eb050aa) `f2d74f6001005b103f759ad1e30dfc661625af67f4303bc898364b8ed0a1a957`
  - `dockerconfig.json.tpl` (go-skeleton@eb050aa) `bcf89cfc8cba8b36a2f0b33fdc28a1096bad349dc5207e5e23d417939433bc7a`
- [ ] **`upload` / `clean` / `buca` stay local, refused at parse time** — `ROOTDIR=/workspace make -n buca | grep -cE 'docker push|docker rmi'` returns ≥ 2; and each of `ROOTDIR=/workspace make -n buca BUILD_TARGET=nukeprod REF=<sha> VERSION=probe`, `… make -n upload BUILD_TARGET=nukeprod VERSION=probe` and `… make -n clean BUILD_TARGET=nukeprod VERSION=probe` exits non-zero and prints a message naming those targets as local-only — all three, not `buca` alone, because `buca: build upload clean apply` means a `$(error)` keyed on `buca` alone would leave `upload` pushing a locally-tagged image under a cluster-intent invocation. The refusal is a parse-time `$(error …)` keyed on `$(MAKECMDGOALS)`, so it fires under `-n` where no recipe runs. Evidence: the count, and all three exit codes with the refusal message.
- [ ] **The documented build path matches reality** — a new `README.md` `## Build + Deploy` section shows the exact cluster invocation `make build BUILD_TARGET=<cluster> REF=<sha> VERSION=<tag>`, states that `buca` remains local, records that the repository name is the same on both arms, and records where `hack/` came from — both upstream repositories and their SHAs — so a later maintainer can tell a deliberate copy from drift; `CHANGELOG.md` carries a `## Unreleased` section with a bullet for this change. Evidence: `grep -c 'BUILD_TARGET=<cluster> REF=<sha> VERSION=<tag>' README.md` ≥ 1, `grep -c 'docker.io/bborbe/github-dark-factory-watcher' README.md` ≥ 1, `grep -c '^## Build + Deploy$' README.md` = 1, `grep -c 'agent-task-executor' README.md` ≥ 1, `grep -c 'go-skeleton' README.md` ≥ 1, `grep -c '91a7e64e160f30be8af0aea64635e1e11bc86731' README.md` ≥ 1, `grep -c 'eb050aa20bb6194d8af7e47baf8880df789bd1ca' README.md` ≥ 1, `grep -c 'buca' README.md` ≥ 1, `grep -c '^## Unreleased$' CHANGELOG.md` = 1, and `awk '/^## Unreleased$/{f=1;next} /^## /{f=0} f' CHANGELOG.md | grep -c '^- '` ≥ 1 — the last one is what distinguishes a bullet from an empty heading.
- [ ] **The prompt lifecycle's directories exist in a fresh clone** — `ls -1 prompts/.keep prompts/in-progress/.keep prompts/completed/.keep specs/.keep specs/in-progress/.keep specs/completed/.keep` exits 0, and `grep -cE '^/(\.dark-factory\.(log|lock)|prompts/log|specs/log|prompts/in-progress/\*\.lock|specs/in-progress/\*\.lock)$' .gitignore` returns 6, so the runtime log and lock files stay out of the tree. Evidence: the six paths and the count. ⚠️ A tracked file creates its own directory in a clone — the four directories with no tracked file are the ones this AC exists for, and this repo tracks no `.keep` today.
- [ ] **The cluster arm refuses a tag it did not receive from the caller** — `ROOTDIR=/workspace make -n build BUILD_TARGET=nukeprod REF=<sha>` with no `VERSION` exits non-zero and names `VERSION` before any Job is rendered; the same invocation where `VERSION` would otherwise fall back to `Makefile.docker:3-5`'s file default refuses too. Evidence: both exit codes and the message. ⚠️ `make -n` is the probe because the refusal must be a parse-time `$(error …)`; a recipe-level refusal is invisible to a dry run, and `VERSION` is never truly undefined here — the file default always supplies one, so the guard must test `$(origin VERSION)` for `file` as well.

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

`Makefile.variables:1` and `Makefile.precommit:1` both set `ROOTDIR ?= $(shell git rev-parse --show-toplevel)`, and `Makefile.precommit:5` includes `$(ROOTDIR)/tools.env`. Under a workflow that masks `.git` that expansion is empty and `include /tools.env` kills every target at parse time; under `workflow: direct` it resolves correctly. **Pass `ROOTDIR=/workspace` on every `make` call in this rung** so the checks do not depend on which workflow the config names, and use filesystem checks rather than git.

- `ROOTDIR=/workspace make precommit` — format, lint, test and security checks clean
- `ROOTDIR=/workspace make test` — unit and integration suites pass
- `bash -n hack/build-cluster.sh && bash -n hack/cluster-alias.sh && bash -n hack/build-cluster-bootstrap.sh` — the copied runners parse
- `grep -c '^BUILD_TARGET' Makefile.docker` returns ≥ 1
- `grep -cE '^[[:space:]]*hack/build-cluster\.sh' Makefile.docker` returns ≥ 1
- `ls -1 hack/` lists the five expected files
- `ls -1 prompts/.keep prompts/in-progress/.keep prompts/completed/.keep specs/.keep specs/in-progress/.keep specs/completed/.keep` exits 0
- `grep -cE '^/(\.dark-factory\.(log|lock)|prompts/log|specs/log|prompts/in-progress/\*\.lock|specs/in-progress/\*\.lock)$' .gitignore` returns 6
- `ROOTDIR=/workspace make -n build` prints a `docker build` recipe and no `hack/build-cluster.sh`
- `ROOTDIR=/workspace make -n build BUILD_TARGET=nukeprod REF=<sha> VERSION=probe` prints `hack/build-cluster.sh` and the composed `docker.io/bborbe/github-dark-factory-watcher:probe` line
- `ROOTDIR=/workspace make -n build BUILD_TARGET=nukeprod REF=<sha>` with no `VERSION` exits non-zero and names `VERSION`
- `ROOTDIR=/workspace make -n buca BUILD_TARGET=nukeprod REF=<sha> VERSION=probe` exits non-zero naming the local-only targets
- `grep -rn 'BUILD_TARGET' --exclude-dir=vendor --exclude-dir=prompts --exclude-dir=.git . | grep -vc 'Makefile.docker\|README.md\|CHANGELOG.md\|specs/\|hack/'` returns 0 — every reference in the repo names the new invocation; no stale build documentation is left behind. `prompts/` is excluded because the generated prompts necessarily name `BUILD_TARGET`; `hack/` is allow-listed because the five copied runners mention it 17 times by design and Constraint 94 forbids editing them.

### Operator-executable (runs on the host after PR merge, spec verification ladder)

- `make build BUILD_TARGET=nukeprod REF=<sha> VERSION=<scratch-tag>` — one real cluster build, Job name, node and digest recorded
- `crane digest docker.io/bborbe/github-dark-factory-watcher:<scratch-tag>` — the push landed
- `grep -n 'github-dark-factory-watcher' ~/Documents/workspaces/nuke/dark-factory/Makefile` — the tag-pinned `MIRROR_IMAGES` entry that carries this repository into the private registry still names it (run here, not in the container: `nuke/` is not mounted)
- `make build` then `docker image inspect docker.io/bborbe/github-dark-factory-watcher:<tag>` — the local arm still works
- `make build VERSION=<scratch-tag>` on an untagged HEAD — the version guard still refuses

## Desired Behavior

1. **A build target selects the arm.** `BUILD_TARGET` defaults to `local`. With `local`, `make build` behaves exactly as it does today: it runs `check-version-tag`, it vendors the module tree, it runs `docker build` on this machine, and it tags `$(DOCKER_REGISTRY)/bborbe/$(SERVICE):$(VERSION)`.
2. **Any other value routes to a cluster Job.** With `BUILD_TARGET` set to a cluster alias or kubectl context, `make build` prints the composed image reference, then runs `hack/build-cluster.sh`, passing the repository URL and the push-secret name as literals. It waits for the Job and reports success or failure with the Job's own log lines.
3. **The cluster arm needs a caller-supplied tag.** The cluster arm refuses to run unless `VERSION` came from the caller — the command line, or the environment. It refuses in particular when `VERSION` resolves from the file default or is undefined, so a tag nobody chose can never be pushed. The refusal names the variable and happens before any Job is created.
4. **Both arms publish to one repository.** The cluster arm passes the repository inline as `IMAGE='$(DOCKER_REGISTRY)/bborbe/$(SERVICE)'` with `TAG='$(VERSION)'` — the same repository the local arm tags and the fleet pulls, differing only in tag, composed from the same two variables so the two cannot drift. There is no separate `IMAGE` variable: unlike the pilot, this repo has no second repository to name.
5. **The guards cover the local arm only.** `check-version-tag` and `check-go-mod` run when `BUILD_TARGET` is `local` and not otherwise: a cluster build compiles a named ref, so preparing the local working tree would be meaningless and the tag guard would refuse a scratch tag. `upload`, `clean` and `buca` remain local-only and are refused at parse time when given a cluster `BUILD_TARGET`.
6. **The cluster build works from a clone.** `Dockerfile:7` switches from `-mod=vendor` to `-mod=mod`, so the image build resolves its Go dependencies from the module proxy and a Job that clones the repo with no `vendor/` directory builds. `check-go-mod` stays on the local arm unchanged — whether it is still worth running is not this spec's concern, and removing it would change the local arm beyond the arm itself.
7. **The documented build path matches reality.** The README gains a `## Build + Deploy` section — it does not exist today; `## Development` holds only the three local commands — showing both arms, saying `buca` stays local, and recording that both arms publish to the same Docker Hub repository. The CHANGELOG records the change under `## Unreleased`.
8. **The lifecycle's own directories exist in a clone.** `prompts/`, `prompts/in-progress/`, `prompts/completed/`, `specs/`, `specs/in-progress/` and `specs/completed/` are tracked through `.keep` files, and the runtime log and lock files they produce are ignored — so a fresh clone has every directory the prompt lifecycle writes into, and a run does not dirty the tree.

## Constraints

- The local arm's command and its tag must not change. `check-go-mod`'s `go mod vendor` stays on the local branch only.
- `check-version-tag` must still fire for a bare local build — narrowing it by `BUILD_TARGET` is the only permitted change. A guard that stops refusing untagged local builds removes the protection that keeps a `vX.Y.Z` image from being cut off-tag.
- The local-only refusal for `upload` / `clean` / `buca` is a parse-time `$(error …)`, not a recipe-level `exit 1`. A recipe-level refusal is invisible to `make -n`, so the AC could not observe it.
- The `VERSION` guard must sit **after** `Makefile.docker:3-5`'s default assignment and must reject `$(origin VERSION)` values `file` **and** `undefined`. A guard testing only for `file` never fires when the variable is undefined, and one placed before the assignment sees a different origin than the one it is judging.
- `Makefile.docker` is included by `Makefile:3`, and `SERVICE` is assigned at `Makefile:5` — after the include. `IMAGE` and the recipes therefore have to expand `$(SERVICE)` lazily; a `:=` assignment at include time captures an empty value.
- The five `hack/` files are copied unmodified from two repositories, **pinned by commit** so the copy is reproducible and byte-identical across prompt runs. `hack/build-cluster.sh`, `hack/cluster-alias.sh` and `hack/build-job.yaml` from `bborbe/agent-task-executor` at `91a7e64e160f30be8af0aea64635e1e11bc86731` (its Job spec carries `automountServiceAccountToken: false`, which `go-skeleton`'s does not); `hack/build-cluster-bootstrap.sh` and `hack/dockerconfig.json.tpl` from `bborbe/go-skeleton` at `eb050aa20bb6194d8af7e47baf8880df789bd1ca`, the only repo holding those two. Fetch each as `https://raw.githubusercontent.com/bborbe/<repo>/<sha>/hack/<file>`. The bootstrap pair is copied for parity with the goal's recorded arm-source decision and is not run — the `build` namespace and its secret already exist. `cluster-alias.sh` is required at runtime — the runner sources it and fails at that line without it.
- `Dockerfile:7` changes `-mod=vendor` to `-mod=mod`. Do not instead keep `-mod=vendor` and vendor inside the Job: the Job clones a named ref and has no working tree to prepare, and Go auto-selects vendor mode whenever a `vendor/` directory exists, so a leftover local `vendor/` would mask the difference on the local arm while the cluster still failed.
- The Job template already passes `DOCKER_REGISTRY=docker.io` as a build argument, matching this Dockerfile's own default. Do not change either back to a private mirror.
- `IMAGE`, `SERVICE`, the registry and the tag must each satisfy the runner's character whitelist; no value may carry a newline.
- `.dark-factory.yaml` sets `workflow: direct`, so the container mounts the repo with a real `.git` and commits land on the branch the task checked out. `ROOTDIR=/workspace` stays on every container-rung `make` call as an explicit override rather than a workaround — it is correct under either workflow, and the checks must not depend on which one the config names.
- The new `README.md` `## Build + Deploy` section is the in-repo home for the two-arm topology; the spec dies after implementation, the README does not. It is a new section, not a rewrite of `## Development`, which keeps its three local commands.
- Track the dark-factory directory layout in the same change: `.keep` files under `prompts/`, `prompts/in-progress/`, `prompts/completed/`, `specs/`, `specs/in-progress/` and `specs/completed/`, plus `.gitignore` entries for `/.dark-factory.log`, `/.dark-factory.lock`, `/prompts/log`, `/specs/log`, `/prompts/in-progress/*.lock` and `/specs/in-progress/*.lock`. Do not delete the existing `.gitignore` entries — `/vendor/` in particular is what keeps `check-go-mod`'s output out of the tree. The pilot carries exactly this layout on `agent-sentry-issue-analyzer`'s `feat/cluster-build-arm` branch (merged as PR #141) — its default branch does not, so read the branch, not the repo. Copy that shape rather than inventing one.
- No new scenario. The behaviour is reachable by checks on the rendered invocation plus one real cluster build.

## Assumptions

These must hold before the cluster AC can pass; none is created by this spec.

- The `build` namespace and the `buildkit-push` dockerconfigjson secret exist in the target cluster, **and that credential covers the registry the arm pushes to**. Measured: the secret exists in `nukeprod`'s `build` namespace, and the pilot's `# Progress` 2026-11-08 records that this credential authenticates against Docker Hub — a mirror push from the same lane failed `401 Unauthorized` after a successful build. This repo's both arms already compose a Docker Hub reference, so the hazard does not arise.
- **Which cluster.** Sun is host-down on 2026-11-09 (goal ruling, card `81b8776ca69d52487fcaf66f777b2920`): while it is down, cluster builds run on **nukeprod**, and each evidence row records the cluster it used through the Job name and node. Sun is the target once it returns.
- The repo is public, so the Job's anonymous `git fetch` succeeds; the init container carries no clone credential.
- `crane` is installed on the host running the ACs.
- `REF` exists in the local repository **and on `origin`**: the runner derives `SOURCE_DATE_EPOCH` from the local repository, and the Job's init container fetches the ref from the remote. A commit that exists only locally fails at the clone.
- The build pod can reach the Go module proxy — required now that the build resolves dependencies in module mode rather than from a vendored tree. The local arm needs the proxy too, for the same reason.
- The prompt container can reach `raw.githubusercontent.com`, which is where the five `hack/` files are copied from at their pinned SHAs. There is no vendored fallback; if the fetch fails, the copy step cannot run.
- `kubectl` is configured for the target cluster through the alias files `hack/cluster-alias.sh` resolves.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `BUILD_TARGET` names an unknown cluster or context | The runner fails before applying anything and lists the accepted values | Re-run with a listed target; no Job exists to clean up |
| `VERSION` omitted on the cluster arm | Refusal before the Job is created, naming `VERSION` | Re-run passing `VERSION=<scratch-tag>` |
| `REF` is absent from the local repository | The runner dies resolving `SOURCE_DATE_EPOCH`, naming the ref | `git fetch`, then re-run with the full 40-character SHA |
| `REF` is a short SHA that does exist locally | Epoch resolution succeeds, then the clone init container fails: `git fetch --depth=1 origin <short>` does not resolve | Re-run with the full 40-character SHA |
| `REF` exists locally but was never pushed | The clone init container fails: the ref does not exist on `origin` | Push the branch or commit, then re-run |
| The clone's commit epoch differs from the locally derived one | The build container refuses before `buildctl` runs and names both epochs | `git fetch`, then re-run |
| A Job with the derived name already exists in `build` | The runner refuses and names the existing Job rather than overwriting it | Delete the finished Job, then re-run |
| The runner is interrupted after `apply` | The Job keeps running in the cluster while the local invocation exits; the next run then hits the existing-Job refusal | Inspect `kubectlnukeprod -n build get jobs`, read the pod's log, delete the Job once finished |
| The build pod cannot reach the module proxy | The Job fails; the runner prints the last 100 log lines and the Job name | Inspect the Job logs; the local arm is unaffected |
| The build pod is OOM-killed or throttled | The Job reports `failed`; the pod status carries `OOMKilled` or a CPU-throttling event | Raise the template's limits in the same change, or build locally for that one image |
| The registry throttles or rejects the push | The build fails at the push step with the registry's own error | Re-check the `build` namespace secret with `kubectlnukeprod -n build get secret buildkit-push -o jsonpath='{.metadata.name}'`, then re-run |
| A caller-supplied `VERSION` names an already-released tag | No guard — the push replaces that tag irreversibly | Re-push the image from the released tag, then cut a new release |
| A caller sets `DOCKER_REGISTRY` to another registry | Both arms compose their *push reference* from it and cannot be split; the cluster push then fails the credential check unless that registry accepts the `buildkit-push` secret. The Job's base-image build-arg stays `docker.io` regardless — the template hardcodes it — so the override moves the push only | Re-run without the override; confirm the real reference with `crane digest docker.io/bborbe/github-dark-factory-watcher:<tag>` |
| A local build with no network and a cold module cache | `go build -mod=mod` fails at the module download — the local arm now needs the proxy where the vendored tree used to be enough | Warm the cache, or `go mod vendor` and build with `-mod=vendor` by hand; the cluster arm is unaffected |
| The local arm regresses | `make build` fails, refuses, or tags differently | Revert the arm; the local path is the regression gate |

## Security / Abuse Cases

- The runner substitutes caller-supplied values into a manifest and into embedded shell, so every value is checked against a character whitelist and rejected for embedded newlines before rendering. Do not add a value path that bypasses that check.
- The repository URL and the push-secret name are literals in the dispatch, not configurable inputs. The whitelist constrains the *shape* of a URL, not *which* repository is cloned and built with a mounted push credential; a configurable clone source on this build would let a caller point the credential-bearing Job at another repo.
- `DOCKER_REGISTRY` is different: it is the pre-existing make variable both arms compose their tag from, and it stays overridable. A caller who sets it redirects *both* arms to the same place — the pair cannot be split — and the cluster push then fails the credential check unless the named registry happens to accept the `buildkit-push` secret. This spec does not narrow that, because narrowing it would change the local arm. It is named here so the implementer does not mistake the composition for a guarantee.
- The build Job mounts a push credential from a `build`-namespace secret and runs with `automountServiceAccountToken: false`. The credential is scoped to pushing an image; it grants nothing else.
- The build container runs as an unprivileged user with the sandbox profiles relaxed because rootless BuildKit requires it. That relaxation is deliberate and must not be copied to the clone init container.
- `VERSION` is caller-controlled and becomes an image tag and a Job name. The tag is whitelisted and the Job name is sanitized to a valid DNS name, truncated, and stripped of trailing separators.
- `hack/build-cluster-bootstrap.sh` reads a TeamVault credential and renders it into a 600 temp file for the length of one `kubectl create secret --dry-run`. It is copied for parity with the sibling repos; this spec does not run it, because the namespace and secret already exist.

## Suggested Decomposition

Prompts should be generated in this order — each row is a single prompt with a clear scope.

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | `Makefile.docker` dispatch: the `BUILD_TARGET` split, the caller-supplied `VERSION` guard, the composed-reference echo, local-only `upload`/`clean`/`buca`, and narrowing `check-version-tag` / `check-go-mod` to the local arm | 1, 2, 3, 4, 5 | 1, 3, 5, 8 | — |
| 2 | The build path itself: the five `hack/` files and the module-mode image build | 2, 6 | 2, 4 | prompt 1 (the dispatch calls the runner) |
| 3 | Docs: the README build section and the CHANGELOG bullet | 7 | 6 | — |

Rationale: prompt 1 establishes the dispatch and the guard that makes any of it reachable; prompt 2 supplies the runner the dispatch calls and the Dockerfile that works from a clone; prompt 3 is independent and touches no build logic.

**The repo-hygiene requirement (DB8 / AC7) is deliberately not a prompt.** It is already in this PR, because no generated prompt can satisfy it: the lifecycle writes each prompt's metadata into `prompts/in-progress/` when that prompt finishes, so a prompt that creates the directory still fails before it can save its own metadata. Measured twice — see the Problem section.

## Do-Nothing Option

Leaving this repo without a cluster arm keeps a watcher image tied to the Mac. Every rebuild, every release cut while the operator is away from this machine, and every future unattended pipeline needs a human at the keyboard for this one repo — while its siblings build themselves. The cost is not the one build; it is that the automation has a hole shaped exactly like the repo nobody is watching.
