MILL_REVIEW_BEGIN
# Review: plan-format v3: flat card list — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-25
```

## Findings

### [LOW] Roadmap Done entry misdescribes v3's actual card schema
**Location:** `manifest/roadmap.md:182-186`
**Issue:** The Done entry says "a card carries `card`/`name`/`description`/`changes-files`/`depends-on` only," but the just-created `docs/reference/plan-format-v3.md` it links to pins a completely different schema (`What:`/`Context:`/`Edits:`/`Creates:`/`Deletes:`/`Moves:`/`Depends-on:`), and explicitly describes `changes-files` as a *derived* union, never a literal per-card field. The entry directly contradicts the authoritative doc it now points at.
**Fix:** Rewrite the entry's field-list clause to match the shipped schema (or drop the field-list detail entirely and just describe the flat-card-list concept), consistent with how the neighbouring `webster`/`builder` Done entries stay accurate.

## Verdict

APPROVE
All batches' cards are realized correctly; links resolve; only one low-severity stale-prose nit found.
MILL_REVIEW_END
