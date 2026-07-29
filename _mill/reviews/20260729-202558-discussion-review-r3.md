MILL_REVIEW_BEGIN
# Review: fabric: Fabric.Commit classify+dispatch + unified diff/status

```yaml
verdict: GAPS_FOUND
reviewer_model: sonnet
reviewer_self_id: claude-sonnet-5
reviewed_file: /home/knatte/Code/loomyard/wts/fabric-commit-api/_mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] classifier input: raw vs filtered cfg.Dirs()
**Section:** Decisions → classification-input-contract
**Issue:** The Decision text routes the classifier via raw `ScopedPathspec(l.RelPath, cfg.Dirs())`, but the same block's Rationale says it reuses `cfg.Dirs()` "filtered against `hubgeometry.HubReservedNames()` as `junctionNames`/`WiredNames` do" — two different inputs. `internal/fabricengine/junctionnames.go` confirms `junctionNames`/`WiredNames` do apply `filterHubReserved`; a misconfigured fabric.yaml pathspec containing a hub-reserved token (e.g. `_board`) would classify differently depending on which is actually used.
**Fix:** State explicitly whether the classifier takes raw `cfg.Dirs()` or the filtered set, and reconcile the Decision/Rationale wording so they agree.

### [GAP] weft-lock-before-warp-commit vs WEFT_SKIP_GIT unaddressed
**Section:** Decisions → warp-first-ordering / skip-git-weft-scoped
**Issue:** warp-first-ordering states the weft write lock (and its side effects: `ensureWeftLockDir`, git-exclude seeding, a `rev-parse --git-path` git spawn) is acquired before the warp commit whenever the classifier routes any path to weft, with no stated exception for `opts.SkipGit`. Today's `CommitWeft` (`internal/fabricengine/weftgit.go`) checks `opts.SkipGit` and returns *before* ever touching the lock. The discussion never says whether the new pre-warp-commit lock acquisition preserves that early-out or now unconditionally spawns git/creates lock+exclude state even under the CI bypass envs.
**Fix:** State that weft-lock acquisition is itself gated on `!opts.SkipGit`, matching `CommitWeft`'s existing early return, so a `WEFT_SKIP_GIT=1` two-sided commit stays fully offline.

### [NOTE] async-push helper can't literally reuse fabriccli.spawnPush
**Section:** Open items for mill-plan → Async-push child wiring
**Issue:** The item asks to place the new spawn helper in `fabricengine` while "reusing/consolidating with `fabriccli.spawnPush`" — but per the CLI/Cobra Invariant (cli imports engine, never the reverse), `fabricengine` can never import `fabriccli`, so literal code sharing across that boundary is impossible.
**Fix:** Clarify the consolidation means moving/duplicating spawnPush's logic into `fabricengine` (mirroring `boardengine.spawnSync`, confirmed at `internal/boardengine/spawn.go`) and reducing `fabriccli.spawnPush` to a thin caller — not importing across the engine/cli boundary.

### [NOTE] CommitWeft call-site count is off by one
**Section:** Decisions → snapshot-trailer-written-now
**Issue:** States "seven existing 3-arg call sites" but lists `fabriccli/weft_verbs.go` ×3 plus five other single-site files (`buildercli/weft.go`, `webstercli/weft.go`, `fabricengine/syncweft.go`, `initengine/undo.go`, `perchcli/run.go`) = eight; grep against source confirms eight production call sites today, not seven.
**Fix:** Correct "seven" to "eight" — cosmetic, does not change the backward-compatible variadic design.

## Verdict

GAPS_FOUND
Two decisions have unresolved internal contradictions (classifier input, skip-git/lock ordering); rest verified against source.
MILL_REVIEW_END
