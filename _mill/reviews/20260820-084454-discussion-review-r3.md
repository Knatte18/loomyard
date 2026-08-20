MILL_REVIEW_BEGIN
# Review: shedengine: per-producer bounce budget + explicit OnDone routing

```yaml
duration_s: 181.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] All-time count burns budget across successful passes
**Section:** § Decisions → "The count is all-time, and a resumed run does not get fresh budget"
**Issue:** The rationale's defence — "after a human fixes the underlying problem the producer returns `Done` and never consults the budget at all" — holds only for an immediate resume; in loom's own shipped list `Discussion-Validate` is re-entered on every `Discussion-Review` bounce (`Discussion-Review` OnStuck → `Discussion-Write`, whose `Done` routes back to `Discussion-Validate`, verified in `internal/loomshed/loomshed.go:126-127`), so its all-time `Stuck` count grows across legitimate, successful review cycles and permanently shrinks its remaining budget with no reset on an intervening `Done`.
**Fix:** State this cross-episode consequence for loom's existing rows explicitly, say whether the default of ten stays adequate under it, and either dispose of the rejected "episode" alternative against *this* case (not only the never-passing-gate case) or add a test scenario pinning bounce→Done→re-entry accumulation.

### [NIT:decision] Escape hatch is not reachable by a loom operator
**Demoted-from:** BLOCKING
**Section:** § Decisions → "The count is all-time…" (escape-hatch paragraph)
**Issue:** "Raise the producer's `MaxBounces` (or `Shed.MaxBounces`)" is not an operator act in loom today — `internal/loomcli/wiring.go:91` leaves `Deps.MaxBounces` zero and there is no flag or config key for it anywhere (`MaxBounces` appears in no CLI or config path), so raising it requires a source edit and rebuild; the second hatch, editing the status file, means hand-deleting `history[]` entries, contradicting `contracts/specs/loom-status-spec.md:44`'s "one entry per producer call" and the append-only property the derivation itself relies on.
**Fix:** Name which of the two hatches is actually supported for loom, and either bring `MaxBounces` onto a real operator surface (flag/config) in scope, or state plainly that a source edit is the intended remedy and that status-file editing breaks the spec's per-call history contract.

### [NIT:scope] roadmap.md missing from the Scope "In" docs bullet
**Section:** § Scope → In (docs bullet, line 37)
**Issue:** The docs bullet lists only `shed.md`, `doc.go`, and `loom.md`, while the doc-falsification inventory additionally commits to moving `manifest/roadmap.md`'s Planned item 1 to Shipped and to rewriting `producer.go:34` and `loomshed.go:50-52` — a plan writer working from Scope alone would miss all three.
**Fix:** Fold the roadmap move and the two Go doc-comment sites into the Scope "In" docs bullet so Scope and the inventory agree.

### [NIT:decision] loom-status-spec silent on history now being load-bearing
**Section:** § Scope → Out ("Any new persisted status-file field… `loom-status-spec.md` is unchanged")
**Issue:** After this task `history[]` is no longer only a log — it is the sole storage of every producer's budget — yet the spec is declared unchanged and states no never-truncate guarantee, so a future compaction task would silently grant fresh budget with nothing to warn it.
**Fix:** Either add a one-line note in the spec that `history[]` is budget-bearing and must never be truncated, or record explicitly why leaving that dependency undocumented is acceptable.

## Verdict

REQUEST_CHANGES
Two blocking gaps in the all-time-budget decision: cross-episode accumulation and an unreachable escape hatch.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
