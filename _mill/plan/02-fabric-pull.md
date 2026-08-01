# Batch: fabric-pull

```yaml
task: 'fabric: warp-rebase / remote-reconcile recovery'
batch: fabric-pull
number: 2
cards: 6
verify: go test ./internal/fabricengine/ -run TestReachableAnchor && go test -tags integration ./internal/fabricengine/ -run TestPull
depends-on: [1]
```

## Batch Scope

This batch delivers the whole fabricengine half of the slice: the pure nearest-older-reachable anchor walk (the TDD candidate), the `PullResult`/`PartialPullError` result contract, and the unprefixed `Fabric.Pull` method that pulls weft then warp in one call, detects a warp history rewrite, safely re-anchors weft's correspondence when local warp is clean, aborts loudly on the double-conflict and no-surviving-anchor cases, and enumerates post-anchor weft commits touching `_pattern/`. It consumes batch 1's `gitrepo` primitives (`Fetch`, `IsAncestor`, `HasUnpushed`) and reuses existing fabricengine machinery unchanged (`PullWeft`, `commitEmptySnapshot`, `ensureWeftLockDir`/`weftWriteLockFile`, `appendWarpSHATrailer`, `loadCorrIndex`/`corrIndexPath`/`corrEntry`). The external interface batches 3-4 consume: `func (f *Fabric) Pull(opts SyncOptions) (PullResult, error)`, the `PullResult` struct, and the `*PartialPullError` typed error. All new code lives in two new files (`anchor.go`, `pull.go`) plus two test files — no existing fabricengine `.go` file is edited. Batch-local decision: `Fabric.Pull` acquires the fabric weft write lock ONLY around the reconcile anchor commit (mirroring `CommitWeft`), never around the warp fetch/reset, which are warp-only and need no weft lock.

## Cards

### Card 5: reachableAnchor pure anchor-walk function

- **Context:**
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/revert.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/anchor.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/anchor.go` (package `fabricengine`) with a pure, git-free function `func reachableAnchor(entries []corrEntry, reachable func(warpSHA string) (bool, error)) (corrEntry, bool, error)`. `entries` is the correspondence index's `corrEntry` slice (sorted ascending by `WarpSeq`, as `corrIndex.entries()` returns it). Walk it newest-to-oldest — iterate from `len(entries)-1` down to `0`, since ascending `WarpSeq` means the highest index is the topologically newest recorded entry — calling `reachable(entries[i].WarpSHA)` for each; return the FIRST entry whose predicate reports `(true, nil)` together with `true`. If the predicate returns an error, abort the walk and return `(corrEntry{}, false, err)` (do not swallow it). If no entry is reachable, return `(corrEntry{}, false, nil)`. This is the nearest-older-reachable-anchor walk the discussion names as the cleanest unit-testable piece; it takes an injected `reachable` predicate precisely so it is testable without git. Add a godoc comment explaining the newest-to-oldest direction and that the predicate is ancestry (reachability), never object-existence.
- **Commit:** `feat(fabricengine): add pure reachableAnchor anchor-walk`

### Card 6: reachableAnchor Tier-1 unit test

- **Context:**
  - `internal/fabricengine/anchor.go`
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/corrindex_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/anchor_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/anchor_test.go` (untagged, Tier-1, package `fabricengine`) with `TestReachableAnchor_*` cases driving `reachableAnchor` against hand-built `[]corrEntry` slices and a fake in-memory `reachable` predicate (a `map[string]bool` closure) — NO git spawn (required by the Test Tier Purity Invariant for an untagged file; model the git-free style on `corrindex_test.go`). Cover: (a) newest entry reachable → returns the newest entry; (b) newest few unreachable, an older one reachable → returns the nearest-older reachable entry (single-back and multi-back); (c) no entry reachable → `(_, false, nil)`; (d) empty slice → `(_, false, nil)`; (e) the predicate returning an error → the error propagates and the walk stops. Assert the returned `corrEntry`'s `WarpSHA`/`WeftSHA` are the expected ones and the direction is genuinely newest-first (an entry with a higher `WarpSeq` wins over an older reachable one).
- **Commit:** `test(fabricengine): unit-test reachableAnchor walk`

### Card 7: PullResult, PartialPullError, and sentinel errors

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/revert.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/fabric.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/pull.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/pull.go` (package `fabricengine`) and define the result/error contract only in this card (the method body lands in card 8). Define `type PullResult struct` with these fields, each documented: `WeftPulled bool` (weft ff-pull ran and succeeded), `WarpFetched bool`, `WarpAdvanced bool` (warp branch pointer moved, via ff or reconcile reset), `NewWarpHEAD string` (the fetched upstream SHA warp advanced to, empty if unchanged), `RewriteDetected bool` (warp pull was a non-fast-forward / history rewrite), `Reconciled bool` (a re-anchor weft commit was written), `AnchorWarpSHA string` (the surviving correspondence entry's warp SHA — the confirmed re-anchor baseline), `AnchorWeftSHA string` (that entry's weft SHA — the baseline the PATTERN-residue range starts from), `ReanchorWeftSHA string` (the new empty weft anchor commit's SHA, bound to `NewWarpHEAD`), and `PatternResidue []PatternResidueEntry`. Define `type PatternResidueEntry struct { WeftSHA string; Paths []string }` (a post-anchor weft commit and the `_pattern/...` paths it touched). Define `type PartialPullError struct { WeftPulled bool; Stage string; Err error }` mirroring `PartialCommitError`'s shape (commit.go): `WeftPulled` is always true for this type (weft succeeded, warp failed), `Stage` names which warp-side step failed (e.g. `"fetch"`, `"reset"`, `"reanchor"`), and it implements `Error() string` (a message stating weft succeeded and the named warp stage failed) plus `Unwrap() error` returning `Err`. Define two sentinel errors with `errors.New`, matching the `ErrStaleSHA`/`ErrNoCorrespondence` style: `ErrWarpDivergedUnpushed` ("fabricengine: warp remote diverged and local warp has unpushed commits; aborting, no changes") and `ErrNoSurvivingAnchor` ("fabricengine: warp history rewritten and no recorded correspondence survives; aborting, no changes"). Add the file's package-doc-style header comment describing the read/reconcile path, echoing commit.go's header style.
- **Commit:** `feat(fabricengine): add PullResult and PartialPullError contract`

### Card 8: Fabric.Pull orchestration

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/anchor.go`
  - `internal/gitrepo/pull.go`
  - `internal/gitrepo/ancestry.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/reset.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/fabricengine/pull.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (f *Fabric) Pull(opts SyncOptions) (PullResult, error)` and a private helper `func (f *Fabric) warpUpstreamSHA() (string, error)` to `pull.go`. `warpUpstreamSHA` runs `gitexec.RunGit([]string{"rev-parse", "@{u}"}, f.warpPath)` and returns the trimmed SHA, wrapping a non-zero exit / spawn error (this resolves the fetched upstream tip to a plain hex SHA usable by `ResetHard` and `IsAncestor`). `Pull` implements this exact flow:
  1. If `opts.SkipGit`, return a zero `PullResult{}, nil` immediately (honour the uniform bypass, matching `PullWeft`).
  2. **Weft first.** Call `f.PullWeft(opts)`. On error, return `PullResult{}` and the wrapped error — warp is never touched (`unified-pull-dispatch` / `pull-partial-failure-contract`). On success set `result.WeftPulled = true`. Every warp-side failure AFTER this point returns a `*PartialPullError{WeftPulled: true, Stage: <step>, Err: err}` (never unwind the weft pull).
  3. **Capture local-unpushed BEFORE fetch.** `hadUnpushed, err := f.Warp.HasUnpushed()` — this MUST run before the fetch so `@{u}` still names the pre-fetch upstream tip (`warp-refresh-primitives`, discussion r5 gap). On error return `*PartialPullError{Stage: "unpushed-check"}`.
  4. **Fetch.** `f.Warp.Fetch()`; on error → `*PartialPullError{Stage: "fetch"}`. Set `result.WarpFetched = true`.
  5. Resolve `upstreamSHA, err := f.warpUpstreamSHA()` and `localHEAD, err := f.Warp.CurrentSHA()`; either error → `*PartialPullError{Stage: "resolve"}`.
  6. **Up-to-date:** if `localHEAD == upstreamSHA`, nothing to advance — return `result` (no rewrite, no reconcile).
  7. **Classify:** `isFF, err := f.Warp.IsAncestor(localHEAD, upstreamSHA)` (error → `*PartialPullError{Stage: "classify"}`).
  8. **Clean fast-forward** (`isFF == true`): `f.Warp.ResetHard(upstreamSHA)` (error → `*PartialPullError{Stage: "reset"}`); set `result.WarpAdvanced = true`, `result.NewWarpHEAD = upstreamSHA`; return `result` (no `RewriteDetected`, no reconcile — a clean ff cannot orphan any recorded `Warp-SHA`).
  9. **History rewritten** (`isFF == false`): set `result.RewriteDetected = true`.
     - If `hadUnpushed` → return `result` and `ErrWarpDivergedUnpushed`, making NO change to either repo (the double-conflict abort).
     - Load the correspondence index: `path, _ := f.corrIndexPath()`, `ix, _ := loadCorrIndex(path)`, `entries := ix.entries()` (errors → `*PartialPullError{Stage: "load-index"}`).
     - **Empty index** (`len(entries) == 0`): no recorded history to orphan (`rebase-detection-scope` empty-index case) — advance only: `f.Warp.ResetHard(upstreamSHA)`, set `WarpAdvanced`/`NewWarpHEAD`, return `result` (`Reconciled` stays false).
     - Otherwise **anchor walk:** `anchor, found, err := reachableAnchor(entries, func(sha string) (bool, error) { return f.Warp.IsAncestor(sha, upstreamSHA) })` (error → `*PartialPullError{Stage: "anchor-walk"}`).
       - `!found` → return `result` and `ErrNoSurvivingAnchor`, NO change to either repo.
       - `found` → **reconcile:** `f.Warp.ResetHard(upstreamSHA)` (warp only; error → `*PartialPullError{Stage: "reset"}`), then set `WarpAdvanced`/`NewWarpHEAD`, `result.AnchorWarpSHA = anchor.WarpSHA`, `result.AnchorWeftSHA = anchor.WeftSHA`. Capture `weftHEADBeforeAnchor, _ := f.Weft.CurrentSHA()` for the residue range (card 9). Write the re-anchor weft commit under the weft write lock exactly like `CommitWeft` does: `lockDir, err := f.ensureWeftLockDir()`, `l, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))`, `defer l.Release()`; compose `msg := appendWarpSHATrailer("fabric: re-anchor weft after warp rebase", upstreamSHA)`; call `reanchorSHA, _, err := f.commitEmptySnapshot(msg, upstreamSHA)` (this lands the empty weft commit carrying `Warp-SHA: <upstreamSHA>` and records the correspondence — reusing the exact `tags-force-a-weft-commit` empty-commit mechanism). A reconcile-commit error → `*PartialPullError{Stage: "reanchor"}` (report-not-rollback: warp already advanced, weft not re-anchored; a subsequent `Fabric.Pull` re-detects and retries). Set `result.Reconciled = true`, `result.ReanchorWeftSHA = reanchorSHA`. Compute PATTERN residue (card 9) over `anchor.WeftSHA..weftHEADBeforeAnchor`, set `result.PatternResidue`. Return `result, nil`.

  Import `gitexec`, `lock`, `filepath` as needed (all already used elsewhere in the package). Do NOT use `f.Warp.SHAExists` anywhere in this flow — reachability only (Shared Decision "reachability, never object-existence").
- **Commit:** `feat(fabricengine): add Fabric.Pull unified pull + reconcile`

### Card 9: PATTERN residue enumeration

- **Context:**
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/fabric.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/pull.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a private helper `func (f *Fabric) patternResidueCommits(fromWeftSHA, toWeftSHA string) ([]PatternResidueEntry, error)` to `pull.go` and wire it into `Fabric.Pull`'s reconcile branch (card 8) to populate `result.PatternResidue`. It enumerates the weft commits in the exclusive range `fromWeftSHA..toWeftSHA` that touch `_pattern/...` paths, via one `gitexec.RunGit` call in `f.weftPath`: `git log --name-only --format=<sep-format> <fromWeftSHA>..<toWeftSHA> -- <patternDir>` where `patternDir` is `hubgeometry.PatternDirName` (NEVER a `"_pattern"` string literal — the Hub Geometry Invariant reserves that token to `hubgeometry`; a git-pathspec slice-literal argument sourced from the exported constant is the compliant form). **Separator placement caveat:** unlike `scanWarpSHATrailers` (which uses NO `--name-only`), `--name-only` appends each commit's changed-file list as separate lines AFTER that commit's `--format` output, so a *trailing* record separator would land between a commit's own SHA and its own file list, misassigning paths to the wrong commit. The record separator must therefore LEAD each commit's block (place it at the START of the `--format` string, so it delimits the boundary BEFORE each commit's SHA), not trail it — verify the parse empirically against card 10's `TestPull_IdentifiesPatternResidue` (which will catch a misalignment). Reuse the control-character unit/record separator constants `scanWarpSHATrailers` (index.go) already establishes (`warpSHATrailerFormatUnitSep`/`warpSHATrailerFormatRecordSep`) so the split can never be confused by commit content. Parse the output into one `PatternResidueEntry{WeftSHA, Paths}` per commit, where `Paths` are the `_pattern/...` file paths that commit changed; factor the parse into a small pure helper if it aids a Tier-1 test, but a dedicated unit test is optional here since card 10's integration matrix asserts the end-to-end residue identification. **RelPath-blind scope (documented limitation):** the pathspec is the bare `hubgeometry.PatternDirName` at the weft worktree root, matching the slice's `relpath-is-dot-for-slice-2` precedent (the same simplification `Fabric.Commit` already accepts and the integration tests exercise at `RelPath == "."` via `lyxtest.CopyWeft`). Add a one-line godoc note that this residue scan is `RelPath`-blind — a subpath-anchored hub whose `_pattern` lives at `RelPath/_pattern` in a shared weft checkout is out of scope for this slice, consistent with the accepted precedent — rather than resolving the layout `RelPath` here. If `fromWeftSHA == toWeftSHA` (no post-anchor commits) return `(nil, nil)` without spawning git. A non-zero git exit returns a wrapped error (surfaced by card 8's reconcile branch as `*PartialPullError{Stage: "residue"}`); the empty-output case (a real range with zero `_pattern`-touching commits) returns `(nil, nil)`.
- **Commit:** `feat(fabricengine): enumerate PATTERN residue in PullResult`

### Card 10: Fabric.Pull integration matrix

- **Context:**
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/anchor.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/coalesce_integration_test.go`
  - `internal/fabricengine/syncweft_integration_test.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/trailer.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/pull_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/pull_integration_test.go` (first line `//go:build integration`, package `fabricengine`). Reuse this package's existing fixture helpers (all in the same package, no redefinition): `newPlainWarpRepo`, `commitWarp`, `currentSHA`, `newFabric`, `commitWeftWithTrailer` (index_integration_test.go); `addWarpBareRemote`, `commitPlain`, `bareBranchSHA` (coalesce_integration_test.go); `lyxtest.CopyWeft` for the weft side (its upstream tracking lets `PullWeft`'s ff-pull no-op cleanly). Build the rebased-remote fixture by modelling on `coalesce_integration_test.go`'s `addWarpBareRemote` + second-clone divergence pattern, then rewrite warp history in the bare remote (a second clone that resets to an earlier commit, commits a different history, and force-pushes) so the latest recorded `Warp-SHA` becomes unreachable from the new remote tip. All `TestPull_*` functions (so the batch verify's `-run TestPull` selects them). Cover every case from discussion.md's Testing section:
  - `TestPull_DetectsDriftUnreachableUnprunedObject`: after fetch the rebased-away warp commit's object still physically exists (git fetch never prunes) yet is unreachable — assert `Fabric.Pull` still detects the rewrite (`RewriteDetected == true`) and reconciles, guarding against any regression to `SHAExists`-style detection.
  - `TestPull_ReanchorsSingleCommitBack` and `TestPull_ReanchorsMultiCommitBack`: assert `AnchorWarpSHA`/`AnchorWeftSHA` resolve to the correct nearest-older reachable correspondence for both a one-back and a several-back rewrite.
  - `TestPull_IdempotentAfterReconcile`: a second immediate `Fabric.Pull` on the reconciled pair reports `RewriteDetected == false` and `Reconciled == false` (the new anchor commit made detection idempotent).
  - `TestPull_LeavesWeftHistoryUntouched`: the pre-existing weft content commits are unchanged by the warp-only reset — only the one new empty re-anchor commit is added on top.
  - `TestPull_IdentifiesPatternResidue`: seed a synthetic `_pattern/PATTERN.md` weft commit (plus a non-`_pattern` weft commit) after the anchor point and assert `PatternResidue` names exactly the `_pattern`-touching commit(s), not the others.
  - `TestPull_AbortsOnUnpushedPlusDiverged`: local warp has an unpushed commit AND the remote diverged — assert `Fabric.Pull` returns `ErrWarpDivergedUnpushed` (via `errors.Is`) and mutates NOTHING (warp HEAD and weft HEAD both unchanged).
  - `TestPull_NoSurvivingAnchorAborts`: the entire recorded warp history is rewritten away (no correspondence entry reachable) — assert `ErrNoSurvivingAnchor` and no mutation.
  - `TestPull_CleanFastForwardAdvancesWarp`: a plain fast-forward warp remote — assert warp's local branch actually MOVED to the fetched tip (`WarpAdvanced == true`, HEAD equals the new tip), `RewriteDetected == false`, and weft history untouched (regression guard against a fetch-only no-op).
  - `TestPull_EmptyIndexNoDrift`: a non-ff remote but an empty correspondence index — assert warp advances with no reconcile commit (`Reconciled == false`).
  - `TestPull_WeftPullFailsWarpUntouched`: force the weft ff-pull to fail (e.g. diverge weft locally) and assert warp is never fetched/reset and the error surfaces with warp HEAD unchanged.
- **Commit:** `test(fabricengine): integration matrix for Fabric.Pull`

## Batch Tests

`verify` runs two scopes: (1) `go test ./internal/fabricengine/ -run TestReachableAnchor` — the Tier-1 pure anchor-walk unit test (no git, compiles the untagged surface); (2) `go test -tags integration ./internal/fabricengine/ -run TestPull` — the full `Fabric.Pull` integration matrix, scoped by `-run TestPull` to this slice's new tests rather than fabricengine's large existing integration suite. Detection/reconcile/abort/idempotency/PATTERN-residue/clean-ff/empty-index/weft-first-partial-failure are all covered by card 10's named cases. The whole-repo `go test ./...` done-gate backstops any cross-package regression.
