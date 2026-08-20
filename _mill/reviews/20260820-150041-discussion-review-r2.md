MILL_REVIEW_BEGIN
# Review: Bouncer: the generic review-gate producer

```yaml
duration_s: 195.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: /home/knatte/Code/loomyard/wts/shedadapters-generic-bouncer-producer/_mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Conditional focus file breaks the judge spawn's file contract
**Section:** "Artifact naming and paths" + "Stale outputs"
**Issue:** `shuttleengine` classifies a run `done` only when EVERY `Spec.OutputFiles` entry exists (`internal/shuttleengine/wait.go:222,273-277` — `allOutputFilesExist`), yet the judge call writes `round-<N+1>-focus.md` "only when the verdict is `BLOCKING`"; the discussion never states the judge spawn's `OutputFiles` list, and the obvious three-entry list makes every APPROVED run classify non-`done` and degrade to `Stuck`, so `Done` becomes unreachable.
**Fix:** Pin the judge spawn's `OutputFiles` explicitly and say who writes the next-round focus file — e.g. two output files (verdict + ledger) with the Bouncer itself writing `round-<N+1>-focus.md` after parsing a `BLOCKING` verdict, using the writer already in scope.

### [NIT:consistency] Budget off-by-one is stated backwards
**Demoted-from:** BLOCKING
**Section:** "Budget semantics, and the seed call's off-by-one"
**Issue:** `run.go:197` tests `episodeStuckCount(st.History, def.Name) >= effectiveMaxBounces(...)` against the history read *before* appending the current `Stuck`, so with `MaxBounces: N` the Bouncer's Nth judge call still runs (and can return `Done`) and only its `Stuck` blocks — `N` judged rounds, not `N-1`; at `N=1` the discussion claims zero judged rounds while one full judged round occurs.
**Fix:** Restate the rule as "`MaxBounces: N` yields N judged rounds, the Nth of which blocks the run if it comes back `BLOCKING`", since this number is what the three loom wiring tasks and the constructor doc are told to size against.

### [BLOCKING:design] Seed mode has no already-seeded guard
**Section:** "Three modes, told apart by file existence only"
**Issue:** The seed branch is `N == 0` alone and never consults whether `round-1-focus.md` already exists, so a round producer that bounces back at round 1 without writing a report (pinned as reachable by roadmap.md:17's unconditional-`Stuck` rule) sends the Bouncer straight back into a full seed spawn that archives the prior focus file and pays for another session — the exact failure the re-bounce mode was added to prevent, unhandled at the seed.
**Fix:** Extend the discriminator so an existing `round-1-focus.md` with `N == 0` is a cheap logged re-bounce (no spawn), or state explicitly why re-seeding is preferred there.

### [BLOCKING:design] Re-bounce premise "that ledger provably exists" is false
**Section:** "Cancellation and the output pointer" + third mode
**Issue:** A judge spawn whose agent wrote the verdict but not the ledger classifies non-`done` and degrades, leaving a verdict file on disk with no ledger; the next `Call` then reads verdict-presence as "already judged", never re-judges round N, and reports a pointer to a ledger that does not exist — violating the discussion's own exists-or-empty rule, which matters because `Shed` never stats a pointer.
**Fix:** Make the re-bounce mode stat the ledger (empty pointer when absent) and state the disposition for a verdict-present/ledger-absent or unparseable-verdict round: re-judge, or proceed with the round unjudged.

### [BLOCKING:design] Judge spawn's model/effort input unspecified
**Section:** "Stencils" / constructor surface
**Issue:** `shuttleengine.Spec` carries `Model` and `Effort` (`spec.go:38,46`) and the reference `runJudgeCall` takes both, but the discussion pins only `RunDir`, report-name func, rubric stencil name and `now`; `StencilsDir` appears in prose yet is absent from the constructor-validation test list, and model/effort are never mentioned at all.
**Fix:** Enumerate the constructor's full parameter set and its validation (including `StencilsDir` non-empty/absolute) and state where `Model`/`Effort` come from.

## Verdict

REQUEST_CHANGES
Two file-contract gaps, a wrong budget rule, and an unpinned constructor surface.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 4._
MILL_REVIEW_END
