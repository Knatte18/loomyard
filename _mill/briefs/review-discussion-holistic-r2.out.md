MILL_REVIEW_BEGIN
# Review: Worktree spawn/teardown as Shed producers

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] LoomRun seam split contradicts itself
**Section:** `three-registry-entries-closures-not-engine-imports` vs `loom-run-drives-via-no-attach` / Testing
**Issue:** `Env.RunLoom func(ctx) (string, error)` implies the closure spawns and polls and returns the terminal state, but `loom-run-drives-via-no-attach` puts spawn+poll inside the `lifecycleshed.LoomRun` producer, and Testing demands producer-level "fake status reader" and "fake process runner" seams while the integration test stubs `RunLoom` wholesale to return `Done`.
**Fix:** Decide which layer owns the child spawn and the status poll, and restate the closure signature and the producer's seam set consistently (including which entry reads `Env.TaskWorktreeRoot`, which is currently validated by entries but read by nobody named).

### [BLOCKING:design] Nobody is assigned loom's status path in the task worktree
**Section:** Technical context — "Path derivation" / "Loom status shape"
**Issue:** Reading `_lyx/loom/status.json` plus its `.lyx` lock in the task worktree requires either naming the `_lyx`/`.lyx` literals (Lyxdirs Single-Declarer Invariant) or `loomengine`'s accessors, which take `*lyxcwd.Location` and so pull the import Told-Geometry bars from `lifecycleshed`; and at `lifecyclecli.wire()` time the task worktree does not exist yet, so the paths cannot simply be told up front.
**Fix:** Name the deriving layer and the moment it derives (e.g. `lyxcwd.ResolveWorktree(root)` after `Worktree-Create`, fed through loomengine accessors), and say which of `TaskWorktreeRoot`/status path/status-lock path are told versus derived.

### [BLOCKING:design] No bound or verdict for a loom run that never terminates
**Section:** `loom-run-drives-via-no-attach`; Constraints — Live-Substrate Spawn Observability
**Issue:** "polls loom's own status.json until it reaches a terminal state" states no poll interval, no attempt-count cap (the invariant requires a COUNT cap, not just elapsed time), and no outcome for the never-terminal case that Testing nonetheless asks for; a real loom run spans hours, so this is a design parameter, not an implementation detail.
**Fix:** State the poll interval, the attempt-count cap, and what `LoomRun` returns when the cap is hit (`Stuck` vs hard error) and when `status.json` is absent or unreadable.

### [BLOCKING:design] Resume and contention semantics of `lyx lifecycle run` undecided
**Section:** `lifecycle-cli-two-verbs`; Testing — "Cross-cutting"
**Issue:** The resume test assumes mid-list restart, but the discussion never says what `lyx lifecycle run <slug>` does when a status file already exists (resume / restart / refuse), nor what happens when `run.lock` is already held by another invocation — loom answers both with an explicit bootstrap handshake, and the lifecycle verb has no such story.
**Fix:** Decide and record the pre-existing-status disposition and the held-`run.lock` behaviour (refuse on the envelope vs wait), since both are observable CLI contract.

### [BLOCKING:design] Fabric refusal → Outcome mapping only half decided
**Section:** `create-is-fabric-add-and-nothing-else`, `teardown-escalates-never-forces`, Testing
**Issue:** Only teardown-on-dirtiness is mapped (`Stuck`); Testing defers the rest to "a hard error, per the producer's own contract", which no section states. Unacknowledged refusals verified in source: `Add` fails outright when the driving worktree has uncommitted tracked changes (`internal/fabricengine/add.go:54-60`, so a dirty prime kills every lifecycle run), `Add` fails on a pre-existing warp branch, and `Remove` refuses on `ErrMergeInProgress` (`remove.go:63-71`), not just dirtiness.
**Fix:** State the error→`Done`/`Stuck`/error policy per producer, and say explicitly what a dirty prime or a leftover branch does to `Worktree-Create`.

### [NIT:consistency] Loom status path misstated in Technical context
**Section:** Technical context — "Path derivation"
**Issue:** It claims `loomengine/config.go` derives loom's status paths under `lyxdirs.DotLyxDirName`; `LoomStatusFile` is under `LyxDirName` (`_lyx/loom/status.json`) and only the locks/driver log are under `.lyx` — the `_lyx` form elsewhere in the doc is the correct one.
**Fix:** Correct the sentence so a plan writer copying it does not poll a nonexistent `.lyx/loom/status.json`.

### [NIT:decision] `AbandonedSession` surfacing mechanism unstated
**Section:** Technical context — "Reed APIs"
**Issue:** "should surface that field rather than swallow it" names no channel — log line, `OutputPointer`, or CLI envelope key (`reedcli/up.go` uses an `abandonedSession` payload key).
**Fix:** Name where the teardown producer puts it.

### [NIT:scope] New invariant has no stated enforcement
**Section:** Constraints — "Lifecycle Bookend Invariant"
**Issue:** The invariant is recorded but the discussion never says whether it is review-discipline only or gets an enforcing test in the same commit.
**Fix:** State which, in the invariant's own wording.

## Verdict

REQUEST_CHANGES
Five load-bearing decisions — seam split, status-path ownership, poll bound, resume, refusal mapping — remain open.
MILL_REVIEW_END
