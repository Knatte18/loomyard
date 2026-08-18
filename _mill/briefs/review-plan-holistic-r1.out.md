MILL_REVIEW_BEGIN
# Review: websterengine + webstercli told-geometry, and Webster standalone entry — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (harness-reported model id claude-sonnet-5; self-assessment agrees with that disclosure)
reviewed_file: plan/
date: 2026-08-18
```

## Findings

### [BLOCKING:scope] Batch 8 deletes websterCLI struct fields two untouched test fixtures still reference
**Location:** batch 8 / card 36 (`internal/webstercli/cli.go`), interacting with `cli_test.go` and `verbs_test.go`
**Issue:** Card 36 deletes `websterCLI.layout`, `planDir`, `websterDir`, `reportsDir`, `promptsDir`, `websterScratchDir`. But `internal/webstercli/cli_test.go`'s `newTestCLI` helper (used by every validate/status/pause test, verified live at lines 176-184: `layout:`, `planDir:`, `websterDir:`, `websterScratchDir:`, `reportsDir:`, `promptsDir:`) and `internal/webstercli/verbs_test.go`'s `websterCLI{...}` fixture (verified live at lines 269-289, same field set) both still populate exactly these field names as composite-literal keys. Card 36's `Edits:` list is `cli.go, validate.go, beginbatch.go, recordbatch.go, recoverbatch.go, run.go, status.go, awaitbatch.go, pause.go` — no test file. Card 37 also touches `cli_test.go`, but only to rewrap the two `TestFabricSync_*` calls to the new `fabricSync` signature; its Requirements never mention `newTestCLI`. `verbs_test.go` is not in any batch-8 card's `Edits:` at all. Since a Go composite literal referencing a deleted struct field is a hard compile error, batch 8's own `verify:` (`go test ./internal/webstercli/... ./cmd/lyx/...` and the tagged half) cannot even compile, let alone pass — this violates the "repository builds at every batch boundary" Shared Decision.
**Fix:** Add `internal/webstercli/verbs_test.go` to card 36's `Edits:` (or a dedicated card between 36 and 41), and extend the Requirements to convert `newTestCLI` (cli_test.go) and the `websterCLI{...}` fixture (verbs_test.go) from the deleted fields to `geom`/`refMatcher`/`openFabric`, the same way card 33 already converted the *Deps constructions in batch 7.

## Verdict
REQUEST_CHANGES
Batch 8 card 36's field deletion breaks two test fixtures (`cli_test.go`, `verbs_test.go`) no card repairs, failing the batch's own verify.
MILL_REVIEW_END
