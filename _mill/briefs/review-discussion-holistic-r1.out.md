MILL_REVIEW_BEGIN
# Review: shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:scope] roadmap.md:54 still names E as final owner
**Section:** Scope / Exact edit sites (`manifest/roadmap.md`) **Issue:** `manifest/roadmap.md:54`'s six-task breakdown still reads "`shed-model-contradiction-sweep` (E — final owner of `shed.md`/`loom.md`/this roadmap item, sweeps the remaining contradictions and adds the `CONSTRAINTS.md` pointer-rule invariant)" — a present-tense pending-owner claim about a task that never ran, and the very claim Testing step 2 greps for at "expect 0 outside `shed-followups.md`". **Fix:** name `roadmap.md:54` as an in-scope edit site with a stated disposition (record E's supersession by this slug), or state explicitly why the breakdown line stays verbatim and exclude it from the grep.

### [BLOCKING:scope] loom.md row 9 still pins Batchifier to webster.yaml
**Section:** Exact edit sites (`manifest/designs/loom.md`) **Issue:** `loom.md:57` reads "`plan.md` (approved) + `webster.yaml`'s `batcher:` key", stale since task F (`CONSTRAINTS.md:352` and `docs/overview.md:282` both pin `batcher.yaml`'s `active:` key); `shed-followups.md:452` assigns exactly this row to E ("rewritten to match whatever task F landed"), but the inventory and the grep set both omit it. **Fix:** add the row to the loom.md edit-site list and to the acceptance greps (`webster.yaml`'s `batcher:` — expect 0 in `manifest/`/`docs/`).

### [BLOCKING:consistency] Acceptance greps contradict declared out-of-scope text
**Section:** Testing, step 2 **Issue:** `producer-slot` expect-0 is falsified by `manifest/roadmap.md:48` ("no built-in concept of Preflight, a producer-slot, or Finalize"), a correct negation on a line this task does not own; "Raddle named as a phase" expect-0 is falsified by `docs/reference/status-schema.md:45`/`:62`/`:92`'s `webster | raddle | finalize` enum, which Scope declares explicitly out. **Fix:** narrow both greps (path/phrase exclusions naming `roadmap.md:48` and `status-schema.md`) so acceptance cannot be met only by editing text the task forbids editing.

### [BLOCKING:design] Type column carries two axes with no stated shape
**Section:** Decisions → shed-md-is-authoritative-loom-md-points / two-axes-cross-reference **Issue:** the table's Type column today holds engine-type values (`mechanical`, `LLM`, `LLM/perch`, `black box`) — the *other* axis — and the discussion never says whether those are replaced by simple/bespoke, augmented per row, or moved to a new column, so the pointer lands in the one place the two axes are most easily conflated. **Fix:** state the concrete column shape (replace / augment / new column) and how the anchor pointer appears without repeating on all twelve rows.

### [BLOCKING:decision] loom.md:82's gate-section hand-off has no disposition
**Section:** Exact edit sites (`loom.md:76–83`) **Issue:** `:82` says the `## The gate` section "still uses 'gate' in the perch sense (sense A) and is unchanged by this task — it remains **task E's territory**"; the discussion inherits E's positions but never says whether the gate section itself needs any change or only the dangling sentence is dropped. **Fix:** state the disposition — verify-only and delete the hand-off sentence, or name the gate-sense edit as in-scope.

### [BLOCKING:scope] E's phase-enum deferral record is not carried forward
**Section:** Scope → Out (`The phase enum`) **Issue:** `shed-followups.md:529–532` gives E two obligations — leave the enum alone *and* "record this deferral explicitly alongside its roadmap edits, so a later reader finds a decision rather than an oversight"; `manifest/roadmap.md` contains no such record today (no match for `phase enum`/`validPhases`), and the discussion inherits only the first half. **Fix:** either scope the roadmap deferral note in, or state that the record is deliberately dropped and why.

## Verdict

REQUEST_CHANGES
Six inherited-residue and acceptance-criterion gaps; the carve-out decisions themselves are sound.
MILL_REVIEW_END
