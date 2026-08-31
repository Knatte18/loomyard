MILL_REVIEW_BEGIN
# Review: Prefer raw fetch, scope large tree listings — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (self-assessed; harness reports "claude-sonnet-5")
reviewed_file: plan/
date: 2026-08-31
```

## Findings

### [BLOCKING:decision] Card 16's symlink caveat has no disposition for "unresolved"
**Location:** batch 3 / card 16, cross-referencing batch 2 / card 8
**Issue:** Card 8 explicitly allows a third capture outcome for the raw-host symlink probe — "record that the question is unresolved" — and states this is "the disposition batch 3's conditional sentence reads." Card 16's conditional only branches on two outcomes ("recorded a 200 and target path" → add sentence; "recorded a non-2xx instead" → omit), leaving no stated instruction for what to write when CAPTURE.md says the question went unresolved (no symlink was reachable to test at all).
**Fix:** Add an explicit third branch to card 16's instruction — e.g. "if the capture recorded the question as unresolved, omit the sentence, same as the non-2xx case" — so the disposition of all three CAPTURE.md outcomes is stated, not just two.

### [NIT:consistency] Card 7's final scenario reuses a "-rec"-suffixed fixture for a non-recursive endpoint
**Location:** batch 1 / card 7 (the combined `--children` + `--max-entries` scenario)
**Issue:** The scenario maps "the scoped non-recursive endpoint" (i.e. `HEAD:<path>` with no `?recursive=1`) using `scoped-src-rec.json` from Context — the only fixture card 7 is given for it — even though the harness's naming convention (`-rec` vs `-nonrec`, e.g. `children-src-nonrec.json` from card 4/5) otherwise ties the suffix to which endpoint shape a fixture represents. It works functionally (the jq parse is endpoint-agnostic) but breaks that naming convention's implied meaning.
**Fix:** Either add `children-src-nonrec.json` to card 7's Context and have it reuse that instead, or note in card 7's Requirements that the recursive-named fixture is being deliberately reused here since only "a successful listing" is asserted, not the trailing-slash shape.

## Verdict

REQUEST_CHANGES
Card 16 leaves the "unresolved" symlink-capture outcome without a stated documentation disposition.
MILL_REVIEW_END
