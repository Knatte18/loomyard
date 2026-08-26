# Batch: untrack-status-file

```yaml
task: "loom's status file can conflict on the landing merge"
batch: "untrack-status-file"
number: 1
cards: 7
verify: go test ./cmd/lyx/... ./internal/loomengine/... ./internal/loomcli/... ./internal/landingshed/... ./internal/loomshed/... && go vet -tags smoke ./internal/loomcli/...
depends-on: []
```

## Batch Scope

This batch is the whole code change: it re-roots `loomengine.LoomStatusFile` from `lyxdirs.LyxDirName` to `lyxdirs.DotLyxDirName`, deletes `loomengine.LoomStatusRel`, and removes every piece of machinery that existed only because the file was git-tracked — `lyx loom run`'s seed-commit pathspec entry, `landingshed.Deps.CommitStatus` and both producers' calls to it, and the smoke suite's status-file commits.
It is one batch because the deletions and the move are a single compile unit: removing `LoomStatusRel` breaks every remaining caller at build time, so the call sites and the constructor cannot land in separate batches without leaving the tree unbuildable in between.

**Card order is load-bearing.** Cards 1-3 move the Tier-1 assertions first (TDD — they fail against the unmoved constructor).
Cards 4-6 remove every remaining `LoomStatusRel` caller while the function still exists.
Card 7 deletes the function and moves the constructor last, so the build is green at every card boundary except the three deliberate TDD failures in cards 1-3.

Batches 2, 3, and 4 consume nothing from this batch beyond the final on-disk path and the absent `CommitStatus` field; they are pure text/doc updates and one new test file.

## Cards

### Card 1: Move the `cmd/lyx` Tier-1 anchoring assertions to `.lyx`

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `cmd/lyx/notransients_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `notransientsFixture`'s table pair, move the `{"loomengine.LoomStatusFile", loomengine.LoomStatusFile(l)}` row out of `durableSet` and into `transientSet`, placing it immediately before the existing `loomengine.LoomStatusLock` row so the status file and its own lock sit adjacent.
  Leave `durableSet`'s remaining rows, `transientSet`'s remaining rows, and the three direction checks in `TestNoTransientsUnderLyx` unchanged.
  In `TestConstructorAnchoring_Unanchored` and `TestConstructorAnchoring_SubpathAnchored`, move the `assertPath(t, "loomengine.LoomStatusFile", ...)` call out of each function's `_lyx`-durable group and into its `.lyx` group, immediately before the `loomengine.LoomStatusLock` line, changing the expected value from `filepath.Join(lyxBase, "loom", "status.json")` to `filepath.Join(dotLyxBase, "loom", "status.json")` in both.
  Add a `"loomengine.LoomStatusFile": loomengine.LoomStatusFile(l)` entry to `TestConstructorAnchoring_SubpathAnchored`'s `dotLyxConstructors` map, which the file's own comment calls the regression guard for the two-roots bug the re-anchoring exists to remove.
  The individual `assertPath` call already catches a wrong-root migration, so this is defense in depth — but the one accessor this task actually migrates is exactly the one that should not be missing from that map.
  In the same file's header comment, the paragraph beginning "As of this batch there are two groups, not three" enumerates "the `.lyx` group in full" as `loomengine.LoomStatusLock`, `loomengine.LoomDriverLog`, `loomengine.LoomBootstrapLock`, `websterengine.PromptsDir`/`ScratchDir`, and `logger.LogsDir`.
  Add `loomengine.LoomStatusFile` to that list, so the enumeration stays true the moment its assertion joins the group, and add the already-missing `loomengine.LoomRunLock` while the sentence is being touched, so "in full" is finally accurate; leave the rest of the paragraph as it stands.
  Both files stay Tier 1 — pure `filepath.Join` arithmetic over hand-built `*lyxcwd.Location` fixtures, no process spawned, no fixture tree copied — so introduce no `t.TempDir()`, no `exec.Command`, and no build tag.
  These assertions fail until card 7 lands; that is the intended TDD signal.
- **Commit:** `test(cmd/lyx): pin loom's status file under .lyx`

### Card 2: Move `loomengine`'s own status-path assertions and rewrite the file header

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/loomengine/loomstatus_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `TestLoomStatusFile`, change the `want` expression from `filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, "loom", "status.json")` to use `lyxdirs.DotLyxDirName` in place of `lyxdirs.LyxDirName`.
  In `TestLoomStatusFile_UnanchoredEqualsWorktreePath`, make the same substitution in its own `want` expression, which is built on `l.WorktreePath()` rather than `l.AnchorPath()`.
  Leave `TestLoomStatusLock`, `TestLoomRunLock`, `TestLoomStatusLock_UnanchoredEqualsWorktreePath`, and `TestLoomRunLock_UnanchoredEqualsWorktreePath` untouched — they already assert `DotLyxDirName`.
  Rewrite the five-line file-header comment: it currently says the file "pins the scratch-dir split: LoomStatusFile (durable) stays under lyxdirs.LyxDirName, while LoomStatusLock (never-tracked) resolves under lyxdirs.DotLyxDirName at the same mirrored subpath".
  After this change both accessors resolve under `DotLyxDirName`, so the split the sentence describes no longer exists; state instead that the file pins every loom status-directory accessor as a never-tracked transient under `lyxdirs.DotLyxDirName`, for both an unanchored and a subpath-anchored location, and keep the existing "pure path arithmetic, no spawning, untagged (Tier 1)" clause.
  Also rewrite `TestLoomStatusLock_UnanchoredEqualsWorktreePath`'s own doc comment, which calls `TestLoomStatusFile_UnanchoredEqualsWorktreePath` the pin "for the durable file" and itself the pin for "the never-tracked .lyx sibling" — after this change neither is durable.
- **Commit:** `test(loomengine): pin LoomStatusFile under DotLyxDirName`

### Card 3: Delete the two `LoomStatusRel` tests from `config_test.go`

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/loomstatus_test.go`
- **Edits:**
  - `internal/loomengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete `TestLoomStatusRel` and its doc comment, and delete `TestLoomStatusFile_EqualsAnchorPathJoinedWithLoomStatusRel` and its doc comment.
  Both are deleted with the function they cover: the first asserts `LoomStatusRel`'s exact value, and the second's entire subject is the `LoomStatusFile == AnchorPath() + LoomStatusRel()` identity, which has no meaning once `LoomStatusRel` is gone.
  No replacement test is written — `TestLoomStatusFile` in `internal/loomengine/loomstatus_test.go` already pins the surviving constructor's full value at a subpath-anchored location.
  After the deletions, check whether `path/filepath` and `lyxdirs` are still referenced elsewhere in the file and drop either import only if it has become unused; leave every other test in the file untouched.
- **Commit:** `test(loomengine): drop LoomStatusRel coverage ahead of its removal`

### Card 4: Reduce `lyx loom run`'s seed-commit pathspec to the origin record

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/fabricengine/origin.go`
  - `internal/loomcli/seedinput.go`
- **Edits:**
  - `internal/loomcli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the step-3 block, change `commitPaths := []string{loomengine.LoomStatusRel(), fabricengine.OriginRecordRel()}` to `commitPaths := []string{fabricengine.OriginRecordRel()}`.
  Rewrite the comment block above it.
  Its current text explains the origin record's unconditional inclusion by analogy to the status file ("for the same reason the status file's path always is ... exactly as the status file already does") and asserts that the commit must precede the driver spawn because "the phase machine's very first precondition row scans the fabric including untracked files, and neither file is on the never-tracked exclude list".
  The analogy has no second term after this change, and the second claim stops being true for the status file, which now lives under the structurally never-committed scratch tree and so cannot dirty that scan at all.
  State instead: the origin record is committed unconditionally on every invocation, not gated on this invocation's own `writeOrigin`, so that a prior invocation that wrote the record (step 1) but crashed before committing it self-heals on the next `loom run` — `resolveParentBranch` otherwise finds the record present with a matching value and reports `write == false` while it is still uncommitted; and this commit must still precede the driver spawn, because the origin record alone is a tracked path the first precondition row's cleanliness scan would see uncommitted.
  Keep the existing "costs nothing on the ordinary path, since committing an already-clean, already-tracked path is a no-op (StageAndCommit reports committed == false)" sentence.
  Two other places in this same file assert the claim this change falsifies, and both are rewritten in this card.
  The file-header comment says `run` "resolves the recorded parent branch, seeds the status file when absent, commits that seed into the fabric" — keep the seed clause and change the commit clause to name the recorded parent-branch provenance record as what is committed.
  `runCmd`'s `Long` help string's step 1 says "resolve the recorded parent branch, seed the status file when it is absent, and commit that seed into the fabric before anything else touches it" — make the same substitution there, keeping the four-steps-in-order shape and the before-anything-else ordering clause.
  The `Long` text is user-facing Cobra help, and the CLI/Cobra Invariant makes its accuracy after a behavior change part of this card, not a follow-up.
  Verify after the edit whether the `loomengine` import is still used elsewhere in the file (it is, for `LoomBootstrapLock` in step 4) and leave the import block alone unless a symbol genuinely becomes unused.
  Steps 1, 2, and 4 are unchanged.
- **Commit:** `fix(loomcli): drop the status file from the seed commit pathspec`

### Card 5: Remove the `CommitStatus` seam

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/fabricengine/commitweftpaths.go`
  - `internal/fabricengine/mergeguards.go`
  - `internal/landingshed/doc.go`
  - `manifest/designs/loom.md`
- **Edits:**
  - `internal/landingshed/deps.go`
  - `internal/landingshed/publish.go`
  - `internal/landingshed/finalize.go`
  - `internal/loomcli/landingdeps.go`
  - `internal/loomcli/landingdeps_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/landingshed/commitstatus_test.go`
- **Moves:** none
- **Requirements:**
  Delete the `CommitStatus func() error` field from `landingshed.Deps`, together with its four-paragraph doc comment.
  Delete `Publish.Call`'s step-3b block in `internal/landingshed/publish.go` — the `if p.deps.CommitStatus != nil { ... }` guard, its inner error wrap, and the comment above it — leaving step 3's push-skipped gate immediately followed by step 4's `p.resolver.Resolve` call.
  Delete `Finalize.Call`'s step-1b block in `internal/landingshed/finalize.go` — the `if fz.deps.CommitStatus != nil { ... }` guard, its inner error wrap, and the three-paragraph comment above it — leaving `entryErr`'s guard immediately followed by step 2's `fz.mergeInStep` call.
  Do not renumber the surviving step comments in either producer; the step labels are cited from `manifest/designs/loom.md` and renumbering them would break those citations.
  Delete the whole of `internal/landingshed/commitstatus_test.go`.
  It is the only user of `orderRecordingResolver` and `commitStatusRecorder`, so both helpers go with the file; confirm by grep before deleting that no sibling test in the package references either identifier.
  In `internal/loomcli/landingdeps.go`, delete the `CommitStatus:` field assignment and the two-paragraph comment above it, then drop the now-unused `fmt` import if `fmt` is no longer referenced anywhere in the file.
  Keep `ScratchDir: loomengine.LoomScratchDir(l)` and both `fabricengine`-backed opener closures, so neither the `loomengine` nor the `fabricengine` import is removed.
  In `internal/loomcli/landingdeps_test.go`, update the doc comment on `TestLandingDeps_EveryFieldPopulated`, which says a "fifteenth field added later is caught automatically" — `landingshed.Deps` has fourteen fields after this deletion, so name the new count.
  The reflection walk itself needs no change.
  No replacement test is added for the seam's absence — a deleted field needs no guard.
  The seam is removed rather than kept and passed nil: its sole purpose was satisfying `fabricengine`'s merge guard, whose `pairDirtyReason` scans tracked scope only, so an untracked status file cannot trip it; and `internal/landingshed/doc.go` already states this package's house rule against a permanently-nil implementation.
- **Commit:** `refactor(landingshed): remove the CommitStatus seam`

### Card 6: Drop the smoke suite's status-file commits

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/loomshed/seed.go`
  - `internal/fabricengine/commitweftpaths.go`
- **Edits:**
  - `internal/loomcli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rename `seedAndCommitStatus` to `seedStatus` and delete its `fabricengine.CommitWeftPaths` call and the `rec := fabricengine.NewMutations("")` line feeding it, leaving only the `loomshed.Seed` call and its error check.
  Update both call sites, and rewrite the helper's doc comment: it currently claims "the same seed-then-commit shape 'lyx loom run' itself performs at its own steps 2-3", which is no longer a commit at all.
  In `poisonStatusFile`, delete the trailing `fabricengine.CommitWeftPaths` call and its `rec := fabricengine.NewMutations("")` line, leaving the read-modify-write of the status file in place; the file is poisoned untracked now.
  Rename `TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit` to `TestSmokeBootstrap_CleanlinessAfterSeed` and re-express it: keep the `fabricengine.Clean(loc)` assertion and the closing `preflight.Check` assertion unchanged, change the commit-count assertion from `afterCount != beforeCount+1` to `afterCount != beforeCount` with a message saying the seed must produce no commit at all, and delete the `wantFiles`/`weftHeadChangedFiles` assertion outright.
  The ordering hazard this case guarded is gone by construction rather than by a commit landing first: the seed now writes solely under the scratch tree, which the cleanliness scan excludes structurally.
  Do not substitute `fabricengine.OriginRecordRel()` for the removed pathspec — `fabricengine.Topology.Add` already commits that path when the pair is created, so a second commit of it is a no-op and would produce no commit for either assertion to observe.
  Rewrite this test's own doc comment, which currently opens by asserting the pair is clean "immediately after the seed commit, and the weft carries exactly one new commit touching only the status file", and whose second paragraph explains why the case drives "just the seed-then-commit mechanism directly (seedAndCommitStatus, the same Seed+CommitWeftPaths pair 'lyx loom run' itself performs at its own steps 2-3)" rather than a full bootstrap.
  Neither claim survives: there is no seed commit and no `CommitWeftPaths` call left in the helper.
  State instead that the case asserts the pair is clean immediately after the seed with no new commit at all, because the seed now writes solely under the never-committed scratch tree, and keep the second paragraph's surviving reason for driving the seed directly rather than a full bootstrap — a live driver's own persists rewrite the status file on every phase transition, so a post-driver check would observe a state that has nothing to do with what this case pins.
  In the file-header comment, the paragraph naming "the regression home for the two bugs this task's own design rounds found" cites this case as "the cleanliness-ordering blocker (loom's own seed dirtying the weft and failing loom's own first precondition row)".
  Update that clause to name the case's new subject — that the seed cannot dirty the pair at all — and its new name, leaving the double-spawn-window guard beside it untouched.
  After the edits, delete the `weftHeadChangedFiles` helper if it has no remaining caller, and drop the `slices` import if it has become unused; `weftCommitCount` keeps one other caller, with two call sites, and stays.
  This file carries the `smoke` build tag, so `go test ./...` never compiles it — the batch verify runs `go vet -tags smoke ./internal/loomcli/...` for exactly that reason.
- **Commit:** `test(loomcli): stop committing the status file in the smoke suite`

### Card 7: Move the constructor and delete `LoomStatusRel`

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/lyxcwd/anchor.go`
- **Edits:**
  - `internal/loomengine/config.go`
  - `internal/fabricengine/commitweftpaths.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete `LoomStatusRel` and its doc comment from `internal/loomengine/config.go`.
  Rewrite `LoomStatusFile` to return `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, loomStatusFileName)`, matching the shape `LoomStatusLock` already uses, and keep `loomDirName` and `loomStatusFileName` as the file's own declared constants — the Lyxdirs Single-Declarer Invariant forbids a hand-built `.lyx` literal, and the Cwd Resolution Invariant keeps both segments here rather than in `lyxcwd`.
  Note that `LoomStatusLock` spells its own filename as the inline literal `"status.json.lock"` rather than deriving it from `loomStatusFileName`; leave that as it is.
  Rewrite `LoomStatusFile`'s doc comment: keep the AnchorPath-anchoring sentence and the product-collision rationale for the `loom` subdirectory, and replace the durable framing with a statement that the file is a never-tracked transient under `lyxdirs.DotLyxDirName` per the Durable-vs-Ephemeral State Invariant, sitting beside the lock that guards it.
  Rewrite the four other doc comments in the file that describe `LoomStatusFile` as the durable counterpart the scratch-side accessors mirror by analogy — `loomDirName`'s own comment, which says it joins onto `lyxdirs.LyxDirName` or `lyxdirs.DotLyxDirName` when after this change nothing joins it onto the former; `LoomStatusLock`'s, which contrasts its `DotLyxDirName` rooting against `LoomStatusFile`'s `LyxDirName`; `LoomDriverLog`'s and `LoomBootstrapLock`'s, which each say the accessor lives "under the ephemeral tree at the mirrored subpath of the durable status file per the Durable-vs-Ephemeral State Invariant"; and `LoomScratchDir`'s, which calls itself the mirrored-subpath counterpart `loomengine` exposes "beside its durable LoomStatusFile".
  Leave `LoomRunLock`'s comment alone: it names `LoomStatusFile` only in an anchoring analogy ("AnchorPath-anchored like LoomStatusFile and LoomStatusLock"), which stays true after the move, and it makes no durability claim.
  Leave `DiscussionDirRel`, `DiscussionDir`, `DiscussionDecisionRecord`, `DiscussionSupportLog`, and everything below `LoomReviewsDir` untouched — `DiscussionDir` is `loomengine`'s remaining durable `_lyx` path and must keep joining onto `lyxdirs.LyxDirName`.
  In `internal/fabricengine/commitweftpaths.go`, fix `CommitAnchoredPaths`'s doc comment, which says `relPaths` are "the same shape OriginRecordRel and LoomStatusRel already return" — name `OriginRecordRel` alone.
  This card is where the build breaks if cards 4-6 missed a caller: `go build ./...` must be clean before the batch verify runs.
- **Commit:** `fix(loomengine): move the status file to .lyx and delete LoomStatusRel`

## Batch Tests

`verify:` runs the four packages that own the moved constructor, its Tier-1 guards, and the removed seam — `cmd/lyx` (the two invariant guards `notransients_test.go` and `constructoranchoring_test.go`), `internal/loomengine` (`loomstatus_test.go` and `config_test.go`), `internal/loomcli` (`landingdeps_test.go` and `wiring_test.go`, which resolves `StatusPath` through `LoomStatusFile`), `internal/landingshed` (both producers' hermetic tests, minus the deleted `commitstatus_test.go`), and `internal/loomshed` (`seed_test.go`, whose fixtures follow the constructor).
The trailing `go vet -tags smoke ./internal/loomcli/...` is not redundant: `internal/loomcli/smoke_test.go` carries the `smoke` build tag, so an untagged `go test` never compiles it, and card 6's edits would otherwise go unchecked until an operator ran the smoke suite by hand.
`go vet` type-checks the tagged file without needing a tmux server.

Cards 1-3 are the TDD half and fail until card 7 lands; that is expected and is not a batch failure until the batch's own final verify.
The overview's module-wide `go vet ./...` catches any package outside these five that still names `LoomStatusRel`.
