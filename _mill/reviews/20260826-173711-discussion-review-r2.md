MILL_REVIEW_BEGIN
# Review: loom's status file can conflict on the landing merge

```yaml
duration_s: 126.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Enumeration method misses the "durable" wording class
**Section:** Scope (grep bullet) / Technical context ("not the whole consumer set")
**Issue:** The two stated methods — constructor trace plus full-text grep for the literal `_lyx/loom/status.json` — cannot find text that calls the file durable/tracked without naming the path, yet the discussion hand-fixes exactly two such sites (`shedengine.StatusPath`, `loomshed/seed.go`) with no method behind them; `manifest/designs/fabric-unified-view.md:68` lists `LoomStatusFile` in "the durable, weft-synced, git-tracked `_lyx` group" and appears in neither the grep hit list nor "Docs that make claims this change falsifies", and `internal/loomengine/config.go`'s own comments (lines 88–90, 97–100, 119–120, 129–130, 141–142) all describe `LoomStatusFile` as the durable counterpart.
**Fix:** State a second enumeration pass — a grep over `LoomStatusFile`/`LoomStatusRel` identifiers plus "durable"/"tracked" wording near loom status — and add `manifest/designs/fabric-unified-view.md`'s anchoring table to the falsified-docs list.

### [NIT:scope] loomcli smoke rig commits the status file and is unaccounted for
**Demoted-from:** BLOCKING
**Section:** Testing ("dead-code removal is proven by compilation")
**Issue:** `internal/loomcli/smoke_test.go` is not mentioned anywhere, but its rig depends on the file being tracked: `seedAndCommitStatus` (line 329) and `poisonStatusFile` (line 364) commit `LoomStatusRel()` weft-side, and the named regression guard `TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit` asserts the seed commit's changed-file set is exactly `[LoomStatusRel()]` (line 681) plus a one-commit delta — assertions about the very behaviour this task deletes, not mechanical follow-the-constructor edits.
**Fix:** Add these smoke cases to the test inventory and state their disposition (poison rig writes the untracked file without committing; the cleanliness guard's expected file set becomes the origin record alone).

### [NIT:consistency] "three ephemeral files", four listed
**Section:** Scope, first In bullet
**Issue:** The bullet says "beside the three ephemeral files loom already keeps there" and then names four (`status.json.lock`, `run.lock`, `driver.log`, `bootstrap.lock`).
**Fix:** Say four, or drop the count.

## Verdict

REQUEST_CHANGES
Enumeration method has a named blind spot; loomcli smoke rig's committed-status assertions unscoped.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
