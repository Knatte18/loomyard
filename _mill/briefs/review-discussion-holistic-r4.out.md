MILL_REVIEW_BEGIN
# Review: Surface merge-in-progress in fabric status

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [NIT:consistency] "Every peer carries integration tag" is false
**Section:** Testing → "Build tier"
**Issue:** `internal/fabriccli` has two untagged test files (`argsarity_test.go`, `envelope_test.go`), so "every peer in this package already carries `//go:build integration` on line 1" is wrong, and it contradicts the same section's instruction to run "the untagged tier for the regression pins" and its listing of `argsarity_test.go` as regression coverage.
**Fix:** Restate as "every hub-driving peer is integration-tagged; the two pure cobra-inspection files are untagged" — the tag decision itself is correct on Test Tier Purity grounds either way.

### [NIT:consistency] Doc inventory table is not the literal grep output
**Section:** `docs-in-same-commit` → "How the doc inventory was enumerated"
**Issue:** The table is presented as the full hit list of `grep -rn "fabric status"`, but `SANDBOX-FABRIC-SUITE.md:404` contains no such string (it was found by judgment), while a real hit, `SANDBOX-FABRIC-SUITE.md:142` (F3's Goal line), is absent from the table. No in-scope work is actually missed — F3 and F18 are both listed — but the stated method under-describes how the list was reached.
**Fix:** Note that the grep was supplemented by a read of the merge-relevant sandbox scenarios, and list the F3 Goal line under the F3 row.

### [NIT:consistency] Stale line cite for envelope.go's read-only-verb sentence
**Section:** `read-only-envelope-unchanged`
**Issue:** `envelope.go:16-18` is cited as naming `status` among the four read-only verbs; the sentence is at `envelope.go:14-15` (line 16 is `package fabriccli`). The quoted key-set contract in `key-always-present` is at lines 8-12.
**Fix:** Correct the line range, or cite the file and comment by name rather than by line.

## Verdict

APPROVE
Decisions, scope, failure modes and test tiering are grounded and verified; only cosmetic citation drift remains.
MILL_REVIEW_END
