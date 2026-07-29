MILL_REVIEW_BEGIN
# Review: fabric: Fabric.Commit classify+dispatch + unified diff/status

```yaml
verdict: APPROVE
reviewer_model: claude-sonnet-5
reviewer_self_id: claude-sonnet-5 (hub session, manual review)
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [MAJOR] `CommitWeft`'s Snapshot-trailer extension is not reconciled with the "no caller migration" scope
**Section:** Technical context, `weftgit.go` bullet (l.115: "Fabric.Commit's weft side should reuse this path (extended to also append `Snapshot:` trailers)"); Scope §Out (l.30: "Migrating existing weft-commit callers ... is pure API addition; caller migration is a later, separate concern").
**Issue:** `CommitWeft(pathspec []string, message string, opts SyncOptions)` has seven existing call sites across six files today (`fabriccli/weft_verbs.go` ×3, `buildercli/weft.go`, `webstercli/weft.go`, `fabricengine/syncweft.go`, `initengine/undo.go`, `perchcli/run.go`), none passing a snapshot-tags argument. "Extended to also append Snapshot: trailers" is stated as settled but never says how — if that means adding a required parameter to `CommitWeft` itself, it force-migrates all seven call sites immediately (a compile break), directly contradicting the Out-of-scope line that caller migration is a later concern. The discussion decides *that* the trailer gets written (snapshot-trailer-written-now) but not the *mechanism* that keeps `CommitWeft`'s existing signature intact while `Fabric.Commit`'s weft side gets the extra trailers.
**Fix:** Add this as an explicit decision or Open item for mill-plan: e.g. a trailing variadic `snapshotTags ...string` on `CommitWeft` (backward-compatible — all seven existing 3-arg calls keep compiling unchanged) or an unexported shared helper both `CommitWeft` (passing nil) and `Fabric.Commit`'s weft path call. Either is cheap; the discussion just needs to say which, so mill-plan doesn't have to invent it mid-implementation.

### [MAJOR] No lock spans the warp-commit → weft-trailer-read window
**Section:** Decisions §"warp-first-ordering" (l.45–49); §"Who materializes the instruction files" is not this doc, ignore — see §"partial-failure-report-not-rollback" (l.51–55).
**Issue:** The warp-first-ordering rationale is explicit: "for the trailer to name the warp commit that includes *this* `Fabric.Commit`'s warp-side files, warp must commit first." But `CommitWeft` re-reads `f.Warp.CurrentSHA()` fresh, inside its *own* lock (`ensureWeftLockDir`/`weftWriteLockFile`), which is only acquired once `Fabric.Commit` calls into `CommitWeft` — after the warp-side `StageAndCommit` has already returned. Nothing in the discussion holds a lock across both steps. If any other actor (a concurrent `Fabric.Commit` call, a human `git commit`, another lyx process) commits to warp in that window, the weft trailer will name a *later* warp SHA than `CommitResult.WarpSHA` — `RecordCorrespondence` then indexes that later SHA, not the one the caller was told about, so a caller doing `WeftSHAForWarpSHA(CommitResult.WarpSHA)` afterward can miss (`ErrNoCorrespondence`) even though this exact call just "succeeded." This silently weakens the guarantee warp-first-ordering is written to establish, and isn't mentioned as an accepted risk anywhere (unlike the async-push race, which the doc explicitly accepts and names).
**Fix:** Either (a) explicitly accept this as a known, low-probability race — consistent with the doc's own "no cross-repo transaction" philosophy — and say so in the decision's Rationale, or (b) hold the weft write lock across both the warp commit and the weft commit (acquire it in `Fabric.Commit` before touching warp, then call an already-locked internal variant of the weft-commit path). Either is fine; the gap is that the discussion doesn't decide it either way.

### [NOTE] Classifier has no stated behavior for malformed input paths
**Section:** Decisions §"classification-input-contract" (l.63–67); Testing §classifier bullet (l.141).
**Issue:** The classifier is specified as a pure function over worktree-root-relative paths, and the Testing section lists a thorough set of boundary cases (path-segment-vs-substring, `RelPath` variants, all-warp/all-weft, empty list) — but nothing states what happens on an absolute path or a path escaping the worktree root via `..`, and no test case covers it. Given `Fabric.Commit`'s "everything in LYX that writes files" caller base, a caller passing a stray absolute path is plausible.
**Fix:** Either state explicitly that the classifier trusts its caller and performs no validation (consistent with `ScopedPathspec`'s own no-validation posture), or add one boundary test/behavior line for it. A one-sentence decision either way closes this rather than leaving it implicit.

### [NOTE] "Mirrors spawnPush/spawnSync" glosses over a real difference in where the skip-env gating lives
**Section:** Decisions §"async-push-both-sides-detached", Mechanism bullet (l.88).
**Issue:** `fabriccli.spawnPush` checks `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` *inside itself* before forking; `boardengine.spawnSync` has no such check internally — its caller (`board.go`'s `if !b.skipGit`) gates the call instead. The discussion cites both as one precedent ("mirroring `spawnPush`/`spawnSync`") for "the async push honors `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH`," but the two precedents actually gate at different layers. This is already substantively covered by the existing "Async-push child wiring" Open item, so it's not a new gap — just worth making explicit there rather than leaving "mirrors both" to imply they're the same shape.
**Fix:** When mill-plan resolves the "Async-push child wiring" open item, pick one layer (helper-internal, matching `spawnPush`) and say so, rather than citing both precedents as interchangeable.

## Verdict

APPROVE
Thorough, well-grounded against the actual `fabricengine`/`gitrepo`/`boardengine`/`fabriccli` source (verified read during this review), and every major decision is rationale-backed with rejected alternatives named. The two MAJOR findings above are implementation-detail-level gaps mill-plan can close by adding explicit decisions/open items — neither requires re-opening a directional decision already made, so nothing here blocks moving to mill-plan.
MILL_REVIEW_END
