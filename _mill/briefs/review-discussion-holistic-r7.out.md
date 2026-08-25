MILL_REVIEW_BEGIN
# Review: loom: interactive Discussion-Write

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] Multi-candidate precedence is unspecified
**Section:** `one-live-match-or-none` vs `attach-only-a-run-that-never-terminated` / `mechanism-failures-…`
**Issue:** Two matched records can coexist on an ordinary path: `finalize` cleans up only on `OutcomeDone` (`wait.go:405`) and `sweepOrphans` only removes *untracked* strands, so an autonomous `Discussion-Write` that ended `Asking` leaves a tracked, live, idle pane with `Outcome: "asking"`; the operator's resume respawns, and a second `Asking` leaves a second such dir — after which the scan matches two records that are both "live" for the same output-file set. `one-live-match-or-none` says error on more than one live match, while `attach-only-a-run-that-never-terminated` says a non-empty `Outcome` is `found == false`; the discussion never fixes which filter runs first, nor how conflicting per-candidate dispositions (e.g. one untracked-and-young → error, one live-empty-`Outcome` → attach) are combined.
**Fix:** State the evaluation order explicitly — candidates first reduced by the `Outcome`/liveness filters, the multiplicity error applying only to the surviving attachable set — and add a test for the two-leftover-`asking`-dirs case.

### [BLOCKING:decision] No disposition for a run.json with no `Outcome` field
**Section:** `attach-only-a-run-that-never-terminated`
**Issue:** `RunState` (`rundir.go:65-75`) is a plain JSON struct, so every run.json written by the pre-change binary decodes with `Outcome == ""` — the one value that means "attachable". An in-flight worktree that is blocked on an `Asking` run at upgrade time (the same in-flight population the config-migration note already addresses) therefore attaches to an idle pane on the first resume and waits out a fresh 480-minute `discussion_timeout_min`, which is exactly the regression this decision exists to prevent. The same hole exists if `finalize`'s `Outcome` write itself fails, whose error handling (warn vs. fail the run) is also unstated.
**Fix:** Give both cases a stated disposition — e.g. a version/marker field so an `Outcome`-less record is treated as non-attachable, or an explicit "accepted, one-shot, self-clearing" record — and say whether a failed `Outcome` write warns or errors.

### [NIT:scope] No test named for `finalize` writing `Outcome`
**Section:** Testing — `internal/shuttleengine`
**Issue:** The `Attach` test list consumes the persisted `Outcome` extensively, but no listed test asserts `Run.finalize` *writes* it for each terminal outcome (including that the `Done` path's write precedes its `os.RemoveAll` cleanup, `wait.go:405-413`).
**Fix:** Add a `finalize` write test per outcome to the shuttleengine list.

## Verdict

REQUEST_CHANGES
Attach's candidate-precedence order and the `Outcome`-less legacy record need explicit dispositions.
MILL_REVIEW_END
