# Batch: fabricengine in-package weft

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'fabricengine in-package weft'
number: 9
cards: 11
verify: go vet -tags integration ./... && go test -tags integration ./internal/fabricengine/...
depends-on: [8]
```

## Batch Scope

This batch moves nine `package fabricengine` test files to `package fabricengine_test` and migrates their thirty-eight `CopyWeft` sites onto `hubforge.NewHub`.
The move is forced, not stylistic: `internal/fabricengine` is inside `internal/fabriccli`'s dependency set, so an in-package test importing `hubforge` would close a compile cycle.

Every unexported identifier these nine files reach for gets an exported alias in `internal/fabricengine/export_test.go`, which is the existing shim precedent in this package.
That shim growth is planned work — card 53 does it first, in one card, so the eight migration cards that follow are pure per-file work and the shim's diff is reviewable on its own.

Batch-local decision: no file is renamed.
Moving a test file out of its package is a one-line change to its `package` clause;
a `git mv` on top would add churn without adding history.

Batch-local decision: the eight migration cards are ordered smallest first (one site, then three, then four, then six, then eleven), so the pattern for `CopyWeft`'s replacement is settled on `weftgit_unborn_warp_test.go` before it is applied to `snapshot_integration_test.go`'s eleven sites.

## Cards

### Card 53: Grow the export shim for the nine relocating files

- **Context:**
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/weftgit_pathspec_integration_test.go`
  - `internal/fabricengine/weftgit_unborn_warp_test.go`
  - `internal/fabricengine/diff_integration_test.go`
  - `internal/fabricengine/coalesce_integration_test.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/snapshot_integration_test.go`
  - `internal/fabricengine/pull_integration_test.go`
  - `internal/fabricengine/syncweft_integration_test.go`
- **Edits:**
  - `internal/fabricengine/export_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Read the nine files listed in `Context:`, collect every unexported `fabricengine` identifier they reference (functions, types, constants, struct fields reached through a helper), and add one exported alias per identifier to `internal/fabricengine/export_test.go`, following the file's existing convention: `var XForTest = x` for functions and values, `type XForTest = x` — a type **alias**, not a defined type — for types whose fields the test constructs, `const XForTest = x` for constants, and a small `XForTest(...)` wrapper function where the unexported seam takes unexported parameter types.
  Give every added identifier a doc comment saying which file needs it and why the exported surface does not already cover it, matching the existing entries' tone.
  Do not export anything the nine files do not actually reach — the shim is a seam, not a mirror of the package's private surface.
  This card produces a shim that nothing yet consumes, so it must compile but changes no behavior;
  cards 54 through 61 consume it, and any identifier this card missed is added by the card that discovers it, noted in that card's commit message.
- **Commit:** `test(fabricengine): grow the export shim for the relocating weft suites`

### Card 54: Relocate and migrate weftgit_unborn_warp_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/export_test.go`
- **Edits:**
  - `internal/fabricengine/weftgit_unborn_warp_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `package fabricengine` to `package fabricengine_test`, keeping the `//go:build integration` tag, and qualify every now-external reference with `fabricengine.` or its `ForTest` alias from `internal/fabricengine/export_test.go`.
  Replace the one `gitkit.CopyWeft(t)` call with `hubforge.NewHub(t, ".")`, mapping `.WeftPath` to `h.PrimeWeft()` and `.Bare` to `h.WeftBare` per the overview's table.
  The three `gitkit.MustRun(` calls stay on `gitkit` unchanged.
  This file's subject is an unborn warp branch, which the old fixture reached by handing `CopyWeft` a weft with no warp at all;
  on a real hub the warp exists, so the unborn state must be arranged explicitly rather than obtained by omission.
  Arrange it on the real hub rather than reaching back for a warp-less fixture — that reachability is the point of the migration.
  This is the smallest of the nine and sets the pattern;
  run it green before starting card 55.
- **Commit:** `test(fabricengine): relocate the unborn-warp suite onto hubforge`

### Card 55: Relocate and migrate commit_integration_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/weftgit_unborn_warp_test.go`
- **Edits:**
  - `internal/fabricengine/commit_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `package fabricengine` to `package fabricengine_test`, keeping the `//go:build integration` tag, and qualify every now-external reference per card 54's pattern.
  Replace the three `gitkit.CopyWeft(t)` calls with `hubforge.NewHub(t, ".")`, mapping `.WeftPath` to `h.PrimeWeft()` and `.Bare` to `h.WeftBare`.
  The four `gitkit.MustRun(` calls stay on `gitkit` unchanged.
- **Commit:** `test(fabricengine): relocate the weft-commit suite onto hubforge`

### Card 56: Relocate and migrate coalesce_integration_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/export_test.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/fabricengine/coalesce_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `package fabricengine` to `package fabricengine_test`, keeping the `//go:build integration` tag, and qualify every now-external reference per card 54's pattern.
  Replace the three `gitkit.CopyWeft(t)` calls with `hubforge.NewHub(t, ".")`, mapping `.WeftPath` to `h.PrimeWeft()` and `.Bare` to `h.WeftBare`.
  The nine `gitkit.MustRun(` calls stay on `gitkit` unchanged.
  This file forces a genuine non-fast-forward through `gitrepo.Push`'s rebase-retry by pushing from a second clone of the bare;
  `h.WeftBare` is a real bare reached by path and behaves identically, so that arrangement carries over unchanged — it is one of the two places the repo already proves a path-reached bare is a first-class remote.
- **Commit:** `test(fabricengine): relocate the coalesce suite onto hubforge`

### Card 57: Relocate and migrate syncweft_integration_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/export_test.go`
- **Edits:**
  - `internal/fabricengine/syncweft_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `package fabricengine` to `package fabricengine_test`, keeping the `//go:build integration` tag, and qualify every now-external reference per card 54's pattern.
  Replace the three `gitkit.CopyWeft(t)` calls with `hubforge.NewHub(t, ".")`, mapping `.WeftPath` to `h.PrimeWeft()` and `.Bare` to `h.WeftBare`.
  The five `gitkit.MustRun(` calls stay on `gitkit` unchanged.
- **Commit:** `test(fabricengine): relocate the weft-sync suite onto hubforge`

### Card 58: Relocate and migrate pull_integration_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/export_test.go`
- **Edits:**
  - `internal/fabricengine/pull_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `package fabricengine` to `package fabricengine_test`, keeping the `//go:build integration` tag, and qualify every now-external reference per card 54's pattern.
  Replace the three `gitkit.CopyWeft(t)` calls with `hubforge.NewHub(t, ".")`, mapping `.WeftPath` to `h.PrimeWeft()` and `.Bare` to `h.WeftBare`.
  The thirty-nine `gitkit.MustRun(` calls — the highest count in the repo — stay on `gitkit` unchanged;
  this file's bulk is arrangement, not fixture construction.
  It force-pushes from a second clone to build the diverged upstream `Fabric.Pull` re-anchors from, which `h.WeftBare` supports exactly as the old fixture's bare did.
- **Commit:** `test(fabricengine): relocate the pull suite onto hubforge`

### Card 59: Relocate and migrate diff_integration_test.go and index_integration_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/export_test.go`
- **Edits:**
  - `internal/fabricengine/diff_integration_test.go`
  - `internal/fabricengine/index_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `package fabricengine` to `package fabricengine_test` in both files, keeping the `//go:build integration` tags, and qualify every now-external reference per card 54's pattern.
  Replace the four `gitkit.CopyWeft(t)` calls in each file with `hubforge.NewHub(t, ".")`, mapping `.WeftPath` to `h.PrimeWeft()` and `.Bare` to `h.WeftBare`.
  The two `gitkit.MustRun(` calls in `internal/fabricengine/diff_integration_test.go` and the nine in `internal/fabricengine/index_integration_test.go` stay on `gitkit` unchanged.
  `internal/fabricengine/index_integration_test.go` asserts on index state after a checkout;
  a real hub's weft worktree has the `_lyx` junction and the anchored layout the old fixture lacked, so index assertions counting entries will move.
  Re-express them rather than relaxing them.
- **Commit:** `test(fabricengine): relocate the diff and index suites onto hubforge`

### Card 60: Relocate and migrate weftgit_pathspec_integration_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/export_test.go`
- **Edits:**
  - `internal/fabricengine/weftgit_pathspec_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `package fabricengine` to `package fabricengine_test`, keeping the `//go:build integration` tag, and qualify every now-external reference per card 54's pattern.
  Replace the six `gitkit.CopyWeft(t)` calls with `hubforge.NewHub(t, ".")`, mapping `.WeftPath` to `h.PrimeWeft()` and `.Bare` to `h.WeftBare`.
  The two `gitkit.MustRun(` calls stay on `gitkit` unchanged.
  This file's subject is the anchored weft pathspec, which is precisely where the real hub differs most from the old fixture: the old `CopyWeft` weft carried a single tracked `_lyx/config.yaml` and nothing else, whereas a real weft worktree carries the materialized config tree for every registered module under the anchored base.
  Expect every pathspec-scoping assertion here to need re-expression, and treat any that still passes unchanged as a result to double-check rather than a relief.
- **Commit:** `test(fabricengine): relocate the weft-pathspec suite onto hubforge`

### Card 61: Relocate and migrate snapshot_integration_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/export_test.go`
- **Edits:**
  - `internal/fabricengine/snapshot_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `package fabricengine` to `package fabricengine_test`, keeping the `//go:build integration` tag, and qualify every now-external reference per card 54's pattern.
  Replace the eleven `gitkit.CopyWeft(t)` calls — the largest single-file block in the batch — with `hubforge.NewHub(t, ".")`, mapping `.WeftPath` to `h.PrimeWeft()` and `.Bare` to `h.WeftBare`.
  The ten `gitkit.MustRun(` calls stay on `gitkit` unchanged.
  This file also contains one `gitkit.HermeticGitEnv(` reference;
  leave it alone — it stays on `gitkit` and is unaffected by the package flip.
  Snapshot assertions enumerate files, so this is the file where the ~155-versus-~36 file-count difference bites hardest.
  Re-express each enumeration against `fabricengine`'s own name accessors and `HubReservedNames`, never against a hardcoded list.
  This card is last in the batch deliberately: by the time it runs, the `CopyWeft` replacement pattern has been proved on twenty-seven other sites.
- **Commit:** `test(fabricengine): relocate the snapshot suite onto hubforge`

### Card 62: Confirm no in-package weft fixture survives

- **Context:**
  - `internal/fabricengine/weftgit_unborn_warp_test.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/coalesce_integration_test.go`
  - `internal/fabricengine/syncweft_integration_test.go`
  - `internal/fabricengine/pull_integration_test.go`
  - `internal/fabricengine/diff_integration_test.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/weftgit_pathspec_integration_test.go`
  - `internal/fabricengine/snapshot_integration_test.go`
  - `internal/fabricengine/export_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Verification-only gate, no diff.
  Confirm that `gitkit.CopyWeft` appears nowhere in `internal/fabricengine/`, that all nine files listed in `Context:` now declare `package fabricengine_test`, and that every alias added to `internal/fabricengine/export_test.go` in card 53 has at least one consumer — an unused alias is private surface exported for nothing and must be removed under card 53's ownership.
  Confirm that no file gained an `internal/hubforge` import while still declaring `package fabricengine`, which would be a compile cycle the build catches but which is worth eyeballing since it is this batch's whole reason for existing.
  If any check fails, fix it under the card that owns the file rather than here.
- **Commit:** none

## Batch Tests

`verify:` compile-checks the repo under `-tags integration` and runs `internal/fabricengine`'s integration suite in full.
Full-package scope is unavoidable here: nine files change package membership, which relinks the package's whole test binary, and the shim they consume is shared with every other file in the directory.

The `smoke` tag is not compile-checked because no file this batch touches is smoke-tagged.
