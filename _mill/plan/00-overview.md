# Plan: shuttleengine + reedengine + tokenvocab told-geometry

```yaml
task: "shuttleengine + reedengine + tokenvocab told-geometry"
slug: "shuttle-reed-told-geometry"
approved: false
started: "20260817-144614"
parent: "standalone-producers"
root: ""
verify: go vet ./... && go vet -tags smoke ./... && go vet -tags integration ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: hublogsdir-move
    file: 01-hublogsdir-move.md
    depends-on: []
    verify: go test ./internal/fabricengine/... ./internal/reedengine/... ./cmd/lyx/... && go vet -tags smoke ./internal/reedcli/...
  - number: 2
    name: tokenvocab-plain-fields
    file: 02-tokenvocab-plain-fields.md
    depends-on: []
    verify: go test ./internal/tokenvocab/... ./internal/reedengine/...
  - number: 3
    name: reed-geometry-hubgeom
    file: 03-reed-geometry-hubgeom.md
    depends-on: [1, 2]
    verify: go test ./internal/hubgeom/... ./internal/reedengine/... ./internal/reedcli/... ./internal/shuttlecli/... ./internal/burlercli/... ./internal/perchcli/... ./internal/webstercli/... ./cmd/lyx/... && go test -tags integration ./internal/reedengine/... && go vet -tags smoke ./internal/reedcli/... ./internal/shuttlecli/... ./internal/treadleengine/... ./internal/burlerengine/...
  - number: 4
    name: shuttle-told-strings
    file: 04-shuttle-told-strings.md
    depends-on: [3]
    verify: go test ./internal/shuttleengine/... ./internal/websterengine/... ./internal/shuttlecli/... ./internal/webstercli/... ./internal/burlercli/... ./internal/perchcli/... ./internal/tokenvocab/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/... && go vet -tags smoke ./internal/shuttlecli/... ./internal/treadleengine/... ./internal/burlerengine/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: naming — `AnchorPath`/`anchorPath`, never `AnchorRoot`/`anchorRoot`

- **Decision:** the value derived from `lyxcwd.Location.AnchorPath()` is spelled `AnchorPath` as a struct field and `anchorPath` as a parameter or local, everywhere in this task.
  The value derived from `Location.WorktreePath()` is spelled `WorktreeRoot` / `worktreeRoot`.
  This overrides `manifest/designs/producers-standalone.md:291-292`, which spells the first one `anchorRoot`.
- **Rationale:** `CONSTRAINTS.md`'s Cwd Resolution Invariant reserves `root` for the git worktree/repo root and bans naming a `cwd`-equal value `root`; `cwd` is gated to equal `AnchorPath()` exactly, so `anchorRoot` is the precise inversion the rule bans.
  T4 of the same decomposition doc already spells the identical concept `anchorPath`.
- **Applies to:** all batches

### Decision: told geometry is trusted verbatim, never validated

- **Decision:** `reedengine.New` keeps its `(*Engine)`-only return and validates no field of `Geometry`.
  Populating every field with a usable absolute path (or socket-safe key) is the caller's obligation, stated in `New`'s doc comment and in `Geometry`'s own doc comment.
  The same posture applies to `shuttleengine.NewRunner`'s two string parameters.
- **Rationale:** validation would ripple an error return through nine construction sites and every test fixture, for a class of bug only a caller bypassing `hubgeom` could produce; it also contradicts the told-geometry premise that the engine is told its geometry by a caller that knows more than it does.
- **Applies to:** all batches

### Decision: no additive twins

- **Decision:** every `*lyxcwd.Location`-taking signature this task changes is replaced outright.
  No wrapper, no deprecated alias, no parallel old-and-new pair.
  Each card must leave the tree compiling — a card that changes an exported signature changes every caller of it in the same commit.
- **Rationale:** `manifest/designs/producers-standalone.md`'s "no additive twins" decision; parallelism in this decomposition comes from wave scheduling, not from keeping both shapes alive.
- **Applies to:** all batches

### Decision: hub-mode geometry is built in exactly one place

- **Decision:** every hub-mode site that needs a `reedengine.Geometry` calls `hubgeom.ReedGeometry(layout)`.
  No site builds the seven-field struct literal itself, and no site re-derives `layout.AnchorPath()` / `layout.WorktreePath()` beside a `Geometry` value that already holds both.
  The one exception is `internal/reedengine`'s own in-package tests, which cannot import `hubgeom` without closing an import cycle and therefore build `Geometry` literals directly.
- **Rationale:** a swapped anchor/worktree pair compiles cleanly and fails silently; one teller is the only structural guard against the single silent failure mode this refactor introduces.
- **Applies to:** all batches

### Decision: hub-mode behaviour must be byte-identical

- **Decision:** every derived value — socket name, session name, run-dir root, state dir, logs dir, strand name, `Strand.Worktree`, header tokens — resolves to exactly what it resolves to today, when the geometry comes from `hubgeom.ReedGeometry(layout)`.
  `reedengine.ServerName`, `reedengine.SessionName` and `socketName` in `internal/reedengine/server.go` are not changed at all; only their callers move outward.
- **Rationale:** this is a signature refactor, not a behaviour change; `cmd/lyx/constructoranchoring_test.go` and `internal/reedengine/server_test.go` are the machine guards on that claim.
- **Applies to:** all batches

### Decision: spawn-observability logging survives byte-identical in intent

- **Decision:** every `logger.Info` / `logger.Warn` call in `internal/reedengine/lifecycle.go` and `internal/shuttleengine/run.go` survives this refactor unchanged in intent — same event, same level, same key/value fields.
  Only the expression producing a path value may change (e.g. `HubLogsDir(e.layout)` becoming `e.geom.LogsDir`).
- **Rationale:** both files are named instrumented call sites under `CONSTRAINTS.md`'s Live-Substrate Spawn Observability invariant.
- **Applies to:** hublogsdir-move, reed-geometry-hubgeom, shuttle-told-strings

### Decision: tagged test files are compile-checked with `go vet`, not run

- **Decision:** `//go:build smoke` files touched by this task are verified with `go vet -tags smoke <packages>`, which type-checks test files without executing them.
  `//go:build integration` files in `internal/reedengine` are run for real (`go test -tags integration ./internal/reedengine/...`), since the discussion names them as a required verify step.
- **Rationale:** the smoke suites drive real tmux servers and hub fixtures; the only breakage this refactor can introduce in them is a call-site signature mismatch, which `go vet` catches.
  The repo-wide `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) already covers the untagged and integration tiers task-wide.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `cmd/lyx/constructoranchoring_test.go`
- `docs/overview.md`
- `internal/burlercli/cli.go`
- `internal/burlerengine/smoke_cluster_test.go`
- `internal/burlerengine/smoke_round_test.go`
- `internal/fabricengine/hubscratch_test.go`
- `internal/fabricengine/junctionnames.go`
- `internal/hubgeom/doc.go`
- `internal/hubgeom/hubgeom.go`
- `internal/hubgeom/hubgeom_test.go`
- `internal/perchcli/cli.go`
- `internal/reedcli/cli.go`
- `internal/reedcli/smoke_debuglog_test.go`
- `internal/reedengine/contract_integration_test.go`
- `internal/reedengine/doc.go`
- `internal/reedengine/geometry.go`
- `internal/reedengine/header.go`
- `internal/reedengine/header_test.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/lock.go`
- `internal/reedengine/lock_test.go`
- `internal/reedengine/mouse_boot_integration_test.go`
- `internal/reedengine/strand.go`
- `internal/shuttlecli/cli.go`
- `internal/shuttlecli/cli_test.go`
- `internal/shuttlecli/smoke_interrupt_test.go`
- `internal/shuttleengine/doc.go`
- `internal/shuttleengine/run.go`
- `internal/shuttleengine/run_inject_test.go`
- `internal/shuttleengine/run_test.go`
- `internal/shuttleengine/rundir.go`
- `internal/shuttleengine/rundir_test.go`
- `internal/shuttleengine/wait.go`
- `internal/shuttleengine/wait_test.go`
- `internal/tokenvocab/doc.go`
- `internal/tokenvocab/leaf_enforcement_test.go`
- `internal/tokenvocab/tokenvocab.go`
- `internal/tokenvocab/tokenvocab_test.go`
- `internal/treadleengine/smoke_judge_test.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/verbs_test.go`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/recoverbatch_test.go`
- `internal/websterengine/runlevel.go`
- `manifest/designs/producers-standalone.md`
