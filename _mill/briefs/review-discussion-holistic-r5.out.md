MILL_REVIEW_BEGIN
# Review: Worktree spawn/teardown as Shed producers

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:consistency] `blocked` lifecycle status has no disposition
**Section:** `lifecycle-run-resumes-and-refuses-concurrency` vs `loom-run-poll-bound` / `refusal-to-outcome-mapping`
**Issue:** The three dispositions split on "non-terminal state" vs `done`, but `loom-run-poll-bound` uses "terminal" to include `blocked`/`paused`, so a lifecycle status left at `blocked` — the everyday outcome of every `Stuck` in `refusal-to-outcome-mapping`, whose rationale promises "re-running the verb resumes from the same row" — matches neither branch. `shedengine.Run` itself does resume from `StateBlocked`/`StateFailed` (`internal/shedengine/run_pause_test.go:264-303`), so the CLI wording is the only thing ambiguous, and it is the dominant recovery path.
**Fix:** State the disposition per `shedengine.State` value explicitly — `running`/`blocked`/`failed`/`paused` resume, `done` refuses — rather than via the word "terminal".

### [BLOCKING:design] Cross-slug concurrency from prime undecided
**Section:** `lifecycle-run-resumes-and-refuses-concurrency`, `lifecycle-driver-runs-from-prime`
**Issue:** `run.lock` is per-slug (`<prime>/.lyx/lifecycle/<slug>/run.lock`), so two `lyx lifecycle run` invocations for different slugs are permitted to drive `Topology.Add`/`Remove` against the same prime repository concurrently, for hours each; `Add` takes no lock of its own (`internal/fabricengine/add.go:45-60`) and its dirty-prime probe reads prime's working tree while the other run may be mutating it. Nothing in scope says whether this is supported, refused, or merely tolerated.
**Fix:** State the disposition — a prime-wide lock, an explicit refusal, or an explicit "concurrent slugs are the operator's risk" with the race named.

### [NIT:consistency] `Spawn`'s child command is unspecified
**Section:** Technical context, "Process spawning"
**Issue:** `LoomRunDeps.Spawn` must run `lyx loom run --no-attach` and **wait**, but the section points at `internal/proc.Detach` and the two detached spawn sites as the model, and never says how the `lyx` binary is resolved (`os.Executable()` is the in-tree precedent, and it is the thing `gitkit/reexecguard.go` exists to catch).
**Fix:** Say the child is waited on, not detached, and name the executable-resolution mechanism.

### [NIT:consistency] Dirty-prime test asserts remedy wording that does not exist
**Section:** Testing, `internal/lifecycleshed`
**Issue:** The test requires both named `WorktreeCreate` refusals to assert "the reason contains fabric's own remedy wording"; the branch-exists error does name remedies (`add.go:73-76`), but the dirty-prime error is bare `"source worktree has uncommitted changes"` (`add.go:59`) with no remedy to assert.
**Fix:** Scope the remedy-wording assertion to the branch-exists case and assert verbatim pass-through for the dirty-prime case.

### [NIT:design] Teardown seams' resolution timing not pinned
**Section:** `loom-status-paths-derived-lazily-by-lifecyclecli`, `teardown-is-one-row-sequencing-internally`
**Issue:** Laziness is mandated by name only for `LoomRunDeps.ResolveStatus`; `TeardownDeps.Shutdown`/`Remove` also need `lyxcwd.ResolveWorktree` + `hubgeom.ReedGeometry` against a worktree that does not exist at `wire()` time, and the singling-out invites resolving it eagerly in `wire()`.
**Fix:** Say the teardown closures resolve the task worktree's `Location` inside the closure body, for the same reason.

## Verdict

REQUEST_CHANGES
Two gaps: `blocked`-status disposition and cross-slug concurrency from prime.
MILL_REVIEW_END
