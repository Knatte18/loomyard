# Batch: Merge target-pair verb

```yaml
task: 'fabric: merge-conflict primitive'
batch: Merge target-pair verb
number: 4
cards: 2
verify: go test ./internal/fabricengine/ ./cmd/lyx/ && go test -tags integration -run Merge ./internal/fabricengine/
depends-on: [3]
```

## Batch Scope

Delivers `Fabric.Merge` — merging the task pair into a *target* pair, squash-capable, expected conflict-free — on a handle the caller opens at the target pair's worktree (`lyxcwd.ResolveWorktree` + `fabricengine.Open`; Fabric resolves no topology of its own).
Adds `Merge`'s guard set (dirty target, upstream-sync read-only check with the no-upstream vacuous pass, source resolution, fabric-managed), the mutating pre-merge sync step (fetch then `MergeFFOnly`, recorded as `KindRepoAdvanced`), and the conflict self-abort that redirects to `MergeIn` via `*ErrMergeInRequired`.
The external interface batch 6 consumes is the completed five-verb Go surface.
Batch-local decisions: none beyond Shared Decisions.

## Cards

### Card 11: Fabric.Merge with sync-before-merge and self-abort redirect

- **Context:**
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/mergestate.go`
  - `internal/fabricengine/mergepaths.go`
  - `internal/fabricengine/mergeerrors.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/mutation.go`
  - `internal/gitrepo/merge.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergeguards.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/fabricengine/merge.go` the verbatim-pinned signature:

  ```go
  func (f *Fabric) Merge(source string, opts MergeOptions) (MergeResult, error)
  ```

  and to `mergeguards.go` a `syncedToUpstreamReason(f *Fabric) ([]string, error)`:
  per side, `upstreamSHAAt(dir)`;
  a side with no upstream passes vacuously (`Fabric.Pull`'s no-upstream rule);
  with an upstream, the read-only guard-stage check is "the target tip is not diverged from its upstream" — upstream ancestor of HEAD (in sync or ahead) passes, HEAD ancestor of upstream (behind) passes (the sync step will advance), neither direction (genuinely diverged) appends the closed-set reason `mergeReasonNotSynced`.
  Both sides evaluated before combining.

  `Merge` flow, in order:
  1. Recorder allocation + `defer` snapshot, exactly as `MergeIn`.
  2. `lyxcwd.ResolveWorktree(f.warpPath)` + `resolveMergeGeometry` — guard-stage failure semantics as in `MergeIn`.
  3. Foreign-state refusal → `&ErrForeignMergeState{}`.
  4. Aggregate guards, all evaluated: `mergeInProgressReason` + `pairDirtyReason` + `syncedToUpstreamReason` + `resolveMergeSources` (source resolvable, fabric-managed — same helpers and reasons as `MergeIn`).
     Any reasons → `newMergeGuardError`, nothing mutated (the guard stage is strictly read-only).
  5. Pre-merge sync step — runs only after every guard passed, and is a recorded mutation, not a guard:
     per side with an upstream, best-effort `Fetch()` (failure tolerated and logged via `logger.Warn`), re-resolve the upstream SHA, and when HEAD is strictly behind it advance via `MergeFFOnly(upstreamSHA)` — `merge --ff-only`, never `reset --hard`, failing loudly on a raced divergence — recording `KindRepoAdvanced` (`Target` = checkout path, `Detail` = new SHA, the `Pull` precedent).
  6. Post-sync already-up-to-date probe: both sides' resolved source SHAs already ancestors of their HEADs → return `MergeResult{AlreadyUpToDate: true}` without taking the lock or writing a record (the sync advances, if any, remain in the mutation record — they are real upstream catch-up the merge did not cause).
  7. Acquire the combined write lock;
     capture pre-merge SHAs **after** the sync (so `MergeAbort` returns the pair to its synced state, never undoing a legitimate upstream advance);
     `saveMergeState` (`Verb: "merge"`, `Source`, `Squash: opts.Squash`, `Message: opts.Message`, start SHAs, `StartedAt`) before the first merge command.
  8. Attempt both sides unconditionally, warp then weft, `MergeStart(<resolved SHA>, opts.Squash)` — `Squash` applied identically to both sides;
     record outcomes into the state record and `KindMergeStaged` per the scenario-symmetric rule.
     A genuine (non-`MergeConflicted`) error from `MergeStart` on either side self-aborts identically to `MergeIn`'s step 8, per the Shared Decision on mid-attempt errors: `resetMergeSides(rec, warpStart, weftStart)`, `deleteMergeState()`, return the wrapped error — never `*ErrMergeInRequired`, which is reserved for the conflict case in step 9;
     a failing reset retains the record and returns the reset error.
  9. Any conflict on either side (mappable or not) → self-abort: `resetMergeSides(rec, warpStart, weftStart)`, `deleteMergeState()`, return `&ErrMergeInRequired{Source: source}` — the target pair is restored exactly, no conflicted state is ever left behind, and the conflicting side is not disclosed (fixed message; the source travels in the field).
  10. Both clean → `concludeMergeSides` (message precedence `opts.Message` → git's prepared `MERGE_MSG`/`SQUASH_MSG`), `RecordCorrespondence` on the post-merge HEADs, `deleteMergeState`, return `MergeResult{Committed: true}`.
     A conclude failure returns `&ErrMergeIncomplete{}` with the record retained — the quartet's crash-recovery role covers `Merge` too, driven by the same record.
- **Commit:** `feat(fabricengine): Merge — squash-capable target-pair merge with sync and self-abort`

### Card 12: Merge target-verb integration tests

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
  - `internal/fabricengine/merge_target_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `//go:build integration`, `package fabricengine_test`, reusing `newMergePairFixture` plus a second pair (or the prime pair) as the merge target;
  the target handle is opened via `lyxcwd.ResolveWorktree(<target worktree>)` + `fabricengine.Open`, driven while cwd is elsewhere — proving the path-anchored, cwd-gate-free Go API the handle decision requires.
  Test names contain `Merge`.
  - Clean squash merge of a task pair into the target → `Committed: true`;
    history shape on both sides is a single new commit with one parent (squash), task-side history absent from target;
    correspondence recorded for the target pair;
    record deleted.
  - Clean non-squash merge → merge commit with two parents on both sides (plain `git merge` semantics preserved).
  - `Message` empty → each side's commit uses git's prepared message;
    `Message` set → used verbatim on both sides.
  - `Merge` with a dirty target → halts before mutating anything on either side (SHAs unchanged, no record);
    dirty-warp-only and dirty-weft-only produce byte-identical `*MergeGuardError` values (guard-report shape pinned).
  - `Merge` with a stale target ref (behind its upstream) → fetches and fast-forwards via the sync step (`KindRepoAdvanced` recorded), then merges;
    with a genuinely diverged target → `*MergeGuardError` with `"branch not synced to upstream"`, mutating nothing;
    with a no-upstream side → guard passes vacuously and the result is indistinguishable from the with-upstream clean case (compare result values and mutation kinds).
  - `Merge` that would conflict → self-aborts: target pair's SHAs and worktrees exactly unchanged, `*ErrMergeInRequired` with `Source` set, no record left, `MergeInProgress` false, and the error value is identical whether the warp or the weft side would have conflicted.
  - Both sides already up to date → `AlreadyUpToDate: true`, no record written, no lock taken.
  - Crash recovery for `Merge`: a record left mid-flight (manufactured over a staged target) → `MergeInProgress` true on a fresh handle, `MergeAbort` restores the target pair, and `MergeContinue` concludes a crashed-after-clean-staging merge.
- **Commit:** `test(fabricengine): Merge target-pair scenarios`

## Batch Tests

`verify` as in batch 3: untagged fabricengine + `cmd/lyx` guard suites plus all `Merge`-named integration tests, which now include the full target-verb matrix of card 12 alongside batches 2–3's tests.
