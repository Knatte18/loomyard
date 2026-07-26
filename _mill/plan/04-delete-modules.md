# Batch: D1 -- delete modules + enforcement

```yaml
task: 'fabric: cutover -- rewire consumers onto fabric, delete warp/weft'
batch: D1 -- delete modules + enforcement
number: 4
cards: 4
verify: go build ./... && go test -tags integration ./internal/fabricengine/... ./internal/lyxtest/...
depends-on: [1, 2, 3]
```

## Batch Scope

With every production importer of the old engines removed (A rewired the six consumers, B
collapsed configreg/configcli, C de-registered the CLI), delete the four old modules and the
four fabric differential tests (which import the old engines as a reference fixture and
cannot compile once they are gone), then update the enforcement test + CONSTRAINTS.md that
name the deleted packages. Card order keeps every commit green: delete the differential
tests first (removes the last test-side warp/weft importers), then delete the modules, then
fix the now-stale enforcement/constraint text. Depends on batches 1, 2, 3.

## Cards

### Card 15: confirm standalone coverage, then delete the four differential tests

- **Context:**
  - `internal/fabricengine/clone_test.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/checkout_rollback_test.go`
  - `internal/fabricengine/revert_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/corrindex_test.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/fabricengine/trailer_test.go`
  - `internal/fabricengine/syncweft_integration_test.go`
  - `internal/fabricengine/hook_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `internal/fabricengine/clone_differential_test.go`
  - `internal/fabricengine/lifecycle_differential_test.go`
  - `internal/fabricengine/reconcile_differential_test.go`
  - `internal/fabricengine/weftgit_differential_test.go`
- **Moves:** none
- **Requirements:** The four `*_differential_test.go` files assert fabric == warp/weft on a
  shared fixture; they import the old engines and cannot survive their deletion. Before
  deleting, read each differential test and confirm every assertion about **fabric's own**
  behaviour (not the warp/weft reference arm) is already exercised by a standalone test in
  the Context list: clone -> `clone_test.go`/`clone_adopt_test.go`; lifecycle (add/checkout/
  revert) -> `add_test.go`/`checkout_rollback_test.go`/`revert_test.go`/`clone_adopt_test.go`;
  reconcile -> `reconcile_stale_registration_test.go`/`corrindex_test.go`/
  `index_integration_test.go`; weftgit -> `weftgit_exclude_test.go`/`trailer_test.go`/
  `syncweft_integration_test.go`. If EVERY assertion is covered (expected -- fabric has a
  dedicated standalone suite), `git rm` the four differential files. If a genuinely unique
  fabric-behaviour assertion is found with no standalone equivalent, STOP and report it as a
  stuck (`stuck_type: logic`) rather than silently dropping coverage -- do not invent a test
  blind. Delete via `git rm` (blind script per the scripts decision), no need to open the
  files beyond the coverage read.
- **Commit:** `test(fabricengine): delete differential tests (coverage confirmed standalone)`

### Card 16: delete the four old modules

- **Context:**
  - `cmd/lyx/main.go`
  - `internal/configreg/configreg.go`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `internal/warpengine`
  - `internal/warpcli`
  - `internal/weftengine`
  - `internal/weftcli`
- **Moves:** none
- **Requirements:** Delete the four packages entirely (`git rm -r internal/warpengine
  internal/warpcli internal/weftengine internal/weftcli`), including all their `*_test.go`
  files. This is a blind script delete: no production or test file imports these packages any
  more (A/B/C removed every import; card 15 removed the differential tests). The Context
  files are read-only confirmation that `cmd/lyx/main.go` and `internal/configreg/configreg.go`
  no longer reference the old engines -- if either still imports one, that import is a defect
  from batch A/B/C and must be resolved there, not worked around here. `go build ./...` in
  `verify` is the safety net that proves no dangling importer survived.
- **Commit:** `refactor: delete warpengine/warpcli/weftengine/weftcli`

### Card 17: update lyxtest leaf enforcement test

- **Context:**
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `internal/lyxtest/leaf_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The `bannedImports` list is a string-match walk that names the four
  now-deleted feature packages (`warpengine`/`warpcli`/`weftengine`/`weftcli`). It still
  compiles, but the entries are stale. Remove the four deleted-package entries; keep
  `fabricengine`/`fabriccli` (and every other feature package) in the banned list -- the
  invariant that `internal/lyxtest` imports only stdlib + `internal/hubgeometry` is
  unchanged. Do not weaken the test.
- **Commit:** `test(lyxtest): drop deleted warp/weft packages from leaf enforcement list`

### Card 18: update CONSTRAINTS.md for the collapsed modules

- **Context:**
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the two invariants that name the deleted modules:
  - **Weft Git Invariant:** collapse the dual-ownership bullets. Where it currently says
    weft-internal git goes through "`internal/weftengine` **or** `internal/fabricengine`" and
    host<->weft topology through "`internal/warpengine` **or** `internal/fabricengine`", name
    **only** `internal/fabricengine` for both concerns. Delete the "**Parallel-build note**"
    sentence about dual ownership lasting "only until the warp/weft cutover task". In the
    "Orchestration, not agent" and "Enforced by" paragraphs, replace `weftengine.Sync`/
    `weftengine.Commit` and "inside `weftengine`/`warpengine`" with the fabric equivalents
    (`fabricengine`'s `SyncWeft`/`CommitWeft`, "inside `internal/fabricengine`").
  - **lyxtest Leaf Invariant:** in the parenthetical example list of banned feature packages,
    remove `warpengine`/`warpcli`/`weftengine`/`weftcli`; keep the `boardengine`/`boardcli`
    example and add nothing new.
  No new cross-cutting invariant is introduced by this task, so add no new section.
- **Commit:** `docs(constraints): collapse Weft Git + lyxtest invariants onto fabricengine`

## Batch Tests

`verify` runs `go build ./...` (the acceptance check that no dangling importer of the deleted
packages survives anywhere in the tree) plus the integration suites for the two modules this
batch's edits/deletes touch: `internal/fabricengine` (four differential tests removed --
confirms the remaining standalone suite still passes) and `internal/lyxtest` (the leaf
enforcement guard). CONSTRAINTS.md is prose and does not affect the build.
