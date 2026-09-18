MILL_REVIEW_BEGIN
# Review: Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (self-assessment; harness reports "Opus 5")
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [NIT:consistency] helptree assertion is substring, not set-equality
**Section:** Technical context → "The root alias"; Testing → `cmd/lyx/helptree_test.go` **Issue:** The discussion says line 114 "asserts loom's subcommand set is exactly `{run, drive, step, ...}`" and calls the two assertions "the primary structural guard"; `cmd/lyx/helptree_test.go:133-137` is a per-item `strings.Contains` over help text, so it can never catch a surviving `drive` command. **Fix:** Drop "exactly" and state that the new tier-1 retired-name cobra walk — not helptree — is what proves `drive` is gone; the TDD ordering claim itself holds, since neither "start" nor a `Short`/`Long` containing it exists on today's tree.

### [NIT:scope] "Third gotcha" reads as a closed list of two sites
**Section:** Technical context → "Third gotcha: prose whose *meaning* turns on the two verbs being different" **Issue:** "Two sites must be rewritten as sentences" carries no landmarks-not-boundary caveat, unlike every other list in the document, yet at least two more sentences name both verbs in a contrast a token swap collapses — `internal/shuttleengine/attach.go:415-421` ("recreated in-band by \"lyx reed up\", or simply by \"lyx loom run\" and \"lyx loom drive\"") and `internal/loomengine/seedownership_test.go:77-78`. **Fix:** Apply the Scope section's own "landmarks, never the boundary" wording here so the classify step, not this list, bounds the meaning-inversion class.

## Verdict

APPROVE
Decisions complete, scope stated as a rule, testing concrete; two wording NITs only.
MILL_REVIEW_END
