MILL_REVIEW_BEGIN
# Review: reed: per-hub daemon reaps orphaned sessions — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-19
```

## Findings

No findings. The implementation was checked against all five batches and the shared decisions, with source verified directly rather than assumed:

- Batch 1 (`internal/reedengine/lock.go`, `overlay.go`, `overlay_test.go`): `ShellPath()` matches its `TmuxPath()` sibling exactly; `ReapSession`'s four-step ordering, throwaway `Engine{cfg: Config{Shell: shellPath}}` construction (no `Geometry` literal, per the Told-Geometry Invariant), logging keys (`socket`/`session`/`pids`), and unwrapped `killErr` return all match the card verbatim. `reapSessionPanes`/`reapSessionKill` and their `execHook`-driven tests (`TestReapSessionKill`, `TestReapSessionPanes`, `TestReapSession_CallOrdering`) cover exactly what the card specifies, including the list-panes-failure-still-reaches-kill-session case. `ListSessions`'s doc comment was reworded away from "the one engine-less" as required.
- Batch 2 (`internal/reedcli/watchdog.go` seams + `watchdog_test.go`): `watchdogOrphanGoneCycles`, `watchdogTiming`/`watchdogDefaultTiming`, `validateWatchdogFlags` (no shell parameter, verbatim error strings), `worktreeRootGone`/`hubIsLiveDir` (two independent predicates, not negations, both conservative on EACCES — confirmed via the skip-guarded uid-0/Windows test), and `planReapCycle` (hub-probe short-circuit with in-flight exclusion, counter prune/advance/delete semantics) are all implemented and table-tested exactly to the card's per-case list, including counter-map hygiene and the several-sessions-independent-progress case.
- Batch 3 (`dispatchReap`, `runWatchdogLoop` reap pass, `--shell` flag, spawn site, call-site updates): the non-blocking `select{case done<-name: default:}` send, loop-owned `inFlight`/`goneCounters`/`reapDone` with no mutex, cancel-before-dispatch ordering, `wg.Wait()` after cancelling every `known` entry, and `remaining` (not `live`) feeding `planSessionDiff` all match. `validateWatchdogFlags` is wired ahead of every side effect in `RunE`; `--shell` is optional and passed through unconditionally from `ensureWatchdogSpawned`. Grepping `runWatchdogLoop(` confirms exactly the five batch-3 call sites (declaration + `RunE` + four integration-test sites) plus the one new batch-4 site — card 14's zero-diff gate is satisfiable as written.
- Batch 4 (`watchdogreap_integration_test.go`): the three named helpers (`newReapFixture`, `compressedReapTiming`, `orphanWorktree`) have exactly the shapes and reuse (`watchdogIntegrationEngine`, `waitForCondition`) the card mandates. All five `TestWatchdogReap*` functions match the batch's `-run` filter, cover end-to-end reap + sibling survival, never-entered orphan, empty-shell root-only assertion (with a correct platform-scoped rationale), the descendant-closure process half via a `setsid`-detached script, and off-loop loop-liveness under `-race` with in-flight-set re-entry proven via a same-name re-orphan.
- Batch 5 (`manifest/designs/reed-header-selvage.md`, `manifest/roadmap.md`): the design doc's Watchdog section states every required rule (only-proven-gone, hub-probed-first, 3-cycle confirmation, closure-before-kill, off-loop with in-flight exclusion, `ReapSession` is the second engine-less function, `--shell` degrade-not-refuse) and the stale forward-reference bullet under `## Related` was removed while the sibling bullet stayed. `roadmap.md` shows the item exactly once, moved to the top of `## Done` in past tense with the design-doc link retained, and confirmed absent from `## Planned` (single grep hit total).

No out-of-plan files, no duplicated helpers across batches, no constraint violations (Told-Geometry, Live-Substrate Spawn Observability, Test Tier Purity, hubforge Fabric-Fixture) were found on inspection of the actual diffs.

## Verdict

APPROVE
Every batch's cards, shared decisions, and cross-batch contracts are implemented and tested exactly as planned.
MILL_REVIEW_END
