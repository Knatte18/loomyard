MILL_REVIEW_BEGIN
# Review: shedengine: per-producer bounce budget + explicit OnDone routing

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus (session-reported id claude-opus-5); exact build not verifiable from inside the session
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Never-`Done` producer has no reachable escape hatch
**Section:** § "Escape hatches when a budget is exhausted" + § "The count is episode-scoped"
**Issue:** The primary remedy — "fix the underlying failure so the producer returns `Done`, which resets its episode" — is structurally unavailable to a producer that never returns `Done`, and the task's own "Why now" (line 20) defines the Burler as exactly that ("always returns `Stuck` and never `Done`"); for such a row the budget is a task-lifetime cap and the only remaining hatch is the source-edit-and-rebuild one the same decision calls unreachable for operators.
**Fix:** State the disposition for the never-`Done` shape explicitly — either accept it and say so in `shed.md` (a Burler's `MaxBounces` must be sized as a task-lifetime cap, not a per-review one), or name the alternative reset rule considered and rejected.

### [BLOCKING:design] Omitted `OnDone` is silently terminal, with no stated disposition
**Section:** § "OnDone replaces sequential routing entirely" + § Scope (test re-wiring) + § Testing
**Issue:** With no fallback, a forgotten `OnDone` is indistinguishable from an intended terminal and ends the run silently; the discussion acknowledges this only inside a `loomshed` test suggestion, while Scope names `go test ./...` as "the authority" for which other tests need re-wiring — but a suite that asserts only `RunDone`/`state: "done"` (e.g. the single-row and end-state-only shapes in `run_persist_test.go`) passes unchanged on a shortened run, so a green suite is not evidence of a complete migration.
**Fix:** Record a decision on the silent-terminal risk (accept with the exhaustive `OnDone` assertion as the named mitigation, or add a `validate()` terminal rule), and replace "`go test ./...` is the authority" with a mechanical enumeration (`[]ProducerDef` literals / `shed.Producers =` assignments) as the completeness check.

### [NIT:consistency] `ProducerDef` will have six fields, not five
**Section:** § Technical context, "Docs carrying statements this task falsifies"
**Issue:** The bullet calls `producer.go:34`'s struct comment "arithmetically false at five fields", but the struct has three fields today and gains three (`OnDone`, `Segment`, `MaxBounces`) — six.
**Fix:** Correct the count so the rewritten comment is not written against a wrong number.

### [NIT:scope] `findProducer`'s index return becomes dead
**Section:** § Scope, `internal/shedengine/run.go`
**Issue:** `indexAfter` is dispositioned for deletion, but `findProducer`'s `int` return (`run.go:22`) is already discarded at its only call site (`run.go:108`) and becomes wholly unused once `Done` routes by name; no disposition is given.
**Fix:** Say whether the index return is dropped with `indexAfter` or deliberately kept.

### [NIT:consistency] Doc sweep root misses `manifest/parallel-work.md`
**Section:** § Technical context, grep-sweep roots
**Issue:** The sweep covers `internal/**/*.go`, `manifest/designs/*.md`, `contracts/specs/*.md`; `manifest/parallel-work.md:17` states this task is "`internal/shedengine` only", falsified by the mandatory `loomshed` migration, and falls outside every root.
**Fix:** Either widen the sweep to `manifest/*.md` or note that `parallel-work.md` self-declares as a regenerable snapshot and is deliberately excluded.

## Verdict

REQUEST_CHANGES
Two design gaps: never-`Done` producers have no hatch, and silent-terminal `OnDone` is undisposed.
MILL_REVIEW_END
