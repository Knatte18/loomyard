MILL_REVIEW_BEGIN
# Review: loom: phase-machine scaffolding

```yaml
duration_s: 152.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:design] loomshed's constructor input surface is undefined
**Section:** `explicit-deps-struct` vs `batchifier-is-a-gate` / Technical context
**Issue:** `explicit-deps-struct` says every real producer "arrives as an already-constructed `shedengine.ShedProducer`", but `batchifier-is-a-gate` has `loomshed` itself own a Webster wrapper that constructs `NewWebsterProducer` inside `Call`, and `Discussion-Validate`/`Plan-Validate`/`Batchifier` are new code in `loomshed` — so which rows are injected, and which told values the constructor takes (cwd for `Preflight(cwd)`, `anchorPath` for `planparser.PlanDir`, `worktreeRoot` for `planparser.Validate(plan, worktreeRoot)` — two distinct values, `baseDir` for `batcher.Active`, the two discussion file paths, since `loomengine.DiscussionDecisionRecord`/`DiscussionSupportLog` take a `*lyxcwd.Location` `loomshed` may not import) is never stated.
**Fix:** Pin the deps struct explicitly: which of the 12 rows arrive pre-constructed, what seam the Webster row is injected through (a `shedadapters.WebsterRunner`/`RunDeps` pair, not a `ShedProducer`, if the wrapper is `loomshed`-owned), and the full told-path field list.

### [BLOCKING:design] Rewritten coherence check leaves `state`/`error` undefined on resume
**Section:** `rewrite-loom-status-in-place`
**Issue:** The fresh-start narrowing is specified over `history` only ("`start_sha` and `pause_requested` keep their existing treatment"), but a `Preflight` that returned `Stuck` with `OnStuck: ""` persists `state: "blocked"` and `error: "stuck with no OnStuck target"` (verified `run.go:187-200`, `status.go`); the resumed re-run then re-enters check 4 against exactly that file, and "validate the Shed shell" never says whether a non-`running` `state` or non-empty `error` is coherent.
**Fix:** State the disposition of `state`, `error` and `activity` in the rewritten check — presumably `state ∈ {running, blocked}` and any `error` tolerated — or the same permanent deadlock the history narrowing was written to remove returns through a different field.

### [NIT:scope] Same-commit doc set omits two docs the change falsifies
**Demoted-from:** BLOCKING
**Section:** Scope / Constraints → Documentation Lifecycle
**Issue:** `manifest/designs/shed.md:241,251` carries the same statements as `internal/shedengine/doc.go` ("reconciling the two is loom's own later rewiring task"; "`loom-status-spec.md` mandates `phase`, `stage`, and `narration` as top-level fields"), and `manifest/designs/loom.md:170` calls the status file the source of truth for "current phase, current review stage" — both go false, and neither is in the enumerated doc set. `loom.md`'s row-10 **Input** cell ("batch grouping") is falsified by the same reasoning that corrects row 9's Output, and is likewise unlisted.
**Fix:** Extend the doc set to `manifest/designs/shed.md`, `loom.md`'s State-&-contracts bullet and row-10 Input, and state the enumeration method (grep over `loom/status.json` / `loom-status-spec` references) so the list is reproducible rather than incidental.

## Verdict

REQUEST_CHANGES
Three gaps: loomshed's input surface, the rewritten check's non-history fields, and the doc set.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
