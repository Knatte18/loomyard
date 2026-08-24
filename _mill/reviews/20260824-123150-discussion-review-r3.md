MILL_REVIEW_BEGIN
# Review: Migrate planparser.Card to Edits/Uses fields

```yaml
duration_s: 174.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [BLOCKING:design] Retired labels are silently dropped, not flagged
**Section:** `card-grammar` / `field-presence` / `validator-checks`
**Issue:** `parseCardBody`'s fallthrough is `default: i++` (`parse.go:388-389`) and `isCardLabelLine` only knows `cardLabels`, so once `**Context:**`/`**Edits:**`/`**Creates:**`/`**Deletes:**`/`**Moves:**`/`**Depends-on:**`/`**What:**`/`**verify:**` leave the recognized set, a partially-migrated format-4 card carrying one is silently ignored — or swallowed into `Intent:`'s collect-until-next-label prose — with no parse error and no finding; the discussion gives these labels a *mapping* disposition but never a *parser* disposition. The same gap makes `impact-summary-multiline` unimplementable as stated, since the trailing lines it must report are exactly what `default: i++` discards. A card stripped of all fields is caught by `card-type-missing`, but a half-migrated one is not, contradicting the discussion's own stated rule ("a loud validation finding, never a silent misparse").
**Fix:** State the disposition of an unrecognized/retired bold label in a format-4 card body — hard `ParsePlan` error, a new `card-unknown-label` finding, or explicitly-accepted silent drop — and say what the parser retains so `impact-summary-multiline` has lines to report.

### [NIT:consistency] Fifth stale "14 checks" site left uncorrected
**Section:** Scope / `stale-comments`
**Issue:** The discussion enumerates the stale-figure sites as three Go comments plus `loom-plan-spec.md`'s banner (plus `plan-card-format.md:84`), but `manifest/designs/scout-plan-symbol-fields.md:64` carries "`loom-plan-spec.md`'s existing 14 checks" too and is placed out of scope by a rationale ("already assigned elsewhere by the roadmap") that only cites `roadmap.md:62` for `webster-parallel-execution.md`; the roadmap's `scout-backed plan symbol fields` item (`roadmap.md:136-138`) merely references the doc without assigning a reconcile.
**Fix:** Either include that line in the count correction or state explicitly that it is knowingly left stale and why the `plan-card-format.md:84` rationale does not apply.

### [NIT:design] `Custom` card's path-missing false positive unremarked
**Section:** `path-missing-rework`
**Issue:** `Custom` targets are path-checked, so a `Custom` card used as the design doc's "explicit escape hatch" to create something produces a spurious `path-missing` finding; the analogous `Move` false positive is documented as accepted, this one is not mentioned at all.
**Fix:** Add one sentence accepting (or excluding) `Custom` targets under `path-missing`, alongside the `Move` limitation already recorded.

## Verdict

REQUEST_CHANGES
Retired card labels have no parser disposition; silent-drop contradicts the stated fail-loud rule.
MILL_REVIEW_END
