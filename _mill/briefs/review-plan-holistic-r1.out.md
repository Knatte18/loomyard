MILL_REVIEW_BEGIN
# Review: plan-format v3: flat card list — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-25
```

## Findings

### [MEDIUM] Repointed links name relocated content, not its new home
**Location:** batch 1 / card 3 (codeintel-redesign.md links 25, 139, 150 + prose 18)
**Issue:** Card 2 relocates Mechanism 1/2, the machine-mismatch resolution, and the mechanical DAG-derivation *out* of the design doc into `webster-rewrite.md`, but card 3 repoints those inbound links to `docs/reference/plan-format-v3.md` — which card 1 explicitly excludes that detail from (pointer only). So "plan-internal name matching (see plan-format-v3.md)" (139), "mechanical DAG-derivation ... see plan-format-v3.md" (25), and symbol-verification (150) will resolve to a doc lacking that content.
**Fix:** For links/prose that name the relocated mechanism, point at `../designs/webster-rewrite.md` (its new home), not the reference doc.

### [MEDIUM] Bare prose left as a now-false claim
**Location:** batch 1 / card 3 (codeintel-redesign.md line 18)
**Issue:** Card 3 leaves "see plan-format-v3.md's resolution of this exact machine-mismatch problem" unchanged on the rationale "the basename is still plan-format-v3.md so the prose stays accurate" — but that resolution section is moved to `webster-rewrite.md` by card 2, so the reference doc named plan-format-v3.md no longer contains it; the sentence becomes false.
**Fix:** Repoint this prose to `webster-rewrite.md`, or drop the "plan-format-v3.md's resolution" claim.

### [NIT] Stale line number for model-spec.md status header
**Location:** batch 2 / card 6
**Issue:** The card cites the model-spec.md Status clause as "currently line 7"; it actually spans lines 5–6.
**Fix:** Correct to lines 5–6 (the quoted text is accurate and findable regardless).

## Verdict

REQUEST_CHANGES
Cross-references to card-2-relocated content misdirect to the reference doc; otherwise sound.
MILL_REVIEW_END
