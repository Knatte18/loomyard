MILL_REVIEW_BEGIN
# Review: Audit the remaining leaf and seam import invariants

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Retarget of shuttleengine:22 names a non-origin file
**Section:** Decisions → "lyxtest's test is renamed …", Consequence bullet
**Issue:** The decision says the citation "retargets to `internal/pattern/leaf_enforcement_test.go`" as "the actual origin", but `internal/pattern/leaf_enforcement_test.go:4` itself reads "Like modelspec's and tokenvocab's leaf_enforcement_test.go" and `internal/modelspec/leaf_enforcement_test.go:4` reads "Unlike lyxtest's …" — pattern is downstream of modelspec, which is downstream of lyxtest, so the new comment would assert something the tree contradicts.
**Fix:** Decide the target on evidence — either keep citing lyxtest with the new function name (the `go/parser` `ImportsOnly` idiom does survive the conversion, as the discussion itself states at "that idiom survives the conversion unchanged"), or cite modelspec, or drop the cross-reference — and state which, so mill-plan is not choosing.

### [NOTE] Throwaway-import verification must compile to be observable
**Section:** Testing → "How to confirm it is decided"
**Issue:** "Add a throwaway import to a production file locally, watch the test fail" only works if the import compiles (an unused named import is a build error, and a feature package would be a real test-build cycle) — otherwise `go test ./internal/lyxtest` reports a build failure, never a guard-test failure.
**Fix:** Name the concrete form: a blank import (`_ "…/internal/logger"`) of a non-cycle package, which `parser.ImportsOnly` still records in `astFile.Imports`.

### [NOTE] Enforced-by edit and rename must be one commit
**Section:** Work already landed / Decisions → rename consequence
**Issue:** The third `CONSTRAINTS.md` edit (adding `TestLeafInvariant_AllowlistOnly`) and the rename in `internal/lyxtest/leaf_enforcement_test.go` are described as mill-go work, but commit-per-fix could split them, briefly leaving `CONSTRAINTS.md:40` naming a function that does not exist.
**Fix:** State that the rename and the enforced-by line land in the same commit.

## Verdict

GAPS_FOUND
One retarget decision rests on a file relationship the source contradicts.
MILL_REVIEW_END
