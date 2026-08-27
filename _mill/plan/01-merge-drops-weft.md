# Batch: merge-drops-weft

```yaml
task: "Add a local-only file category to weft"
batch: "merge-drops-weft"
number: 1
cards: 7
verify: go build ./cmd/lyx && go test ./internal/fabricengine/... ./internal/lyxcwd/... && go test -tags integration ./internal/fabricengine/...
depends-on: []
```

## Batch Scope

This batch makes the weft stop being a merge participant in either direction: `Fabric.MergeIn` and `Fabric.Merge` merge the warp side only, and the weft's `MergeStart`, its conflict collection, its up-to-date probe's veto, and (in `Merge`) its pre-merge upstream sync are all removed.
`mergeState`'s four weft fields stay in the struct and stay filled — recording the weft as unmoved — so the persisted JSON schema stays byte-compatible in both directions and `mergeAttemptIncompleteReason` keeps working.
Nothing about the weft's power to *block* a merge changes here;
that is batch 2's job, and until batch 2 lands a dirty or diverged weft still refuses these now-warp-only merges.
The external interface batch 2 consumes is `resolveMergeSources`, which this batch leaves untouched.

Batch-local decision beyond `## Shared Decisions`: `concludeMergeSides` keeps both arms and only its doc comment moves.
With `WeftOutcome` written as `mergeOutcomeAlreadyUpToDate`, that function's existing weft guard already skips the weft, so the arm becomes unreachable for records this binary writes while still working for a record a pre-change binary left on disk.

## Cards

### Card 1: MergeIn merges the warp side only

- **Context:**
  - `internal/gitrepo/merge.go`
  - `internal/fabricengine/mergeguards.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/fabricengine/merge.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Fabric.MergeIn` (`internal/fabricengine/merge.go`):
  (a) keep both `f.warp.CurrentSHA()` and `f.weft.CurrentSHA()` reads — pre-lock and re-read-under-lock alike — since `WeftStart` still needs a value;
  (b) delete the `weftUpToDate` computation (`f.weft.IsAncestor(sources.weftSHA, weftStart)`) and narrow the pre-lock degenerate-no-op probe from `if warpUpToDate && weftUpToDate` to `if warpUpToDate`;
  (c) immediately after the first `f.saveMergeState(st)` call, set `st.WeftOutcome = mergeOutcomeAlreadyUpToDate` and re-save, so the record carries the weft as unmoved from the first checkpoint onward;
  (d) delete the `f.weft.MergeStart(sources.weftSHA, false)` call, its `st.WeftOutcome = mergeOutcomeString(weftOutcome)` assignment, its `f.saveMergeState(st)` call, and its `rec.Append(KindMergeStaged, f.weftPath, sources.weftSHA)` guard;
  (e) narrow the conflict condition from `if warpOutcome == gitrepo.MergeConflicted || weftOutcome == gitrepo.MergeConflicted` to `if warpOutcome == gitrepo.MergeConflicted`, delete the `weftConflicts` declaration and its `f.weft.ConflictedFiles()` retrieval, and pass a literal `nil` as `unifyConflictPaths`' second argument;
  (f) leave the `f.weft.CurrentSHA()` read feeding `RecordCorrespondence(newWarpHEAD, newWeftHEAD)` exactly as written, per the `correspondence-unchanged` Decision;
  (g) update `MergeIn`'s own doc comment so it states that the warp side merges `source` and the weft side is not a merge participant, replacing the current "both resolved against the freshness rule" phrasing.
  Do not change `MergeIn`'s exported signature.
  Do not remove the `sources.weftSHA` value, which `st.WeftSource` still records.
- **Commit:** `refactor(fabricengine): MergeIn merges the warp side only`

### Card 2: Merge merges the warp side only and skips the weft pre-merge sync

- **Context:**
  - `internal/gitrepo/merge.go`
  - `internal/fabricengine/mergeguards.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/fabricengine/merge.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Fabric.Merge` (`internal/fabricengine/merge.go`):
  (a) delete the `f.syncSideBeforeMerge(rec, f.weft, f.weftPath, "weft")` call and its `wrapMergeSyncError` return, keeping the warp call unchanged;
  (b) keep the post-sync `f.weft.CurrentSHA()` read for `weftStart`, delete the `weftUpToDate` computation, and narrow the post-sync degenerate-no-op probe from `if warpUpToDate && weftUpToDate` to `if warpUpToDate`;
  (c) immediately after the first `f.saveMergeState(st)` call, set `st.WeftOutcome = mergeOutcomeAlreadyUpToDate` and re-save;
  (d) delete the `f.weft.MergeStart(sources.weftSHA, opts.Squash)` call, its outcome assignment, its `f.saveMergeState(st)` call, and its `rec.Append(KindMergeStaged, f.weftPath, sources.weftSHA)` guard;
  (e) narrow the self-abort conflict condition to `if warpOutcome == gitrepo.MergeConflicted`;
  (f) leave `RecordCorrespondence(newWarpHEAD, newWeftHEAD)` and the `f.weft.CurrentSHA()` read feeding it unchanged;
  (g) update `Merge`'s own doc comment: the pre-merge sync step now synchronizes the warp side alone, and the not-synced precondition's second half applies to warp only;
  (h) leave `syncSideBeforeMerge` itself intact, including its `sideLabel` parameter — it keeps one caller.
  Do not change `Merge`'s exported signature.
  Do not touch `syncedToUpstreamReason`, which batch 2 owns.
- **Commit:** `refactor(fabricengine): Merge merges the warp side only`

### Card 3: correct concludeMergeSides' two-sided doc comment

- **Context:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergestate.go`
- **Edits:**
  - `internal/fabricengine/mergelifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Amend `concludeMergeSides`' doc comment in `internal/fabricengine/mergelifecycle.go` to record that the weft arm is now reachable only for a `fabric-merge.json` record written by a binary predating this change: `MergeIn`/`Merge` write `WeftOutcome: "up_to_date"` from the first checkpoint, and the arm's own `st.WeftOutcome != mergeOutcomeAlreadyUpToDate` guard therefore skips it on every record this binary produces.
  State that the arm is retained deliberately, for cross-binary record compatibility, and not because a weft conclude can still be produced.
  Amend `MergeAbort`'s doc comment in the same file so its "restoring both sides to their pre-merge SHAs" claim is corrected to the warp side alone, and add a forward reference to `resetMergeSides` for why.
  Leave `concludeMergeSides`' executable body byte-for-byte unchanged.
  Leave `MergeContinue`'s body unchanged.
- **Commit:** `docs(fabricengine): correct conclude and abort comments for a non-merging weft`

### Card 4: record the weft as unmoved in mergeState's own documentation

- **Context:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergeguards.go`
  - `internal/fabricengine/mergelifecycle.go`
- **Edits:**
  - `internal/fabricengine/mergestate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/mergestate.go`, amend the `mergeState` struct's doc comment to state that `WeftStart`, `WeftSource`, `WeftOutcome` and `WeftCommitted` now record the weft as unmoved rather than as a merge participant: `WeftStart` is the weft HEAD at record time, `WeftSource` is the best-effort resolved weft counterpart SHA (legitimately empty when no counterpart resolves), `WeftOutcome` is written as `mergeOutcomeAlreadyUpToDate` before any `MergeStart` runs, and `WeftCommitted` stays empty on every path this binary produces.
  State the two reasons the fields are kept rather than dropped: `mergeAttemptIncompleteReason` refuses a resume on an empty `WeftOutcome`, and filling them leaves the persisted JSON byte-compatible in both directions across the binary change.
  Amend `bothSidesAlreadyUpToDate`'s doc comment to record that its weft conjunct is now always satisfied, so the derived `MergeResult.AlreadyUpToDate` answers about the warp side alone.
  Amend `foreignMergeStatePresent`'s doc comment with one sentence recording that it deliberately keeps both weft probes and still refuses a mutating merge verb on weft-side foreign merge state, since the weft is no longer a merge participant and this is the one weft-reading guard the change leaves in place.
  Leave every executable line in this file unchanged.
- **Commit:** `docs(fabricengine): document mergeState's weft fields as unmoved`

### Card 5: rewrite doc.go's merge-surface narrative

- **Context:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/mergestate.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/doc.go`'s `# The merge surface` section, correct every claim that a merge moves both sides.
  Specifically: the "**The two verbs, and why there are two**" paragraph must say `MergeIn(source)` merges `source` into the current pair's warp checkout;
  the "**The lifecycle quartet and crash recovery**" paragraph's "restoring both sides to their pre-merge SHAs" must become the warp side alone;
  the "**What the result flags mean**" paragraph's `AlreadyUpToDate` description must say the attempt found the warp side already carrying the resolved source;
  and the self-abort narrative around `internal/fabricengine/doc.go:858-880` must describe a one-sided reset.
  Add one new paragraph to the same section, titled in the file's existing bold-lead style, stating the rule directly: everything routed to the weft belongs to exactly one worktree and one branch, so the weft is never a merge participant in either direction, and a merge carries code rather than system files.
  Name the two consequences a reader needs: `unifyConflictPaths`' weft list is permanently empty and `fabriccli`'s junction-staging conflict path is unreachable rather than wrong.
  Keep the section's existing heading text unchanged, since `docs/` and `manifest/` link into this package's documentation.
- **Commit:** `docs(fabricengine): rewrite the merge surface narrative for a warp-only merge`

### Card 6: extend the Durable-vs-Ephemeral State Invariant

- **Context:**
  - `_mill/discussion.md`
  - `docs/overview.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add exactly one bullet to `CONSTRAINTS.md`'s existing `## Durable-vs-Ephemeral State Invariant` section, after its current three bullets: weft content is per-branch and is never a merge participant in either direction.
  Introduce no new heading, no new invariant section, and no third state category.
  Match the file's trimmed rules-only voice — no rationale, no narrative, one sentence.
  Follow the repo's semantic-line-break rule: one sentence per line, plain newlines.
- **Commit:** `docs(constraints): weft content is never a merge participant`

### Card 7: integration coverage for the warp-only merge

- **Context:**
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/merge_target_integration_test.go`
  - `internal/fabricengine/mergein_integration_test.go`
  - `internal/fabricengine/mergein_recovery_integration_test.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergeweftlocal_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/mergeweftlocal_integration_test.go` in `package fabricengine_test`, opening with the `//go:build integration` constraint and a file-header comment naming what it pins.
  Reuse `newMergeTargetFixture`, `seedSourceAndTarget`, `newMergePairFixture`, `commitOnCurrentBranch`, `advanceRemoteBranch`, `openFreshFabric`, `gitRevParse` and `fabricengine.CurrentSHAForTest` from the package's existing test files rather than adding new fixture helpers.
  Cover five scenarios, one test function each:
  (1) a `Merge` of a source branch whose weft counterpart rewrote `_lyx/loom/status.json` many times — the target pair's warp HEAD advances, the target pair's weft HEAD is byte-identical before and after, and `MergeResult.Conflicts` is empty;
  (2) the same shape with the target pair carrying its own diverged `_lyx/loom/status.json` — the target's weft file content is unchanged after the merge;
  (3) both sides evolving `_lyx/` from a shared base, which returns `*ErrMergeInRequired` today — it must now complete and report `Committed` true;
  (4) `MergeIn` in the opposite direction — a parent branch's `_lyx/` content never reaches the child's weft worktree and the child's live `_lyx/` content is unchanged;
  (5) a genuine warp-side conflict still reaching `unifyConflictPaths` — `MergeIn` returns a non-empty `Conflicts` list naming the warp path, leaving the pair mid-merge for `MergeContinue`.
  Additionally assert, in scenario 1, that the merge record's `WeftOutcome` is the `up_to_date` string by reading the record through `fabricengine.MergeStatePathForTest` before the verb deletes it, or by asserting `MergeResult.AlreadyUpToDate` is false while the warp genuinely moved.
- **Commit:** `test(fabricengine): pin the warp-only merge against a real pair`

## Batch Tests

`verify:` runs `go build ./cmd/lyx`, then the untagged tier over `./internal/fabricengine/...` and `./internal/lyxcwd/...`, then the `integration` tier over `./internal/fabricengine/...`.

- `./internal/lyxcwd/...` is included because card 6 edits `CONSTRAINTS.md` and `docslink_test.go` in that package is where the Markdown Link Integrity invariant is enforced.
- The `integration` tier is chained as its own ` && ` invocation rather than folded into the first, because card 7 creates an `//go:build integration` file and the existing `merge_target_integration_test.go`, `mergein_integration_test.go` and `mergestate_integration_test.go` suites are the regression surface this batch is most likely to break.
- The scope is one package plus the docs-link package, not `./...`: nothing outside `internal/fabricengine` is edited here, and the repo-wide sweep is `pipeline.done_gate`'s job at the end of the run.
- Cards 3, 4 and 5 are comment-only and carry no test of their own;
  their regression surface is `go build` plus the existing suites already in `verify:`.
