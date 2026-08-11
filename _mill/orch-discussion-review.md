MILL_REVIEW_BEGIN
# Review: format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate

```yaml
verdict: APPROVE
reviewer_model: sonnet
reviewer_self_id: Anthropic Claude, Sonnet-class (session reports model ID claude-sonnet-5)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Scope of this pass

r1's six findings (all NIT, all APPROVE-with-fixes) are resolved in the current text per `_mill/reviews/20260811-041500-discussion-fix-r1.md` — verified directly, not just re-read: `batchifier-row-hands-to-E`, `discussion-review-gate-name-disposition`, the `shed.md:13,41` insertion now in Scope "In", the corrected `loom.md:186`/`:91–92`/`:185` line references, the 8-occurrence `Plan-Review-Gate` count, and the widened `:75` hand-off note are all present and, on independent re-derivation by `grep -n`, exactly correct.

This pass independently re-verified essentially every line-numbered and count-based claim in the document against the live tree (not the manifest's numbers) — `loom.md`'s whole producer table (`:47–60`, every row and cell), `:75`, `:78–83`, `:91–92`, `:185–186`, `:15–17`, `:27,29,39`; `shed.md:13,41,61,63`; `roadmap.md:45–47,68`; the full `Plan-Review-Gate` blast radius (8 hits, 8 lines, 4 files — `loom.md:54,75`, `shed.md:13,41`, `roadmap.md:45,46`, `shed-followups.md:304,306`) and the full `Discussion-Review-Gate` set (9 hits in `shed-followups.md` at exactly `:265,281,283,301,304,325,329,342,424`, plus `roadmap.md:47`); `discussion-format.md`'s and `plan-format.md`'s cited sections; `docs/overview.md:60,93`; `review-finding-classification.md`'s item 5; `CONSTRAINTS.md:368`; the `internal/treadleengine` gate-identifier claim; and commit `80238b3f`'s existence. All of it checks out exactly as stated — every citation in this document is more precise than the manifest it was sourced from, including catching the manifest's own off-by-N errors (`loom.md:188`→`:186`, `:91–94`/`:187`→`:91–92`/`:185`) and correctly cross-verifying `shed-followups.md:445–446`'s citation of those same corrections. This is unusually careful sourcing; only one finding survived the sweep.

## Findings

### [NIT:consistency] `reground-discussion-format-14`'s rationale describes a citation that's already gone
**Section:** Decisions → reground-discussion-format-14
**Issue:** The decision states "`discussion-format.md:14`'s ... rule stays — only its attribution is rewritten ... The current text grounds a live contract in `builder-contract.md`'s 'digest contract' — a doc task A deleted outright." Checked the live file: `discussion-format.md:14` today reads "Two files, not two sections of one file, on purpose: a distilled digest, never raw prose, is what the Plan producer should ever see." — no citation to `builder-contract.md`, no citation to anything. `git log -p -- docs/reference/discussion-format.md` shows task A's own commit (`0149776a`) already rewrote this exact line, from "...this mirrors Builder's 'distilled digest, never raw prose' rule (see [builder-contract.md](builder-contract.md)'s digest contract)." to the current bare sentence — task A stripped the dead citation outright rather than redirecting it, one task earlier than this decision's rationale assumes. There is nothing dangling left at `:14` to "re-ground" in the sense of fixing a broken pointer; the sentence already stands on its own with zero attribution.
**Fix:** Restate the decision as what it actually is now — an *addition* (cite `internal/websterengine`'s doc.go and `docs/overview.md:60` at `:14` to strengthen an already-correct, currently uncited sentence), not a *repair* of a dangling pointer. The two target sources named are still right (verified: `websterengine/doc.go:155` states the distilled-`Digest`-persisted-at-terminal contract, `overview.md:60` carries the exact "Go-distilled digests, never raw prose" phrase) — only the "what's there today" premise needs correcting, so an implementer checking the current file doesn't find their starting assumption already false.

## Verdict

APPROVE.
One NIT: a stale premise in one decision's rationale (the target fix is still correct, the described "current state" it's fixing isn't). Everything else in this heavily-cited document — including all of round 1's fixes — reverified exactly against the live tree.
MILL_REVIEW_END
