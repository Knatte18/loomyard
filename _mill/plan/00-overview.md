# Plan: dev/test lyx.exe separated from production deploy

```yaml
task: dev/test lyx.exe separated from production deploy
slug: dev-test-binary
approved: false
started: 20260725-074748
parent: main
root: ""
verify: go build ./tools/...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: devbin-and-deploy
    file: 01-devbin-and-deploy.md
    depends-on: []
    verify: go test ./tools/internal/devbin/ ./tools/deploy/
  - number: 2
    name: sandbox-resolve-core
    file: 02-sandbox-resolve-core.md
    depends-on: [1]
    verify: go test ./tools/sandbox/
  - number: 3
    name: sandbox-wire-and-guard
    file: 03-sandbox-wire-and-guard.md
    depends-on: [2]
    verify: go test ./tools/sandbox/ ./cmd/lyx/
  - number: 4
    name: launchers-and-lifecycle
    file: 04-launchers-and-lifecycle.md
    depends-on: [3]
    verify: null
  - number: 5
    name: crucible-sweep
    file: 05-crucible-sweep.md
    depends-on: [4]
    verify: null
  - number: 6
    name: suite-docs-sweep
    file: 06-suite-docs-sweep.md
    depends-on: [4]
    verify: null
```

## Shared Decisions

### Decision: derived-dev-path-single-source

- **Decision:** The dev deploy target is the file `lyx`(`.exe`) in `<repoRoot>/.dev-bin/`,
  where `repoRoot` is **derived** (never hardcoded) via `runtime.Caller`. The derivation and
  the `.dev-bin` path convention live in exactly one place: the new `tools/internal/devbin`
  package. Both `tools/deploy` and `tools/sandbox` import it; neither keeps its own
  `runtime.Caller` derivation or hardcodes `.dev-bin`.
- **Rationale:** Operator constraint — never hardcode machine paths. A repo-relative derived
  path is per-worktree, cross-platform, zero-config. A single derivation prevents deploy and
  sandbox from ever disagreeing on where `.dev-bin` is.
- **Applies to:** all batches.

### Decision: same-name-explicit-resolution

- **Decision:** The dev binary keeps the name `lyx` (not `lyx-dev`). Disambiguation is by
  explicit directory resolution (`resolveLyx` in `tools/sandbox`) plus the SHA256 + `Source:`
  fingerprint marker — never by name. `resolveLyx` returns `(path, source)` with `source ∈
  {dev, prod}`: the derived `.dev-bin/lyx` when it exists on disk, else the existing
  `lookPath("lyx")` PATH fallback (backward compatible — prod-only flows unchanged).
- **Rationale:** The 7 black-box SUITE.md docs contain hundreds of literal `lyx <subcommand>`
  lines whose contract is "exactly what a real user types"; renaming would break that and
  merely relocate the silent-fallback risk.
- **Applies to:** sandbox-resolve-core, sandbox-wire-and-guard.

### Decision: agent-path-prepend-launchagent-only

- **Decision:** When `source=dev`, the derived `.dev-bin` directory is prepended to the
  **agent child process** PATH via `launchAgent` **only** (the agent types bare `lyx`).
  `muxDown` is NOT env-threaded — it already execs the resolved dev binary by absolute path,
  and `lyx mux` re-invokes itself via `os.Executable()`, so a PATH prepend would be a no-op.
  The dev directory is never placed on the operator's own PATH.
- **Rationale:** Bare `lyx` in an operator shell must stay prod (safe default). See discussion
  `agent-path-prepend-child-only`.
- **Applies to:** sandbox-wire-and-guard.

### Decision: Go-native tests, Tier-1 pure

- **Decision:** All new/updated tests are Go tests exercised via `go test` (no `PYTHONPATH=`
  prefix — this is Go tooling, not the Python mill harness). Tests stay Tier-1 pure: no real
  `lyx`/`claude`/network/`go build` spawns; use the existing package-var seams (`lookPath`,
  `launchAgent`, `muxDown`, `cloneRun`) and temp dirs.
- **Rationale:** CONSTRAINTS Test Tier Purity / Hermetic Git Test Environment invariants.
- **Applies to:** all Go batches.

### Decision: no new hardcoding; prod untouched

- **Decision:** This task introduces zero hardcoded machine paths. It does not change the
  production deploy path/behaviour, does not rename the binary, adds no env var, and does not
  touch the existing prod-launcher hardcoding (`deploy.cmd` `C:\Code\tools\bin`, sandbox
  `-parent C:\Code`).
- **Rationale:** Q10 — de-hardcoding prod launchers is a separate task with larger blast radius.
- **Applies to:** all batches.

## All Files Touched

- `.gitignore`
- `CONSTRAINTS.md`
- `cmd/lyx/hermeticenv_test.go`
- `cmd/lyx/tierpurity_test.go`
- `crucible/README.md`
- `crucible/board-review-prompt.md`
- `crucible/builder-review-prompt.md`
- `crucible/orchestrator-prompt.md`
- `crucible/review-prompt-template.md`
- `crucible/webster-review-prompt.md`
- `deploy-dev`
- `deploy-dev.cmd`
- `docs/overview.md`
- `docs/sandbox-howto.md`
- `docs/sandbox-hub.md`
- `manifest/roadmap.md`
- `tools/deploy/main.go`
- `tools/deploy/main_test.go`
- `tools/internal/devbin/devbin.go`
- `tools/internal/devbin/devbin_test.go`
- `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
- `tools/sandbox/SANDBOX-BURLER-SUITE.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
- `tools/sandbox/SANDBOX-MUX-SUITE.md`
- `tools/sandbox/SANDBOX-PERCH-SUITE.md`
- `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- `tools/sandbox/main.go`
- `tools/sandbox/main_test.go`
- `tools/sandbox/pathresolve_guard_test.go`
- `tools/sandbox/report.go`
- `tools/sandbox/report_test.go`
- `tools/sandbox/resolve.go`
- `tools/sandbox/resolve_test.go`
- `tools/sandbox/suite.go`
- `tools/sandbox/suite_test.go`
