MILL_REVIEW_BEGIN
# Review: loom: session bootstrap

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build unverifiable from inside the session
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:design] CommitWeftPaths states no weft write-lock discipline
**Section:** `weft-commit-mechanism`
**Issue:** `commit.go:1-4,73` documents that every committing `Fabric.Commit` serialises on the combined weft write lock (`.weft/`, `weftWriteLockFile`), and `weftgit.go:333-336` says `commitWeftAt` "does not acquire the weft write lock; the caller is responsible" — the proposed `CommitWeftPaths` says only "holds no `Fabric`" and never states which side of that it lands on, so loom's seed commit can race a concurrent webster/fabric weft commit in the same pair on `.git/index.lock`.
**Fix:** State whether `CommitWeftPaths` acquires the same combined write lock itself (and how it reaches the lock dir without a `Fabric`), or that its callers must.

### [BLOCKING:design] Anchor-relative handling missing from both new weft paths
**Section:** `weft-commit-mechanism`, `origin-record-ownership-seam`
**Issue:** Every production `ScopedPathspec` call passes `l.AnchorRel` as `relPath` (`fabriccli/weft_verbs.go:100`, `pull.go:436`, `status.go:179`) and weft `_lyx` sits at `WeftWorktree/AnchorRel/_lyx` (`fabric.go:150`), but the pinned signature `CommitWeftPaths(weftPath, relPaths, msg, opts)` carries no anchorRel and `Add` is said to write to `WeftWorktreePath(l, slug)` directly — both are wrong in a subpath-anchored hub, and pushing the join onto `loomcli` contradicts the same section's "loomcli constructs no `_lyx` path" rule.
**Fix:** Say explicitly where `AnchorRel` is joined for the weft-side write path and for the commit pathspec, and adjust the pinned signature/accessor set accordingly.

### [BLOCKING:consistency] Rollback claim false on the adopted-weft-branch path
**Section:** `origin-record-is-committed-and-is-a-new-class`, Testing (integration)
**Issue:** `add.go:243` deletes the weft branch only when `!weftBranchAdopted`, so on the `weftBranchAlreadyExists` adopt path a failure after the `origin.json` commit (e.g. step 11's warp push) leaves that commit on a preserved pre-existing weft branch — contradicting "the same rollback that unwinds a failed `Add` unwinds the record" and the test "leaves no stray commit".
**Fix:** State the record's disposition for the adopt path — either the commit is skipped/reverted there, or the stray-commit outcome is accepted and the test wording corrected.

### [BLOCKING:decision] CONSTRAINTS.md seam counts and loomcli's RunCLIIn undecided
**Section:** Scope (doc updates), Constraints
**Issue:** The CLI/Cobra Invariant pins "Eleven of the twelve seam modules also carry `RunCLIIn`" and names `selfreportcli` as the sole exception (`seamsignature_test.go:1,29,46` repeats the counts); a 13th module makes both stale, yet the doc-update list omits `CONSTRAINTS.md` and the discussion never says whether `loomcli` carries `RunCLIIn`.
**Fix:** Decide `loomcli`'s `RunCLIIn` disposition and list the `CONSTRAINTS.md` count edit among the same-commit doc updates.

### [NIT:design] SyncOptions on a helper that never pushes
**Section:** `weft-commit-mechanism`
**Issue:** `SyncOptions` is `{SkipGit, SkipPush}` (`fabric.go:94-100`); `CommitWeftPaths` performs no push, so `SkipPush` is inert and `SkipGit`'s effect (return `("", false, nil)` as `commitWeftAt` does?) is unstated.
**Fix:** State the `SkipGit` return contract and whether `SkipPush` is accepted-and-ignored or the parameter narrows to a bool.

### [NIT:decision] `lyx loom drive` on a never-seeded pair
**Section:** `drive-as-real-verb`, Testing (smoke)
**Issue:** Only `run` seeds and commits; a direct `lyx loom drive` on a fresh pair hits Preflight check-4's missing-seed failure (`preflight.go:98`), and the smoke case "`lyx loom drive` standalone… the machine advances" does not say how that pair got seeded.
**Fix:** State whether `drive` refuses with a pointer to `run`, or how the smoke fixture seeds first.

## Verdict

REQUEST_CHANGES
Weft-commit locking, anchor handling, and the rollback claim need resolving before plan writing.
MILL_REVIEW_END
