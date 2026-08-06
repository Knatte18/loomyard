# Batch: no-transients-under-lyx-guard

```yaml
task: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)
batch: no-transients-under-lyx-guard
number: 5
cards: 2
verify: go test ./cmd/lyx/...
depends-on: [2, 4]
```

## Batch Scope

This batch installs the machine-enforced guard for the sweep batches 2–4 performed: a runtime test that drives every module's exported path constructors against synthetic `*lyxcwd.Location` values and asserts no `_lyx`-rooted path is a transient, while every corresponding transient path does resolve under `.lyx` at the mirrored subpath.
It also records the resulting invariant in `CONSTRAINTS.md`.

It is one batch, and a small one, because it is the join point of three parallel predecessors: the guard cannot exist before perch (batch 2), webster/builder/loom (batch 3) and the `.lyx` group re-anchoring (batch 4) have all landed, and it must exist before batch 6 deletes the exclusion machinery that is currently the only thing holding these artifacts back.

**Batch-local decision — the guard is a Tier 1 test in `cmd/lyx`, not a `lyxtest` synthetic hub.**
The discussion floated a synthetic-hub runtime test, but every constructor involved is pure `filepath.Join` arithmetic over a hand-built `*lyxcwd.Location` — the exact pattern `cmd/lyx/constructoranchoring_test.go` already uses — so no git spawn and no fixture copy is needed.
That keeps the guard untagged and fast per the Test Tier Purity Invariant, and `cmd/lyx` is the only package that may import every owning module at once.
The one thing this shape cannot reach is treadle's caller-supplied run/scratch dirs, which are not `Location`-derived;
those are covered by feeding the guard perch's own `RunsDir`/`ScratchDir` pair plus `treadleengine.PauseFlagPath`, which is exactly the composition perchcli performs.

## Cards

### Card 31: add the no-transients-under-_lyx guard

- **Context:**
  - `cmd/lyx/constructoranchoring_test.go`
  - `internal/websterengine/state.go`
  - `internal/builderengine/state.go`
  - `internal/perchengine/identity.go`
  - `internal/loomengine/config.go`
  - `internal/treadleengine/state.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/enforcement_test.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/notransients_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** create an untagged `package main` test file with a `TestNoTransientsUnderLyx` function, modelled on `constructoranchoring_test.go`'s hand-built-`Location` approach (build the fixture locally in this file rather than reaching for the other file's unexported helper, so neither file constrains the other).
  Run the whole table against two fixtures: `AnchorRel == "."` and `AnchorRel == "backend"`.
  Assemble two path sets.
  The **durable** set is every `_lyx`-rooted path a module exposes: `loomengine.PlanDir`, `loomengine.PlanOverview`, `loomengine.DiscussionDir`, `loomengine.LoomStatusFile`, `websterengine.Dir`, `websterengine.ReportsDir`, `builderengine.Dir`, `builderengine.ReportsDir`, `perchengine.RunsDir`, and the per-block run dir `filepath.Join(perchengine.RunsDir(l), "blk")`.
  Assert each is prefixed by `filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName)` and that **none** of them ends in `.lock`, has base name `pause`, or has a `prompts` path segment.
  The **transient** set is every relocated artifact: `websterengine.ScratchDir`, `websterengine.PromptsDir`, `builderengine.ScratchDir`, `perchengine.ScratchDir`, `loomengine.LoomStatusLock`, `logger.LogsDir`, `scoutengine.DaemonStateFile(l.AnchorPath(), "go")`, `scoutengine.DaemonLock(l.AnchorPath(), "go")`, `perchengine.PauseFlagPath(filepath.Join(perchengine.ScratchDir(l), "blk"))`, and `treadleengine.PauseFlagPath(filepath.Join(perchengine.ScratchDir(l), "blk"))`.
  Assert each is prefixed by `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName)`, and that **none** is prefixed by the durable root.
  Add a mirrored-subpath assertion: for each of the three module pairs (`websterengine.Dir`/`ScratchDir`, `builderengine.Dir`/`ScratchDir`, `perchengine.RunsDir`/`ScratchDir`), the two paths differ in exactly the one directory-name segment — assert by rewriting the durable path's `lyxdirs.LyxDirName` segment to `lyxdirs.DotLyxDirName` with `strings.Replace(..., 1)` and comparing for equality with the scratch path.
  Add a `scanned_non_empty`-style sanity sub-test, mirroring the guard style already used in `internal/lyxcwd/enforcement_test.go`, so a table accidentally emptied by a refactor cannot pass vacuously.
  Use `t.Errorf` with the constructor's name in the message, like `constructoranchoring_test.go`'s `assertPath`, so a failure names which constructor drifted.
  Spawn no process and copy no fixture tree.
  The file-header comment must state why the file lives in `cmd/lyx` (only package that may import every owning module), why it is untagged, and that it is the machine half of the "every never-tracked file lives under `.lyx`" invariant.
- **Commit:** `test(cmd/lyx): guard against transients resolving under _lyx`

### Card 32: record the mirrored-subpath invariant in CONSTRAINTS

- **Context:**
  - `cmd/lyx/notransients_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/websterengine/state.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add a new top-level section `## Durable-vs-Ephemeral State Invariant` recording three clauses:
  every never-tracked file lives under `.lyx`, at the mirrored subpath of the `_lyx` content it relates to, and `_lyx` holds tracked content only;
  `_lyx` and `.lyx` are directory siblings under `AnchorPath()` (the sole exception being `reedengine.HubLogsDir`, which is hub-anchored);
  and no engine derives its own `.lyx` path — each module exposes a scratch accessor beside its durable one and every consumer is handed the value, so a caller passing the wrong tree is a compile error rather than a silently-broken pause flag.
  Close with **Enforced by** `cmd/lyx/notransients_test.go` (`TestNoTransientsUnderLyx`) and `cmd/lyx/constructoranchoring_test.go`, noting that the mirrored-subpath rule for a *newly added* transient is a review obligation on top of the machine check.
  Place the section immediately after `## Lyxdirs Single-Declarer Invariant` so the three geometry clauses read in order.
  Add a cross-reference bullet to the existing `## Cwd Resolution Invariant` pointing at the new section, since its "A module's own durable-storage subdirectory … is that module's own private relative-path constant, joined onto `AnchorPath()` directly" bullet now has an ephemeral twin.
  Do not touch the Weft Git Invariant's "Cross-module exclusions" bullet here — batch 6 owns retiring it, and editing it now would leave the file describing a mechanism that still exists.
  Follow the repo's semantic-line-break markdown rule.
- **Commit:** `docs: record the durable-vs-ephemeral state invariant`

## Batch Tests

`verify: go test ./cmd/lyx/...` — the guard is a single untagged Tier 1 test in `cmd/lyx`, and running that package also re-runs `constructoranchoring_test.go`, `tierpurity_test.go` and `hermeticenv_test.go`, which are exactly the meta-tests a new test file can violate (the new file must not trip the tier-purity ban on `exec.Command`/`gitexec.RunGit`/`lyxtest.Copy*`, and must not need a `TestMain` since it spawns no git).

Covered files: `cmd/lyx/notransients_test.go` (new), plus the whole `cmd/lyx` guard suite re-run as a side effect.

The load-bearing design choice is that the guard asserts **both** directions.
Checking only "each transient resolves under `.lyx`" would pass an implementation that also left a copy under `_lyx`;
checking only "no `_lyx` path is a transient" would pass an implementation that dropped a transient somewhere else entirely.
The mirrored-subpath equality check is the third leg: it is what makes "same relative subpath" a machine fact rather than a convention, so a future module that puts its scratch tree at, say, `.lyx/webster-scratch` fails immediately.
