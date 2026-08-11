MILL_REVIEW_BEGIN
# Review: fabric: accumulate the result envelope from mutations, not control flow (slice 14)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:design] Oracle cannot observe five of the thirteen Kinds
**Section:** `permitted-roots-and-the-oracle` / `fabrictest-truthfulness-oracle`
**Issue:** `CaptureManifest` records only `.git` itself and `.git/worktrees/<name>` at existence granularity and states "Branch existence is deliberately not carried by the manifest at all" (`fabrictest/manifest.go:109-118,139-141,184-213`), so `branch_created`, `branch_deleted`, `branch_pushed`, `commit_created`, and a `file_written` to `.git/info/exclude` can never have "a matching unfiltered-diff change" — the stated reverse direction fails on every cell that creates a branch or commits.
**Fix:** state which Kinds are exempt from the diff-backed cross-check and what oracle (git itself, as the existing per-verb effect assertions already do) covers them instead.

### [BLOCKING:design] Cross-check matching granularity undefined for subtrees
**Section:** `permitted-roots-and-the-oracle`
**Issue:** the check is specified as a "direct string comparison" against `DiffManifest` keys, but one `worktree_created`/`worktree_removed`/`path_removed` entry names a single root while the unfiltered diff emits one `Change` per path beneath it — so a normal `Add` produces dozens of diff entries with no matching record entry (a false "lie of omission").
**Fix:** define matching as segment-wise subtree containment against the record's targets, not path equality, and say so explicitly in both directions.

### [BLOCKING:design] `repointLink` double-records and cannot fill its `Detail`
**Section:** `mutation-entry-shape` / `gate-auto-records`
**Issue:** `repointLink` (`destroy.go:668-678`) executes by calling `removeLink`, so threading `rec` into all eight sites yields both `link_repointed` and `link_removed` for one act; and the new target exists only at the caller's `fslink.CreateDirLink` (`junction.go:169,319`), outside the gate, so `Detail` "carries the new target" is unfillable where the discussion places the recording site.
**Fix:** decide whether repoint records once (and where) and whether the caller's re-create is a hand-recorded `link_created`.

### [BLOCKING:design] Record timing unstated — auto-recording can lie by commission
**Section:** `gate-auto-records`
**Issue:** the discussion never says whether the gate records before or only after an observed effect; `removePath` returns nil early when the target is already absent (`destroy.go:615-618`), and `removeGitWorktree`/`deleteBranch` return a nonzero `exitCode` with a nil `err` (`:640-653,:683-690`), so an unconditional record produces entries the manifest diff will not corroborate.
**Fix:** state the recording rule — append only after the primitive observably changed state, and define the nonzero-exit and already-absent cases.

### [BLOCKING:design] `sync`'s push record does not exist in-process
**Section:** Technical context, "`push`/`sync` composition, spelled out"
**Issue:** `sync` is not "commit + push": `weft_verbs.go:257-265` calls `fab.Commit` then `spawnPush` → `fabricengine.SpawnDetachedPush` (`fabriccli/spawn.go:13-15`), a detached child, so there is no push record to concatenate; separately the `push` verb itself runs `fab.Commit` before `fab.PushWeft` (`:188-192`), so it needs the same concatenation rule the discussion reserves for `sync`.
**Fix:** restate the composition against the real call graph — what `sync` can record (commit plus a `push_spawned`-style entry or nothing) and that `push`'s envelope is also two concatenated records.

### [NIT:consistency] `Remove`'s dirty refusal is not a `*destructiveRefusal`
**Demoted-from:** BLOCKING
**Section:** Testing, first failure-path scenario / `remove-ordering-anomaly`
**Issue:** the headline cell is asserted to produce "the refusal object must name the dirtiness check", but `remove.go:68-76` returns a bare `fmt.Errorf("worktree has uncommitted changes; use --force")` from `worktreeDirty`, never a gate refusal, so `errors.As` finds nothing and `refusal` is absent.
**Fix:** correct the scenario to assert `mutations` + `partial: true` with no `refusal` object, or decide (and say) that this pre-flight is converted to a gate refusal in this slice.

## Verdict

REQUEST_CHANGES
Oracle cross-check, gate recording semantics, and the sync/refusal premises need resolution before planning.
MILL_REVIEW_END
