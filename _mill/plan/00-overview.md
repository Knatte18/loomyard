# Plan: scoutengine told-geometry (optional uniformity pass)

```yaml
task: "scoutengine told-geometry (optional uniformity pass)"
slug: "scout-told-geometry"
approved: false
started: "20260818-130215"
parent: "standalone-producers"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: told-anchor-root-conversion
    file: 01-told-anchor-root-conversion.md
    depends-on: []
    verify: go build ./... && go vet -tags scout ./internal/scoutengine/... && go test ./internal/scoutengine/... ./internal/scoutcli/... ./cmd/lyx/...
  - number: 2
    name: hub-mode-evidence
    file: 02-hub-mode-evidence.md
    depends-on: [1]
    verify: go build ./... && go vet -tags integration ./internal/scoutcli/... && go test -tags integration ./internal/scoutcli/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: the told parameter is a bare `anchorRoot string`, never a `Geometry` struct

- **Decision:** `DaemonStateFile(anchorRoot string, lang string)` and `DaemonLock(anchorRoot string, lang string)`.
  No `scoutengine.Geometry` type and no new `geometry.go` file in `internal/scoutengine`.
- **Rationale:** scout is told exactly one path.
  The producers-standalone rule that engines take a per-engine `Geometry` struct exists to stop positional parameter lists reaching four or five strings; `websterengine.Dir(anchorRoot)`, `websterengine.ReportsDir(anchorRoot)`, `perchengine.RunsDir(anchorRoot)`, and `planparser.PlanDir(anchorRoot)` are all bare-string free functions today, and `cmd/lyx/constructoranchoring_test.go` already calls them as `f(l.AnchorPath())`.
  Scout's rows become the same shape as the rows directly above them, which is the uniformity this task buys.
- **Applies to:** all batches

### Decision: zero behavioural change is the sole acceptance property

- **Decision:** every resolved daemon-state and lock path must be byte-identical before and after this task, in hub mode and out-of-hub mode, at both unanchored and subpath-anchored geometries.
  Any change to a resolved daemon-state path is a regression, not an improvement.
- **Rationale:** the old out-of-hub synthesis always set `AnchorRel: "."`, so its `AnchorPath()` coincided with `WorktreePath()` byte for byte — the absolute target directory.
  Both new branches reproduce today's `DaemonStateFile`/`DaemonLock` inputs exactly, so no daemon is re-keyed in either mode.
- **Applies to:** all batches

### Decision: the misuse failure mode goes from loud to silent, and that is accepted

- **Decision:** `Options.AnchorRoot` is validated nowhere.
  An empty `AnchorRoot` no longer panics; it yields the relative path `.lyx/scout/<lang>/daemon.json` and the daemon writes its state wherever the process happens to stand.
- **Rationale:** this is the identical trade every converted sibling already made — `burlerengine`, `perchengine`, and `websterengine` all validate no `Geometry` field.
  A scout-only guard would reintroduce the asymmetry this task exists to remove.
  The zero-behavioural-change property above covers the two real call paths only and explicitly excludes this never-taken misuse path.
  Do not add a validation guard, an error return, or a panic to any converted signature.
- **Applies to:** all batches

### Decision: every `layout`/`Location` mention in scout comments is rewritten or deleted, production and test

- **Decision:** this is a closed rule, not a sample list.
  Every mention of `layout` or `*lyxcwd.Location` in `internal/scoutengine` and `internal/scoutcli` comments — production *and* test — is rewritten or deleted by this task.
  The card-level enumerations are a starting point; where a card's list and the tree disagree, the rule wins.
  Prose that survives a purely mechanical type swap but still says "layout" is an incomplete conversion.
- **Rationale:** `internal/scoutengine/doc.go` is this package's module doc (the `manifest/designs/` doc was deleted on landing per the Documentation Lifecycle), so `CLAUDE.md`'s "docs land in the same commit" rule points at it and at the affected file headers.
  A comment describing a shape that no longer exists is the failure this rule prevents.
  Card 8 is the closing grep gate for this rule.
- **Applies to:** all batches

### Decision: enforcement and invariants stay with T10

- **Decision:** do not add `internal/lyxcwd` to `internal/scoutengine/seam_enforcement_test.go`'s banned-import list, and do not edit `CONSTRAINTS.md`.
- **Rationale:** T6 and T7 made `burlerengine`, `perchengine`, and `websterengine` production code `lyxcwd`-free without adding a per-package ban to any of them.
  T10 owns per-producer enforcement and lands it uniformly once every package obeys it.
  A scout-only ban here would make scout the odd one out again.
  This commit introduces no new cross-cutting invariant, so the "record any new invariant in `CONSTRAINTS.md`, same commit" rule does not fire.
- **Applies to:** all batches

### Decision: no new CLI surface

- **Decision:** no `--anchor-root`, `--state-dir`, or similar flag is added; no command, `Short`, or `Long` changes.
  The anchor root stays derived exactly as today.
- **Rationale:** scout already runs against an arbitrary folder with zero lyx setup; this task delivers no new capability.
  Adding a flag would trip the CLI/Cobra Invariant's help-tree tests and is out of scope — if the implementation finds itself needing one, that is a signal to stop and flag it, not to land it.
- **Applies to:** all batches

### Decision: `manifest/designs/producers-standalone.md`, `docs/overview.md`, and `manifest/roadmap.md` are not edited

- **Decision:** leave all three untouched.
- **Rationale:** `producers-standalone.md` is knowingly left stale on three counts, all bounded by T10's deletion of the file — editing a doomed document contends with wave-4 siblings for text that ceases to exist in wave 5.
  `docs/overview.md`'s module table and execution-stack description are unaffected by a signature change.
  Per `CLAUDE.md`, `manifest/roadmap.md` moves only on completing a planned item, and T9 alone does not complete the wave.
- **Applies to:** all batches

### Decision: Go verify commands carry no `PYTHONPATH=` prefix

- **Decision:** every `verify:` in this plan is a native `go` invocation with no `PYTHONPATH= ` prefix.
- **Rationale:** the prefix rule applies to Python/mill projects only; this is a Go repo and uses the native test runner directly.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens)._

- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/notransients_test.go`
- `internal/scoutcli/cli.go`
- `internal/scoutcli/cli_integration_test.go`
- `internal/scoutcli/cli_test.go`
- `internal/scoutcli/testmain_test.go`
- `internal/scoutengine/daemonstate.go`
- `internal/scoutengine/doc.go`
- `internal/scoutengine/ensureserver.go`
- `internal/scoutengine/ensureserver_integration_test.go`
- `internal/scoutengine/ensureserver_test.go`
- `internal/scoutengine/refs.go`
- `internal/scoutengine/refs_integration_test.go`
- `internal/scoutengine/scoutdaemon_test.go`
- `internal/scoutengine/supervised_integration_test.go`
- `internal/scoutengine/supervised_scout_test.go`
- `internal/scoutengine/supervised_test.go`
