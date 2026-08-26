# Batch: regression-coverage

```yaml
task: "loom's status file can conflict on the landing merge"
batch: "regression-coverage"
number: 4
cards: 1
verify: go test -tags integration ./internal/loomcli/...
depends-on: [1]
```

## Batch Scope

This batch adds the one test that would have caught the bug: two tasks landing in sequence off the same parent, with the second landing's parent-side merge asserted conflict-free.
One task alone never conflicts — the divergence the bug needs requires both sides to have rewritten the status file since their merge base — so a single-landing fixture cannot reproduce it, and no existing test does.

It is its own batch because it is a new file in a tier no other batch touches, it depends only on batch 1's finished state, and it can run in parallel with the two text batches.

## Cards

### Card 17: Add the two-sequential-landings regression test

- **Context:**
  - `internal/landingshed/finalize_integration_test.go`
  - `internal/landingshed/testmain_integration_test.go`
  - `internal/landingshed/deps.go`
  - `internal/landingshed/finalize.go`
  - `internal/landingshed/config.go`
  - `internal/loomcli/testmain_test.go`
  - `internal/loomcli/landingdeps.go`
  - `internal/loomengine/config.go`
  - `internal/loomshed/seed.go`
  - `internal/state/state.go`
  - `internal/hubforge/hub.go`
  - `internal/mergeresolve/deps.go`
  - `internal/fabricengine/origin.go`
  - `internal/shedengine/status.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/landing_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create the file in package `loomcli` under the `integration` build tag, with a file-header comment stating what it pins and why it lives here: it needs both loom's own status-file path constructors and `landingshed.Finalize`, and `internal/loomcli` is the one layer that legitimately imports both.
  Add no `TestMain` — the package's existing untagged `internal/loomcli/testmain_test.go` already runs `gitkit.HermeticGitEnv()` once for the whole binary, and a second declaration in the same package would not compile.
  Write one test, `TestFinalize_TwoSequentialLandingsNeverConflictOnTheStatusFile`.
  Build the fixture with `hubforge.NewHub(t, ".")` followed by three `hubforge.AddPair` calls, for a parent pair and two task pairs, mirroring the two-pair shape `TestFinalize_ResolvesConflictAndSquashMergesIntoParent` already uses.
  For each task pair, resolve a `*lyxcwd.Location` from its warp worktree with `lyxcwd.ResolveWorktree`, then call `loomshed.Seed(loomengine.LoomStatusFile(loc), loomengine.LoomStatusLock(loc), <that task's slug>, <the parent pair's branch>)`.
  After seeding, rewrite each task's status file in place with distinct content, so the two tasks' orchestration state genuinely diverges rather than being byte-identical — read it back through `state.ReadJSONStrict[shedengine.Status]`, set a different `CurrentProducer` and a non-empty `History` per task, and write it back through `state.UpdateJSON` under the same status lock, standing in for the per-transition persists a real `Shed` run would have made.
  Divergent content is the whole point: byte-identical files on both sides of a merge cannot conflict, so a fixture that skipped this would pass even against the pre-fix code.
  Give each task pair a small, non-conflicting ordinary commit as well, so each landing has real content to carry and the merge is not a no-op.
  Drive each landing with a `landingshed.Deps` struct literal, copying the field set `TestFinalize_ResolvesConflictAndSquashMergesIntoParent` uses: `WorktreeRoot`, `TaskBranch`, `ParentBranch`, `StencilsDir`, `ScratchDir`, `OpenFabric`, `OpenParentFabric`, `Shuttle`, and a `landingshed.Config` with `Squash: true`, a `Conflict` model spec, and a small `ConflictTimeoutMin`.
  Copy that file's local helpers rather than importing them — the two test packages cannot share unexported test code — specifically its conflict-stencil seeder and its `lyxcwd.ResolveWorktree` + `fabricengine.Open` pair-opener, both renamed to avoid colliding with any identifier already declared in package `loomcli`'s test files.
  For the shuttle seam, write a strict fake whose `Run(shuttleengine.Spec) (shuttleengine.Result, error)` method fails the test outright when called, and assert `mergeresolve.Shuttle` compliance with a `var _` declaration.
  This fake is the load-bearing assertion, not a stub: a conflict-resolution session is spawned only when the merge actually conflicts, so a fake that can never succeed turns "the second landing conflicted" into a test failure with a precise message rather than a downstream symptom.
  Call `landingshed.NewFinalize` and then `Call` for the first task, asserting `shedengine.Done` and a nil error; then do the same for the second task, whose pair forked from the parent before the first task landed and whose catch-up merge is exactly where the old bug surfaced.
  Then assert the parent pair's overlay branch tracks no `loom/status.json` at any depth: run `git ls-files` in the parent pair's overlay sibling worktree, obtained through `hubforge.Hub`'s own sibling accessor, and fail if any listed path's base name is `status.json` under a `loom` directory segment.
  This is the second assertion the discussion asks for — it pins the junk-on-parent half of the fix, which the conflict-free assertion alone does not cover.
  Assert nothing about cross-machine resume in either direction: the property is removed as a claim, so there is nothing to assert about it.
  Add no `//go:build smoke` variant, spawn no tmux server, and start no real provider session — every seam that would reach one is faked here.
- **Commit:** `test(loomcli): pin two sequential landings against a status-file conflict`

## Batch Tests

`verify:` is `go test -tags integration ./internal/loomcli/...`, which is the only invocation that compiles and runs this file — package `loomcli` has no integration-tagged test today, so an untagged `go test` would silently skip it entirely.
The tag is correct rather than incidental: the test spawns real git through `hubforge` fixtures and builds real worktree pairs, which the Test Tier Purity Invariant keeps out of the untagged tier, and the Hermetic Git Test Environment Invariant is satisfied by the package's existing `TestMain`.

The new test is the entire batch, and it is the regression guard for the bug the task exists to fix.
Its two assertions map onto the two halves of the fix: the strict shuttle fake proves no conflict-resolution session was needed on either landing, and the parent-side `git ls-files` scan proves no per-task orchestration artifact was deposited on the parent branch.
The repo-wide done gate already runs `go test -tags integration ./...`, so this file is covered there too without further configuration.
