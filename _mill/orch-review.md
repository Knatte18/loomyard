# Orchestrator review — discussion.md

Reviewed against `main` (`dce9fde8`, unchanged in this worktree — only `_mill/discussion.md`/`status.md` exist so far).

## Citation check

Verified every concrete file/line/quote claim in Scope, Technical context, and Testing.

| Claim | Status |
|---|---|
| `producer.go` 44 lines, fields today `Name`/`Producer`/`OnStuck` | Correct |
| `run.go` 320 lines, six-step loop, five-arm switch | Correct |
| `bouncesRemaining` init/decrement/check lines (69–71, 201, 216) | Correct |
| `indexAfter` sits just above `persist`, called from exactly one site (line 240) | Correct |
| `def.Name == s.Producers[len(s.Producers)-1].Name` (line 225) | Correct |
| `validate.go` 67 lines, flat checks → seen-set loop → `OnStuck` loop | Correct |
| `shed.go` 54 lines, `MaxBounces` doc "total ... for one Run call" | Correct |
| `doc.go` 54 lines | Correct |
| `loomshed.go` 12-row order Preflight→...→Finalize | Correct, exact match |
| `loomshed.go`'s `New` doc comment: "The twelve rows, with their backing and OnStuck target" (line 85) | Correct |
| `loomshed_test.go:99` asserts `shed.MaxBounces` | Correct |
| `fixture_test.go:118` sets `MaxBounces: 3` | Correct |
| `internal/loomcli/wiring.go:91` "MaxBounces is left zero..." comment | Correct |
| `CONSTRAINTS.md` Shed Producer-Seam Invariant / Told-Geometry Invariant sections exist | Correct |
| `seam_enforcement_test.go` test names (`TestProducerSeamInvariant_AllowlistOnly`, `TestToldGeometryInvariant_AllowlistOnly`) | Correct |
| `HistoryEntry{Producer, Outcome, Output, At}`, JSON tags `producer`/`outcome`/`output`/`at` | Correct |
| `manifest/designs/loom.md:45` sequential-bounce sentence | Correct, exact line |
| `shed.md`'s "single total cap across the whole run, not per-producer" paragraph, arguing per-producer would let an A↔B cycle run 2×budget | Correct, exact quote and argument |
| `shed.md` step 6 `Done` bullet ("advance current_producer to the next entry... Past the last entry") | Correct |
| `contracts/specs/loom-status-spec.md` "one entry per producer call" / fresh-start-tolerates-leftover-Preflight-entry mechanism | Correct, matches exactly including the `OnStuck: ""` mechanism cited as the reason |

One inexact citation: `internal/loomshed/resume_test.go:267-301` — the actual function (`TestBounceRouting_BudgetExhaustionBlocks`) spans lines 269–303, not 267–301. Off by two at both ends. Cosmetic — does not change which scenario the discussion is pointing at, and the test itself matches the described assertions (`MaxBounces+1` Stuck entries, then `RunBlocked`) exactly.

## Design read

**`OnDone` with no fallback, `Segment` as a pure grouping label, all-time per-producer budget derived from `history[]`** — internally consistent, and each decision's Rejected list engages with the real alternative rather than a straw one. Two points worth flagging, neither blocking:

1. **The all-time-budget inversion is the single biggest behavior change in this task**, and it is the correct call given the stated goal (bound total wasted spend, not spend-per-invocation) — but it is worth the implementer double-checking that `shed.md`'s prose is rewritten strongly enough that a future reader doesn't reintroduce a per-invocation reset by "fixing" what looks like a bug. The discussion already commits to rewriting both stale rationale sites; make sure the new prose states the inversion explicitly rather than just describing the new behavior in isolation.
2. **The pre-append-vs-post-append ordering gotcha** (§"Ordering gotcha in the Stuck arm") is called out correctly and precisely — `st.History` (pre-append) is genuinely what pins the boundary, and `nextHistory` would shift it by one. This is exactly the kind of off-by-one that survives code review if the test doesn't specifically target it; good that `TestRun_BounceBudgetExhaustion`'s boundary case is already named as the guard.

No open decision looks wrong or underspecified. The `Segment` same-`OnStuck` rule's "existing rows keep `Segment: ""` and pass as one implicit group" is the right minimal migration — it doesn't force a design pass over loom's producer table onto this task, correctly deferred to the review-producer tasks per the Rejected note.

## Verdict

Sound. Nothing here should block moving to Plan. Fix the `resume_test.go` line-range citation if convenient; not worth a discussion round on its own.
