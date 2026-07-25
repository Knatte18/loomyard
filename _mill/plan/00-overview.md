# Plan: webster: rewrite for flat card list

```yaml
task: 'webster: rewrite for flat card list'
slug: webster-rewrite
approved: false
started: 20260725-131925
parent: main
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: planparser-core
    file: 01-planparser-core.md
    depends-on: []
    verify: go test ./internal/planparser/...
  - number: 2
    name: planparser-checks
    file: 02-planparser-checks.md
    depends-on: [1]
    verify: go test ./internal/planparser/...
  - number: 3
    name: batcher
    file: 03-batcher.md
    depends-on: [1]
    verify: go test ./internal/batcher/...
  - number: 4
    name: gitrepo-bisect-primitive
    file: 04-gitrepo-bisect-primitive.md
    depends-on: []
    verify: go test -tags integration ./internal/gitrepo/...
  - number: 5
    name: webster-mechanism-helpers
    file: 05-webster-mechanism-helpers.md
    depends-on: [4]
    verify: go test -tags integration ./internal/websterengine/...
  - number: 6
    name: webster-report-digest
    file: 06-webster-report-digest.md
    depends-on: [5]
    verify: go test -tags integration ./internal/websterengine/...
  - number: 7
    name: engine-retarget
    file: 07-engine-retarget.md
    depends-on: [2, 3, 6]
    verify: go test -tags integration ./internal/websterengine/...
  - number: 8
    name: integration-fork-bisect
    file: 08-integration-fork-bisect.md
    depends-on: [4, 7]
    verify: go test -tags integration ./internal/websterengine/...
  - number: 9
    name: webstercli-rewire
    file: 09-webstercli-rewire.md
    depends-on: [2, 3, 7, 8]
    verify: go test -tags integration ./internal/webstercli/...
  - number: 10
    name: docs-constraints
    file: 10-docs-constraints.md
    depends-on: [9]
    verify: go build ./...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: no-version-suffix-naming

- **Decision:** No Go package, type, function, const, or file introduced or renamed by this plan carries a `v2`/`v3`/version suffix. The new format is simply *the* plan format. "v3" appears only in prose references to the pinned spec `docs/reference/plan-format-v3.md`.
- **Rationale:** The old format dies with builder; a version suffix would imply a coexistence that does not exist in webster's world.
- **Applies to:** all batches.

### Decision: builder-is-frozen-copy-not-move

- **Decision:** `internal/builderengine` / `internal/buildercli` / `internal/builder*` are NOT modified, moved-from, or deleted by any card. Cutting webster's import edge is done by (a) re-pointing the two git helpers to `internal/gitrepo`, and (b) creating webster-LOCAL copies of every other borrowed `builderengine` helper. Every borrowed symbol has at least one in-tree builder caller, so moving it would break frozen builder — copies are mandatory, not a style choice.
- **Rationale:** Builder must stay frozen and functional in-tree (`lyx builder` and its help-tree tests untouched); deleting builder is a separate later task.
- **Applies to:** all batches (especially 5, 6, 7).

### Decision: git-verification-via-gitrepo

- **Decision:** All post-commit git verification goes through `internal/gitrepo` (a `*gitrepo.Repo` built with `gitrepo.New(worktreeRoot)`), never `internal/gitexec` directly, EXCEPT the one webster-local `dirty` helper (gitrepo exposes no porcelain/status method) which wraps `gitexec.RunGit`. No `fabric` dependency — `fabric` is unbuilt and out of scope.
- **Rationale:** Every verification primitive already exists on `gitrepo.Repo`; `fabric` is a future transparent wrapper over exactly these.
- **Applies to:** batches 4, 5, 8.

### Decision: hubgeometry-owns-lyx-paths

- **Decision:** No card constructs an `_lyx`, `plan`, `webster`, `reports`, or config path by joining string literals. All such paths resolve through `internal/hubgeometry` accessors (`PlanDir`, `WebsterReportsDir`, `WebsterPromptsDir`, `ConfigFile`, `LyxDirName`, `Layout.PlanDir()`), in production AND test code. `planparser.ParsePlan` takes an explicit `planDir string` argument; its caller (webstercli) supplies `hubgeometry.PlanDir(layout.Cwd)`.
- **Rationale:** Hub Geometry Invariant (`CONSTRAINTS.md`) — enforced by `internal/hubgeometry/enforcement_test.go`.
- **Applies to:** all batches.

### Decision: go-test-tiers-and-hermetic-git

- **Decision:** A new/edited `*_test.go` file that spawns git (directly or via a `lyxtest` helper) MUST carry a `//go:build integration` (or `smoke`) constraint AND its package must have a `TestMain` calling `lyxtest.HermeticGitEnv()`. Untagged (Tier-1) test files spawn nothing — they mock at the mux/engine/starter/`gitrepo` seam with the existing fakes. Pure-logic helpers (fingerprint, pause, archive, outcome, classify, planparser, batcher) are Tier-1; helpers exercising a real repo (gitwrap, gitrepo bisect primitive, integration bisect) are integration-tagged. `internal/websterengine` and `internal/gitrepo` already have a `TestMain`; `internal/planparser` and `internal/batcher` need NONE (no git).
- **Rationale:** Test Tier Purity Invariant + Hermetic Git Test Environment Invariant (`CONSTRAINTS.md`), both machine-enforced.
- **Applies to:** all batches introducing tests.

### Decision: deviation-list-is-informational

- **Decision:** The fork-return deviation list (files a fork changed outside its cards' declared file-ops union) is ALWAYS informational, never a failure condition. A fork returns `FAILED` only on a non-zero build/unit gate or a non-zero per-card `verify:` — never on deviation. Master records the fork-reported deviation list; whether Master additionally recomputes it via `gitrepo.ChangedFilesSince` is left to the implementer as an optional cross-check (also informational).
- **Rationale:** Plan-predicted file impact is frequently incomplete; treating deviation as failure makes the system brittle.
- **Applies to:** batches 6, 7, 8.

### Decision: verify-command-native-go

- **Decision:** All `verify:` commands are native `go test` / `go build` (this is a Go repo — the `PYTHONPATH= ` prefix rule does not apply). Per-batch verify is scoped to the touched package. Batches that add git-spawning (integration-tagged) tests use `go test -tags integration ./internal/<pkg>/...` so the new tests actually run; the module-wide overview `verify: go build ./...` is the cheap cross-package compile gate run at each batch boundary.
- **Applies to:** all batches.

### Decision: engine-retarget-is-atomic

- **Decision:** The `internal/websterengine` builder-decouple + flat-format rewrite lands as ONE batch (batch 7), not a file-by-file staging, because the borrowed types interlock: `BatchState.Digest`'s type, the fork-return report contract, the plan model (`planparser.Plan`/`batcher.Batch` replacing `builderengine.Plan`/`PlanBatch`), and the fork/master prompt templates all change together — any partial split leaves a non-compiling package. Batches 5 and 6 pre-stage the webster-local replacement types so batch 7 is a pure retarget of call sites (swap `builderengine.X` → `planparser`/`batcher`/webster-local), keeping batch 7 within the oversized limits.
- **Applies to:** batch 7.

## All Files Touched

_Union of every `Creates:` / `Edits:` / `Moves:` **target** path across every
card, sorted. (`Deletes:` and Move sources excluded.)_

- `CONSTRAINTS.md`
- `docs/overview.md`
- `docs/reference/builder-contract.md`
- `internal/batcher/batcher.go`
- `internal/batcher/batcher_test.go`
- `internal/batcher/doc.go`
- `internal/batcher/identity.go`
- `internal/batcher/identity_test.go`
- `internal/batcher/registry.go`
- `internal/batcher/registry_test.go`
- `internal/gitrepo/doc.go`
- `internal/gitrepo/gitrepo.go`
- `internal/gitrepo/gitrepo_test.go`
- `internal/planparser/doc.go`
- `internal/planparser/normalize.go`
- `internal/planparser/normalize_test.go`
- `internal/planparser/parse.go`
- `internal/planparser/parse_test.go`
- `internal/planparser/plan.go`
- `internal/planparser/sections.go`
- `internal/planparser/sections_test.go`
- `internal/planparser/validate.go`
- `internal/planparser/validate_test.go`
- `internal/webstercli/awaitbatch.go`
- `internal/webstercli/beginbatch.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/pause.go`
- `internal/webstercli/recordbatch.go`
- `internal/webstercli/recoverbatch.go`
- `internal/webstercli/smoke_test.go`
- `internal/webstercli/status.go`
- `internal/webstercli/validate.go`
- `internal/webstercli/verbs_test.go`
- `internal/webstercli/weft.go`
- `internal/websterengine/archive.go`
- `internal/websterengine/archive_test.go`
- `internal/websterengine/awaitbatch.go`
- `internal/websterengine/awaitbatch_test.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/beginbatch_test.go`
- `internal/websterengine/classify.go`
- `internal/websterengine/classify_test.go`
- `internal/websterengine/config.go`
- `internal/websterengine/config_test.go`
- `internal/websterengine/digest.go`
- `internal/websterengine/digest_test.go`
- `internal/websterengine/doc.go`
- `internal/websterengine/fingerprint.go`
- `internal/websterengine/fingerprint_test.go`
- `internal/websterengine/fork-template.md`
- `internal/websterengine/gitwrap.go`
- `internal/websterengine/gitwrap_test.go`
- `internal/websterengine/integration-template.md`
- `internal/websterengine/integration.go`
- `internal/websterengine/integration_test.go`
- `internal/websterengine/master-template.md`
- `internal/websterengine/outcome.go`
- `internal/websterengine/outcome_test.go`
- `internal/websterengine/pause.go`
- `internal/websterengine/pause_test.go`
- `internal/websterengine/poll.go`
- `internal/websterengine/poll_test.go`
- `internal/websterengine/recordbatch.go`
- `internal/websterengine/recordbatch_test.go`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/recoverbatch_test.go`
- `internal/websterengine/render.go`
- `internal/websterengine/report.go`
- `internal/websterengine/report_test.go`
- `internal/websterengine/roles.go`
- `internal/websterengine/roles_test.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/runlevel_test.go`
- `internal/websterengine/state.go`
- `internal/websterengine/state_test.go`
- `internal/websterengine/strand.go`
- `internal/websterengine/strand_test.go`
- `internal/websterengine/summary.go`
- `internal/websterengine/summary_test.go`
- `internal/websterengine/template_test.go`
- `manifest/roadmap.md`
