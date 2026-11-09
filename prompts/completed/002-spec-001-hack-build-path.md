---
status: completed
spec: [001-cluster-build-arm]
summary: Added the five pinned, byte-identical hack/ runner files with correct modes and switched the shared Dockerfile from -mod=vendor to -mod=mod, plus an Unreleased changelog bullet.
execution_id: 11b29ae5-c10f-55a1-aa9e-e5e68cdcfec0-exec-002-spec-001-hack-build-path
dark-factory-version: dev
created: "2026-11-09T11:41:00Z"
queued: "2026-11-09T11:45:19Z"
started: "2026-11-09T11:45:21Z"
completed: "2026-11-09T11:55:11Z"
branch: feat/cluster-build-arm
---

<summary>
- The repository gains the five build-runner files the cluster arm invokes, copied byte-for-byte from two pinned upstream commits.
- Each copied file's SHA-256 is asserted against its pinned digest, so an empty, truncated or hand-edited file cannot pass.
- The copied shell scripts are executable and parse under `bash -n`; the YAML and template stay non-executable, matching upstream.
- The image build resolves Go dependencies from the module proxy instead of a vendored tree, so a Kubernetes Job that clones a fresh checkout can build.
- The local build command, its flags and its tag are untouched.
- Nothing in the copied runner is edited: it is a pinned copy, and a later prompt records its provenance in the README.
- No new build knobs, no new configuration fields and no changes to the dispatch are introduced.
</summary>

<objective>
Add the five `hack/` runner files the cluster arm invokes, each copied byte-identical from its pinned upstream commit, and switch the shared `Dockerfile` from `-mod=vendor` to `-mod=mod` so an image build works from a fresh clone that has no `vendor/` directory.
</objective>

<context>
Read these files before making changes:
- `/workspace/Makefile.docker` — already carries the `BUILD_TARGET` dispatch and already invokes `hack/build-cluster.sh` with `REPO_URL`, `REF`, `TAG`, `IMAGE` and `PUSH_SECRET`. Do NOT modify this file.
- `/workspace/Dockerfile` — the single line you change.
- `/workspace/.gitignore` — confirms `hack/` is not ignored (it is not) and that `/vendor/` stays ignored.
- `/workspace/specs/in-progress/001-cluster-build-arm.md` — Desired Behaviors 2 and 6, Constraints, Failure Modes, Security.
- `/workspace/docs/dod.md` — definition of done.

Reference docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-dockerfile-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-makefile-commands.md`

This is prompt 2 of the spec. Prompt 1 (the `Makefile.docker` dispatch) is already merged at commit `614be7c`, so `Makefile.docker` already contains the dispatch and the runner invocation. If `grep -cE '^[[:space:]]*hack/build-cluster\.sh' Makefile.docker` returns 0, STOP and report that prompt 1 is not merged; do not re-add the dispatch here.
</context>

<requirements>
1. Create the `hack/` directory at the repo root (`mkdir -p /workspace/hack`). It does not exist today.

2. Copy exactly these five files from the pinned raw URLs. Use `curl -fsSL <url> -o <path>`; if `curl` is unavailable, use `wget -qO <path> <url>`. Do not run a git clone, do not add a vendored fallback, and do not create an empty placeholder file if a fetch fails — a fetch failure means STOP and report, because the spec has no fallback copy source.

   | Destination | Source URL | Expected SHA-256 |
   |---|---|---|
   | `/workspace/hack/build-cluster.sh` | `https://raw.githubusercontent.com/bborbe/agent-task-executor/91a7e64e160f30be8af0aea64635e1e11bc86731/hack/build-cluster.sh` | `976d4fb2be48fd25db09aa5b72a994ac7ae92269e1f75d1ade60eceef1c9b6b5` |
   | `/workspace/hack/cluster-alias.sh` | `https://raw.githubusercontent.com/bborbe/agent-task-executor/91a7e64e160f30be8af0aea64635e1e11bc86731/hack/cluster-alias.sh` | `292922f77702353bbf9166a3c2fd0d406d0c94246a90b93852c3cf3bea118448` |
   | `/workspace/hack/build-job.yaml` | `https://raw.githubusercontent.com/bborbe/agent-task-executor/91a7e64e160f30be8af0aea64635e1e11bc86731/hack/build-job.yaml` | `e2a7d9d28dc01154ae184d681ff5798c5d256597f500adf1017eaa15089f81a9` |
   | `/workspace/hack/build-cluster-bootstrap.sh` | `https://raw.githubusercontent.com/bborbe/go-skeleton/eb050aa20bb6194d8af7e47baf8880df789bd1ca/hack/build-cluster-bootstrap.sh` | `f2d74f6001005b103f759ad1e30dfc661625af67f4303bc898364b8ed0a1a957` |
   | `/workspace/hack/dockerconfig.json.tpl` | `https://raw.githubusercontent.com/bborbe/go-skeleton/eb050aa20bb6194d8af7e47baf8880df789bd1ca/hack/dockerconfig.json.tpl` | `bcf89cfc8cba8b36a2f0b33fdc28a1096bad349dc5207e5e23d417939433bc7a` |

   These digests were measured from the pinned URLs on 2026-11-09 and must match the bytes you write. Do not "fix" a mismatch by editing the file — re-fetch; if it still mismatches, STOP and report.

3. Set file modes to match the upstream tree:
   - executable (`chmod +x`): `hack/build-cluster.sh`, `hack/cluster-alias.sh`, `hack/build-cluster-bootstrap.sh`
   - non-executable (`chmod 644`): `hack/build-job.yaml`, `hack/dockerconfig.json.tpl`
   `hack/build-cluster.sh` is invoked directly by the `Makefile.docker` recipe, so it must be executable.

4. Do NOT edit any of the five copied files. They are pinned, byte-identical copies. The runner mentions `BUILD_TARGET` in comments by design; leave that text alone. Do not run `hack/build-cluster-bootstrap.sh` — it provisions the `build` namespace and the `buildkit-push` secret, which already exist; it is copied for parity only.

5. Change exactly one line in `/workspace/Dockerfile`, the build `RUN` line. Old:
   ```
   RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -mod=vendor -ldflags "-s" -a -installsuffix cgo -o /main
   ```
   New:
   ```
   RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -mod=mod -ldflags "-s" -a -installsuffix cgo -o /main
   ```
   Change nothing else in the Dockerfile. In particular, keep `ARG DOCKER_REGISTRY=docker.io` and every other `FROM`/`ARG`/`COPY`/`ENV` line as they are. The `go-dockerfile-guide.md` reference implementation uses `-mod=vendor`; this repo deliberately deviates because the cluster arm builds a cloned ref that has no `vendor/` directory, and `-mod=mod` tells Go to ignore a vendored tree and resolve from the module proxy.

6. Do not add a `.dockerignore`, do not vendor anything into the image, and do not add `hack/` to `.gitignore` — the runner must be committed.

7. Do not modify `Makefile`, `Makefile.docker`, `Makefile.precommit`, `Makefile.variables`, `.gitignore`, `.dark-factory.yaml` or `.maintainer.yaml`. The dispatch and the local-only refusals are already in place.
</requirements>

<constraints>
- Do not run `git`: the container has `.git` masked. Use filesystem checks only.
- Pass `ROOTDIR=/workspace` on every `make` invocation; without it `include $(ROOTDIR)/tools.env` resolves to `/tools.env` and every target dies at parse time.
- The five `hack/` files are copied unmodified and pinned by commit; byte identity is the requirement.
- `Dockerfile` changes `-mod=vendor` to `-mod=mod` only. Do not keep vendor mode and do not vendor inside the Job.
- Do not change the Job template's `DOCKER_REGISTRY=docker.io` build argument or the Dockerfile's `DOCKER_REGISTRY` default.
- The local arm's command and tag must not change; this prompt does not touch the local recipe.
- Existing tests must still pass.
- Do not commit — dark-factory handles git.
</constraints>

<verification>
Run from `/workspace` (never use git):

```bash
cd /workspace

# Exactly the five expected files, no extras.
test "$(ls -1 hack/ | wc -l)" -eq 5
for f in build-cluster.sh cluster-alias.sh build-job.yaml build-cluster-bootstrap.sh dockerconfig.json.tpl; do
  test -f "hack/$f" || { echo "missing hack/$f"; exit 1; }
done

# Pinned digests (sha256sum; on a host without it use `shasum -a 256`).
check_digest() {
  file="$1"; expected="$2"
  got="$(sha256sum "$file" | awk '{print $1}')"
  [ "$got" = "$expected" ] || { echo "digest mismatch: $file got $got want $expected"; exit 1; }
}
check_digest hack/build-cluster.sh            976d4fb2be48fd25db09aa5b72a994ac7ae92269e1f75d1ade60eceef1c9b6b5
check_digest hack/cluster-alias.sh            292922f77702353bbf9166a3c2fd0d406d0c94246a90b93852c3cf3bea118448
check_digest hack/build-job.yaml              e2a7d9d28dc01154ae184d681ff5798c5d256597f500adf1017eaa15089f81a9
check_digest hack/build-cluster-bootstrap.sh  f2d74f6001005b103f759ad1e30dfc661625af67f4303bc898364b8ed0a1a957
check_digest hack/dockerconfig.json.tpl       bcf89cfc8cba8b36a2f0b33fdc28a1096bad349dc5207e5e23d417939433bc7a

# Modes and parse.
test -x hack/build-cluster.sh
test -x hack/cluster-alias.sh
test -x hack/build-cluster-bootstrap.sh
bash -n hack/build-cluster.sh
bash -n hack/cluster-alias.sh
bash -n hack/build-cluster-bootstrap.sh

# Module-mode image build, no vendor mode left.
test "$(grep -c -- '-mod=mod' Dockerfile)" -ge 1
test "$(grep -c -- '-mod=vendor' Dockerfile)" -eq 0

# The dispatch still invokes the runner and still composes the reference.
test "$(grep -cE '^[[:space:]]*hack/build-cluster\.sh' Makefile.docker)" -ge 1
ROOTDIR=/workspace make -n build BUILD_TARGET=nukeprod REF=0123456789abcdef0123456789abcdef01234567 VERSION=probe | grep -q 'hack/build-cluster.sh'
ROOTDIR=/workspace make -n build BUILD_TARGET=nukeprod REF=0123456789abcdef0123456789abcdef01234567 VERSION=probe | grep -q 'docker.io/bborbe/github-dark-factory-watcher:probe'

# The local arm still renders docker build and no cluster runner.
ROOTDIR=/workspace make -n build | grep -q 'docker build'
if ROOTDIR=/workspace make -n build | grep -q 'hack/build-cluster.sh'; then echo 'local arm leaked cluster runner'; exit 1; fi

# Definition of done.
ROOTDIR=/workspace make precommit
```
</verification>
