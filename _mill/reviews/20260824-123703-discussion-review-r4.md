MILL_REVIEW_BEGIN
# Review: Migrate planparser.Card to Edits/Uses fields

```yaml
duration_s: 197.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (per runtime environment; not independently self-verifiable)
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [NIT:consistency] "14 checks is a miscount" rests on a false premise
**Demoted-from:** BLOCKING
**Section:** `validator-checks`, `stale-comments`
**Issue:** `contracts/specs/loom-plan-spec.md:200-217` is a **14-row** list whose row 1 bundles two IDs (`format-unrecognized` / `plan-unapproved`), so its "fourteen checks below" banner counts rows, not distinct `Check:` IDs — the 15 the discussion's grep proves is a different metric, not a correction; the code comments cite the spec's row count.
**Fix:** State that the figure is a count of distinct `Check:` IDs, and require the rewritten spec's numbered list to carry one row per ID (16 rows, `format-unrecognized` and `plan-unapproved` unbundled) — otherwise a 15-row list under a "16 checks" banner recreates the same apparent discrepancy this task claims to remove.

### [NIT:consistency] `rename-mechanic-section` contradicts the disposition table
**Demoted-from:** BLOCKING
**Section:** `rename-mechanic-section`
**Issue:** It says "five `move-*` checks drop, one returns renamed", but `validator-checks`' table drops three `move-*` checks and renames two (`move-format` → `rename-format`, `move-mechanic-missing` → `rename-mechanic-missing`), and the 15 − 4 dropped arithmetic depends on that table, not on this sentence.
**Fix:** Reword to "three `move-*` checks drop, two survive renamed" so the amendment matches the table and the arithmetic it claims to amend.

### [NIT:consistency] "`Custom` gets no mechanical check at all" is self-contradicted
**Demoted-from:** BLOCKING
**Section:** `path-missing-rework` (and Scope bullet on the design doc's open items)
**Issue:** The same bullet then says `card-path-malformed` still applies to `Custom` targets, and `field-presence`/`validator-checks` bind `card-type-missing`, `card-missing-field` (`Intent:`), `card-field-empty`, `card-field-overlap` and `card-retired-label` to every card including `Custom` — so "no mechanical check at all" is false as written and a plan writer could implement a blanket skip.
**Fix:** Say `Custom` gets no **type-specific** check (no `path-missing`, no target-shape rule) and name the card-generic checks that still bind it.

### [NIT:scope] Stale-"14" inventory is a hand list and undercounts
**Section:** `stale-comments`
**Issue:** The "six sites … all six figures become 16" list omits `internal/planparser/doc.go:58` (covered only incidentally by the package-doc rewrite) and counts `validate_test.go` once where it carries three occurrences (lines 1, 9, 89) — a hand list where the card-format carriers get two documented grep sweeps.
**Fix:** Give the figure sweep its own re-runnable grep, as Sweeps 1 and 2 have, and drop the fixed "six" count.

### [NIT:decision] Design-doc banner disposition is narrower than the banner
**Section:** Scope (In), `manifest/designs/plan-card-format.md`
**Issue:** Only the "Status: designed, not implemented" phrase is dispositioned, but the same banner (`:3`) also asserts "neither is rewritten yet" of the spec and loom stencil — both rewritten here — and instructs "reconcile or delete them when this lands" for two docs Scope (Out) deliberately leaves alone.
**Fix:** State per-clause what the banner becomes, including that the reconcile-or-delete clause is repointed at the two owning roadmap items rather than left as an unactioned instruction.

## Verdict

APPROVE
Check-count premise is wrong and two decision blocks contradict the disposition table.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
