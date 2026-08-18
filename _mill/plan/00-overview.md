# Plan: websterengine + webstercli told-geometry, and Webster standalone entry

```yaml
task: websterengine + webstercli told-geometry, and Webster standalone entry
slug: webster-told-geometry
approved: true
started: 20260818-091010
parent: standalone-producers
root: ""
verify: go vet ./... && go vet -tags integration ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: reed pane cwd
    file: 01-reed-pane-cwd.md
    depends-on: []
    verify: go test ./internal/reedengine/... ./internal/hubgeom/... && go test -tags integration ./internal/reedengine/...
  - number: 2
    name: batcher moves to the degrading side
    file: 02-batcher-degrading.md
    depends-on: []
    verify: go test ./internal/batcher/...
  - number: 3
    name: preflight doc correction
    file: 03-preflight-doc-correction.md
    depends-on: []
    verify: go test ./internal/preflight/...
  - number: 4
    name: websterengine Geometry and RefMatcher
    file: 04-webster-geometry-and-refmatcher.md
    depends-on: []
    verify: go test ./internal/websterengine/...
  - number: 5
    name: webster path accessors take a told anchor root
    file: 05-webster-accessors-told.md
    depends-on: [4]
    verify: go test ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/... && go test -tags integration ./internal/webstercli/...
  - number: 6
    name: hubgeom.WebsterGeometry and the standalonegeom sibling
    file: 06-hubgeom-standalonegeom.md
    depends-on: [1, 4, 5]
    verify: go test ./internal/hubgeom/... ./internal/standalonegeom/...
  - number: 7
    name: websterengine told Deps and engine-owned fabric seams
    file: 07-webster-told-deps.md
    depends-on: [5, 6]
    verify: go test ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/...
  - number: 8
    name: webstercli standalone entry
    file: 08-webstercli-standalone-entry.md
    depends-on: [2, 3, 6, 7]
    verify: go test ./internal/webstercli/... ./cmd/lyx/... && go test -tags integration ./internal/webstercli/...
```

## Shared Decisions

### Decision: told strings replace resolved Locations, one direction only

- **Decision:** every module this plan touches receives its coordinates as told strings or as a geometry struct of told strings, never as a `*lyxcwd.Location`.
  `internal/hubgeom` and the new `internal/standalonegeom` import the engines and build their geometry structs;
  no engine ever imports either package back.
  `internal/websterengine` remains the sole declarer of its own `_lyx/webster` and `.lyx/webster` subpaths — only the parameter type changes, never the ownership.
- **Rationale:** the Cwd Resolution Invariant keeps `internal/lyxcwd` as the sole owner of cwd resolution while each module keeps owning its own relative subpath.
  Told parameters are what let the same accessors serve a hub anchor and a standalone state directory without either module learning that two modes exist.
- **Applies to:** all batches

### Decision: hub mode is byte-identical after every batch

- **Decision:** no batch may change a path that resolves inside a real hub worktree, a rendered prompt byte in hub mode, or a tmux pane's spawn directory in hub mode.
  Where a told value replaces a derived one, the told value is the exact expression the code evaluated before.
  Concretely: `reedengine.Geometry.PaneCwd` is the anchor path in hub mode;
  `websterengine.Geometry.WorktreeRoot` is the anchor path, not the worktree path, because every one of webster's CLI call sites passes the anchor path today;
  and the fork-audit workdir stays the anchor path in hub mode because there `WorktreeRoot` and `AnchorRoot` are the same directory.
- **Rationale:** this is a signature migration.
  Converging any of these values on its "tidier" sibling would silently change behaviour in a subpath-anchored hub, where the two diverge and no existing test is looking.
  Keeping hub mode fixed is also what makes the existing suites a usable regression net: a converted fixture that still passes with unchanged expectations is evidence, and one that needs a new expected value is a red flag.
- **Applies to:** all batches

### Decision: no zero-value fallbacks on new told fields

- **Decision:** a new told field is never allowed to mean "fall back to the old field when empty".
  An unset `reedengine.Geometry.PaneCwd` must not silently mean `AnchorPath`, and a nil `websterengine.RunDeps.OpenBisector` means "this mode has no fabric", never "construct the production default".
  The one deliberate nil-as-mode signal in this plan — the nil fabric opener — is documented as such at its declaration and is handled by an explicit branch, not by a defaulting fallback.
- **Rationale:** a fallback makes a forgotten field indistinguishable from a correct one, which is precisely the failure the geometry split exists to prevent.
  It is also why every `reedengine.Geometry` literal in the reed test suites gets an explicit `PaneCwd` row rather than relying on a default that would never reach them.
- **Applies to:** reed pane cwd, websterengine told Deps and engine-owned fabric seams, webstercli standalone entry

### Decision: seams are engine-declared interfaces, supplied by the caller

- **Decision:** `internal/websterengine` declares the narrow interfaces it needs — `RefMatcher` for the fork audit's fabric-reference class, `FabricBisector` for the integration bisect — and the CLI supplies an implementation for each.
  `RefMatcher` is never nil in either mode: hub mode supplies the real fabric ref scanner, standalone supplies the pinned `NeverMatches` type declared beside the interface.
  The fabric handle is always reached through a lazy opener closure and is never opened during wiring.
- **Rationale:** injecting both is what actually removes `internal/fabricengine` from the engine, which is this task's stated goal;
  keeping either seam concrete would leave the engine hub-shaped for two call sites.
  The opener stays lazy because opening eagerly stat-checks a weft sibling that is legitimately absent in three healthy hub locations, which work today only because the open happens at a commit point rather than at pre-run.
- **Applies to:** websterengine Geometry and RefMatcher, websterengine told Deps and engine-owned fabric seams, webstercli standalone entry

### Decision: the repository builds at every batch boundary

- **Decision:** the batch order is chosen so that no batch leaves the tree uncompilable.
  The additive types land before the packages that consume them;
  the accessor signature change lands as a caller-mechanical batch of its own;
  and the hub-mode half of the CLI's adaptation ships in the same batch as the engine signature change that forces it, with standalone mode following on a tree that already compiles.
  The module-wide `verify:` above (`go vet` untagged and tagged) is what enforces this at each boundary.
- **Rationale:** a signature migration across three packages can be sequenced either as one enormous batch or as a chain of broken windows.
  Neither is acceptable for a Sonnet-sized unit, and the ordering above avoids both — every batch is independently reviewable and independently green.
- **Applies to:** all batches

### Decision: write the test that a hub fixture cannot fail

- **Decision:** wherever the correct and the broken implementation are indistinguishable under a hub-shaped fixture, the plan names the specific divergent fixture required rather than leaving coverage to judgment.
  The three cases are: a prompt render where the anchor root and the prompt worktree root differ;
  a fork audit where the workdir differs from the anchor root and the recorded write path is relative;
  and a nil-bisector integration failure driven with at least two accumulated card SHAs, since zero and one both take early returns that never reach the call that panics.
- **Rationale:** in a hub, the anchor root, the worktree root and the audit workdir are all the same directory, so a fixture built from a real hub passes under either implementation.
  These three are the task's genuine TDD candidates, and each one is where an earlier draft of the design was wrong.
- **Applies to:** websterengine told Deps and engine-owned fabric seams, webstercli standalone entry

### Decision: docs and invariants land with the code that makes them true

- **Decision:** every doc comment, package doc and `CONSTRAINTS.md` invariant this task falsifies is corrected in the same batch as the change that falsifies it, not deferred.
  `internal/preflight`'s prose is corrected ahead of the mode selection that contradicts it, and is recorded as a deliberate override of the package author's stated intent rather than as a doc catching up to code.
  `manifest/roadmap.md` is deliberately **not** moved: this task is one of two in its wave, and the roadmap move belongs to whichever task closes the wave.
- **Rationale:** CLAUDE.md's same-commit docs rule, and the concrete cost of deferring: from the batch that ships a told stencils directory and a standalone state tree onward, the current invariant wording is false, and a reader has no way to tell a stale invariant from a violated one.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/notransients_test.go`
- `internal/batcher/config.go`
- `internal/batcher/config_test.go`
- `internal/hubgeom/doc.go`
- `internal/hubgeom/hubgeom.go`
- `internal/hubgeom/hubgeom_test.go`
- `internal/hubgeom/webstergeom.go`
- `internal/hubgeom/webstergeom_test.go`
- `internal/preflight/doc.go`
- `internal/preflight/predicates.go`
- `internal/reedengine/contract_integration_test.go`
- `internal/reedengine/geometry.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/lifecycle_test.go`
- `internal/reedengine/lock_test.go`
- `internal/reedengine/mouse_boot_integration_test.go`
- `internal/standalonegeom/doc.go`
- `internal/standalonegeom/reedgeom.go`
- `internal/standalonegeom/standalonegeom_test.go`
- `internal/standalonegeom/webstergeom.go`
- `internal/webstercli/awaitbatch.go`
- `internal/webstercli/beginbatch.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/cli_integration_test.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/pause.go`
- `internal/webstercli/recordbatch.go`
- `internal/webstercli/recoverbatch.go`
- `internal/webstercli/run.go`
- `internal/webstercli/status.go`
- `internal/webstercli/sync.go`
- `internal/webstercli/sync_integration_test.go`
- `internal/webstercli/validate.go`
- `internal/webstercli/verbs_test.go`
- `internal/webstercli/wiring.go`
- `internal/webstercli/wiring_test.go`
- `internal/websterengine/audit.go`
- `internal/websterengine/audit_test.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/beginbatch_test.go`
- `internal/websterengine/doc.go`
- `internal/websterengine/geometry.go`
- `internal/websterengine/recordbatch.go`
- `internal/websterengine/recordbatch_test.go`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/recoverbatch_test.go`
- `internal/websterengine/render.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/runlevel_test.go`
- `internal/websterengine/state.go`
- `internal/websterengine/template_test.go`
- `internal/websterengine/webstergeom_test.go`
