# Batch: MergeIn and the lifecycle quartet

```yaml
task: 'fabric: merge-conflict primitive'
batch: MergeIn and the lifecycle quartet
number: 3
cards: 4
verify: go test ./internal/fabricengine/ ./cmd/lyx/ && go test -tags integration -run Merge ./internal/fabricengine/
depends-on: [2]
```

## Batch Scope

Delivers the first public merge surface: `MergeOptions`/`MergeResult`, `MergeIn` (merge a source branch into the current pair — where conflicts are surfaced and resolved), and the lifecycle quartet `MergeContinue`/`MergeAbort`/`MergeInProgress`, plus the two new mutation `Kind` members and every same-commit guard-test entry.
The external interface batch 4 consumes: `MergeResult`, the guard/source-resolution helpers in `mergeguards.go`, and the shared conclude phase in `mergelifecycle.go`.
Batch-local decision: the guard/freshness helpers land in card 7 sized for both verbs (per-side source resolution and the upstream helper), so batch 4 adds `Merge`'s guard set without reshaping them.

## Cards

### Card 7: mergeguards.go — guard evaluation, source resolution, freshness

- **Context:**
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/dirtiness.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/mergeerrors.go`
  - `internal/fabricengine/mergestate.go`
  - `internal/gitrepo/merge.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/pull.go`
  - `internal/gitrepo/ancestry.go`
  - `internal/gitexec/gitexec.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergeguards.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/fabricengine/mergeguards.go` with the shared precondition machinery (all unexported):
  - `type mergeSources struct` holding, per side, the resolved SHA actually to merge (fields may use warp/weft naming — owner set).
  - `resolveMergeSources(f *Fabric, l *lyxcwd.Location, source string) (mergeSources, []string)` — implements the freshness rule from the guards decision, per side, warp side on `source`, weft side on `WeftBranchName(source)` (the sole `-weft` composition, `branchname.go`):
    best-effort `Fetch()` on that side's repo first (a fetch failure is tolerated and written to the internal log via `logger.Warn`, never fatal — millhouse's fetch-then-prefer-origin rule);
    resolve the local branch and `origin/<branch>` via `gitrepo.ResolveSHA`;
    merge the remote-tracking SHA when the local branch is absent or is an ancestor of it and not equal (`IsAncestor`), the local SHA otherwise;
    a warp source resolvable on neither local nor remote appends the closed-set reason `mergeReasonSourceNotFound`;
    a weft counterpart existing neither locally (`weftBranchExists(l, ...)`) nor as `origin/<source>-weft` post-fetch appends `mergeReasonNotFabricManaged` (see the Shared Decision on the remote-only weft counterpart).
    Both reasons are collected, never returned early — every guard is evaluated.
  - `pairDirtyReason(f *Fabric) ([]string, error)` — `worktreeDirty(scopeTracked, dir)` (`dirtiness.go`) on both checkouts, evaluating both before combining;
    either side dirty appends `mergeReasonWorktreeDirty` once (the deduplicated aggregate never reveals which side, nor that two subjects were checked).
  - `upstreamSHAAt(dir string) (sha string, hasUpstream bool, err error)` — `gitexec.Run([]string{"rev-parse", "@{u}"}, dir)`, classifying a `*gitexec.GitError` as no-upstream `(false)`, following `weftHasUpstream`'s classification in `pull.go` (checked call — the fabricengine pinned raw-site count stays 2).
    Consumed by batch 4's sync guard;
    declared here so batch 4 does not reshape this file.
  - `mergeInProgressReason(f *Fabric) ([]string, error)` — `mergeRecordExists()` → `mergeReasonAlreadyInProgress`.

  Every helper returns reasons for aggregation via `newMergeGuardError`, and none mutates anything — the upstream *sync* (a mutation) is deliberately not here;
  it is a batch-4 pre-merge step, per the guards decision.
- **Commit:** `feat(fabricengine): merge guard evaluation and source-freshness resolution`

### Card 8: MergeIn, the lifecycle quartet, and the merge mutation kinds

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/mergeerrors.go`
  - `internal/fabricengine/mergestate.go`
  - `internal/fabricengine/mergepaths.go`
  - `internal/fabricengine/mergeguards.go`
  - `internal/fabricengine/fabric.go`
  - `internal/gitrepo/merge.go`
  - `internal/gitrepo/ancestry.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lock/lock.go`
- **Edits:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/mutation_test.go`
  - `internal/fabricengine/livestate_mutationoracle_test.go`
  - `internal/fabricengine/livestate_mutationoracle_selftest_test.go`
  - `cmd/lyx/destructiveguard_test.go`
- **Creates:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergelifecycle.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  **`internal/fabricengine/merge.go`** — the exported types, verbatim from the discussion's `public-surface-shapes` decision:

  ```go
  type MergeOptions struct {
      Squash  bool
      Message string
  }

  type MergeResult struct {
      MutationRecord
      AlreadyUpToDate bool     `json:"already_up_to_date"`
      Conflicts       []string `json:"conflicts"`
      Committed       bool     `json:"committed"`
  }

  func (f *Fabric) MergeIn(source string) (MergeResult, error)
  ```

  (`Merge` itself is batch 4; leave a stub-free file — only what this card ships.)
  `Conflicts` is empty and never nil when there are none.
  Conflicts are a result state, not an error: `MergeIn` with conflicts returns `(MergeResult{Conflicts: […]}, nil)`.

  `MergeIn` flow, in order:
  1. `rec := NewMutations(filepath.Dir(f.warpPath))` + `defer func() { res.Mutations = rec.Snapshot() }()` — the `Pull` allocation pattern.
  2. Resolve `l := lyxcwd.ResolveWorktree(f.warpPath)` and geometry via `resolveMergeGeometry` — a failure here is a guard-stage failure: wrapped error, nothing started, no record written.
  3. Foreign-state refusal: `foreignMergeStatePresent()` true with no record → return `&ErrForeignMergeState{}`.
  4. Aggregate guards, evaluating every member: `mergeInProgressReason` + `pairDirtyReason` + `resolveMergeSources`;
     any reasons → `newMergeGuardError(reasons)`, nothing mutated.
  5. Pre-lock already-up-to-date probe: each side's resolved source SHA already an ancestor of that side's HEAD (`IsAncestor`) on **both** sides → return `MergeResult{AlreadyUpToDate: true}` with no lock taken, no record written, empty mutation record — the degenerate no-op (`Commit`'s precedent).
  6. Acquire the combined write lock (`ensureWeftLockDir` + `lock.AcquireWriteLock`, the `commitBothSides` idiom), released by defer before returning — never held across the conflict-resolution window.
  7. Capture pre-merge HEAD SHAs on both sides;
     write the merge-state record (`Verb: "merge-in"`, `Source`, `Squash: false` — `MergeIn` never squashes — empty `Message`, both start SHAs, `StartedAt` now) via `saveMergeState` **before** the first merge command.
  8. Attempt both sides unconditionally, warp first then weft (fixed order), via `MergeStart(<resolved SHA>, false)` — SHAs, never branch names, on both sides (`merges-name-a-sha-never-a-branch`).
     Write each side's outcome into the record and save;
     record `KindMergeStaged` per the Shared Decision's scenario-symmetric rule (per side with outcome ≠ already-up-to-date, `Target` = checkout path, `Detail` = the merged source SHA).
     A `MergeStart` call that returns a genuine error (anything other than the `MergeConflicted` classification, which card 1 returns with a nil error) on either side self-aborts the whole attempt per the Shared Decision on mid-attempt errors: `resetMergeSides(rec, warpStart, weftStart)`, `deleteMergeState()`, return the wrapped error — identical disposition whichever side failed, git cause to `logger.Warn` only;
     a failing reset retains the record and returns the reset error instead.
  9. Collect `ConflictedFiles()` from each conflicted side and unify via `unifyConflictPaths`.
     `unmappable` → self-abort: `resetMergeSides(rec, warpStart, weftStart)`, `deleteMergeState()`, log the offending paths via `logger.Warn` (internal log only), return `&ErrUnmergeableState{}`.
  10. Any conflicts → return `MergeResult{Conflicts: unified}, nil` — record retained on disk, lock released, worktree left mid-merge for resolution.
  11. Both sides clean → conclude via `concludeMergeSides` (below), then `RecordCorrespondence(warpHEAD, weftHEAD)` — pairing the post-merge HEADs even when one is unchanged — `deleteMergeState()`, return `MergeResult{Committed: true}`.

  **`internal/fabricengine/mergelifecycle.go`** — the quartet, verbatim signatures:

  ```go
  func (f *Fabric) MergeContinue(msg string) (MergeResult, error)
  func (f *Fabric) MergeAbort() (MergeResult, error)
  func (f *Fabric) MergeInProgress() (bool, error)
  ```

  plus the shared conclude phase:
  - `concludeMergeSides(f, rec, st, msg)` — under the already-held lock, conclude warp first then weft (fixed internal order), skipping any side whose recorded outcome is `fast_forwarded` or `up_to_date` (no commit is fabricated on a no-op side) and any side whose recorded committed SHA is already set (idempotency).
    Message precedence: explicit `msg` → `st.Message` → empty (git's prepared `MERGE_MSG`/`SQUASH_MSG` via `MergeConclude("")`).
    After each side lands, write its new SHA into the record's committed field and save, and record `KindMergeCommitted` (`Target` = checkout path, `Detail` = new SHA).
    If a conclude fails after the other side landed (or first — same handling), nothing is rolled back: keep the record, log the git failure internally, return `&ErrMergeIncomplete{}`.
  - `MergeContinue`: no record and no foreign git merge state → `&ErrNoMergeInProgress{}`;
    no record but `foreignMergeStatePresent()` true → `&ErrForeignMergeState{}`, per the Shared Decision on foreign-state disposition, leaving that state untouched.
    Unmerged entries remaining on either side (`ConflictedFiles` both sides, evaluated both) → `newMergeGuardError([]string{mergeReasonUnresolvedConflicts})`.
    Otherwise: acquire the lock, run `concludeMergeSides` with the optional message override, then correspondence + `deleteMergeState`, return `MergeResult{Committed: true}`.
  - `MergeAbort`: no record and no foreign git merge state → `&ErrNoMergeInProgress{}`;
    no record but foreign state present → `&ErrForeignMergeState{}`, same rule as `MergeContinue`.
    Otherwise: acquire the lock, `resetMergeSides(rec, st.WarpStart, st.WeftStart)` — both sides unconditionally, including a fast-forwarded side and a side that never moved (`git reset --hard` also clears `MERGE_HEAD`/merge index as a side effect, covering the `MERGE_HEAD`, squash, and fast-forward cases with one mechanism) — then `deleteMergeState`, return `MergeResult{}` (Committed false).
  - `MergeInProgress`: `mergeRecordExists()` — read-only, returns a bare bool, no `MutationRecord` (stays off the embed table by design).
    It never consults `foreignMergeStatePresent` and never errors on foreign state: it answers "does fabric have a merge in progress", which foreign plain-git state does not make true (Shared Decision on foreign-state disposition).

  **`internal/fabricengine/mutation.go`** — same commit as the recording sites, per the Mutation Record Invariant:
  add `KindMergeStaged Kind = "merge_staged"` ("a merge command observably changed a checkout's state") and `KindMergeCommitted Kind = "merge_committed"` ("a merge conclude-commit landed; Detail is the new SHA"), and correct the header comment's stale member counts.
  Guard entries in the same commit:
  - `internal/fabricengine/mutation_test.go`: extend its `Kind` pins with the two new members, following the existing entries' shape.
  - `internal/fabricengine/livestate_mutationoracle_test.go`: classify both kinds in `manifestObservableKind` and `worktreeRootedKind`, mirroring `KindWorktreeReset`'s entries (`manifestObservable: false`, `worktreeRooted: true`);
    update `livestate_mutationoracle_selftest_test.go` if its completeness self-test pins the map cardinalities.
  - `cmd/lyx/destructiveguard_test.go`: add `{"MergeResult", "internal/fabricengine/merge.go"}` to `destructiveGuardMutatingResultTypes`.
- **Commit:** `feat(fabricengine): MergeIn and the merge lifecycle quartet`

### Card 9: MergeIn scenario-matrix integration tests

- **Context:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/pull_integration_test.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergein_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `//go:build integration`, `package fabricengine_test`, test names containing `Merge`.
  Build a shared fixture helper `newMergePairFixture(t, anchor string)` (exported within the test package for reuse by batches 4–5): `hubforge.NewHub` + `AddPair`, returning the hub, the pair's `*fabricengine.Fabric` (via `lyxcwd.ResolveWorktree` + `fabricengine.Open`), and closures for making commits on the prime (source) branches and the pair branches on either side via `gitkit.MustRun` — divergence is manufactured by committing conflicting edits to the same file/line on source and pair branches, and weft-side content is written through the pair's junction or directly in the weft checkouts.
  Each scenario asserts the *pair's* end state, never either side's individually, unless the assertion is itself about internal state via `export_test.go` seams:
  - Both sides clean → `Committed: true`, both concluded, correspondence recorded (probe via `WeftSHAForWarpSHA`), record deleted, `MergeInProgress` false.
  - Warp conflicts, weft clean → nothing committed on either side, `(MergeResult{Conflicts: […]}, nil)`, record present, `MergeInProgress` true.
  - Weft conflicts, warp clean → the observable outcome is **byte-identical** to the previous case: marshal both `MergeResult`s to JSON and compare shapes (conflict paths differ only as paths), compare error values, and assert the mutation records carry the same kinds against the same target set in the same order.
    This is the single most important test in the task.
  - Both sides conflict → one flat, lexically sorted list containing paths originating from both, in the unified namespace, no duplicates.
  - One side fast-forwards while the other conflicts → conflicts reported;
    `MergeAbort` returns the fast-forwarded side to its recorded pre-merge SHA (the B1 case, pinned by SHA comparison).
  - One side already up to date, the other merges → concluded, no empty commit fabricated on the no-op side (its HEAD and commit count unchanged);
    correspondence pairs the new SHA with the unchanged one.
  - Both sides already up to date → `AlreadyUpToDate: true`, empty mutation record, no lock artifact touched, no state record written.
  - Conflicts resolved in the worktree (edit + `git add`) → `MergeContinue` concludes both sides, records correspondence, deletes the record;
    `MergeContinue` with unresolved conflicts refuses with a `*MergeGuardError` whose sole reason is the fixed `"unresolved conflicts remain"`.
  - `MergeAbort` after a conflict → both sides at their exact pre-merge SHAs, worktrees clean, record deleted, `MergeInProgress` false.
  - `MergeIn` never squashes: after a clean non-ff `MergeIn`, the warp history shape shows a real merge commit (two parents).
- **Commit:** `test(fabricengine): MergeIn scenario matrix`

### Card 10: MergeIn recovery, freshness, and illusion-integrity tests

- **Context:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/mergein_integration_test.go`
  - `internal/fabricengine/export_test.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergein_recovery_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Same tags/package as card 9, reusing `newMergePairFixture`;
  test names containing `Merge`.
  - Crash recovery: build the mid-merge state (drive a real conflicted `MergeIn`, record on disk), then open a **fresh** `Fabric` handle on the same pair and drive `MergeAbort` → exact two-sided restore;
    separately, a crashed-after-clean-staging state (record present, both sides staged, nothing concluded — build via export seams or by killing the flow at the record: save a record manually over a staged pair) → `MergeContinue` on a fresh handle concludes both.
  - Conclude-phase partial failure: force the weft side's conclude-commit to fail (a `pre-commit` hook in the weft checkout's hooks path that exits 1, installed by the fixture) → `*ErrMergeIncomplete` with the fixed text, record retained, warp's landed SHA recorded;
    removing the sabotage and re-running `MergeContinue` concludes the remaining side only — idempotency pinned by SHA comparison on the already-landed side.
  - Foreign merge state: a plain-git conflicted merge staged directly in the warp checkout → `MergeInProgress()` false and no error;
    `MergeIn`, `MergeContinue`, and `MergeAbort` all refuse with `*ErrForeignMergeState` (per the Shared Decision on foreign-state disposition), and the foreign state is left untouched (same `MERGE_HEAD`, same conflicted files after).
    Separately, with neither a record nor foreign state present, `MergeContinue` and `MergeAbort` return `*ErrNoMergeInProgress` — pinning that the two errors are not interchangeable.
  - Freshness: local source behind its remote-tracking ref → the remote-tracking SHA is merged (assert the merged content is the remote tip's);
    source existing only remotely → merged;
    source resolvable nowhere → `*MergeGuardError` with `"source branch not found"`.
  - Source without a fabric counterpart (`<source>-weft` absent locally and remotely) → `*MergeGuardError` with the fixed `"source branch is not fabric-managed"`, nothing mutated (SHAs unchanged both sides).
  - Dirty pair → `*MergeGuardError` with `"worktree dirty"`;
    dirty-warp-only and dirty-weft-only produce byte-identical error values.
  - Unmappable-path conflict: manufacture a weft-side conflict on a repo-root file outside the wired name-set (commit divergent edits to a weft-root file on both weft branches with `gitkit.MustRun`) → merge aborted on both sides, `*ErrUnmergeableState`, pair restored to pre-merge SHAs, no record left.
  - Conflict-marker content: a weft-only conflict's markers contain no `-weft`-suffixed name — read the conflicted file through the pair's visible worktree and assert the `>>>>>>>` label is the merged source SHA;
    assert the same marker style on a warp-only conflict so the two are indistinguishable.
  - Path mapping on a subpath-anchored hub (`newMergePairFixture(t, "backend")`): a conflict in a junctioned path is reported at its unified worktree-root-relative path (`backend/<name>/…`), and the reported file is reachable at that path through the junction (`os.Stat` from the visible worktree root).
- **Commit:** `test(fabricengine): MergeIn recovery, freshness, and illusion integrity`

## Batch Tests

`verify` runs the untagged fabricengine and `cmd/lyx` suites (mutation-kind pins, result-type embed guard, vocabulary walk) plus every `Merge`-named integration test — cards 9–10 in full alongside batch 2's state/reset tests.
The two new `Kind` members and the `MergeResult` guard row are exercised by `cmd/lyx/destructiveguard_test.go` in the same run.
