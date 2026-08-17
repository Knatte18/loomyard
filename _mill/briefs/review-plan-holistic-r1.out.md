MILL_REVIEW_BEGIN
# Review: config degrades to embedded template — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-17
```

## Findings

### [NIT:consistency] Card 9's "four siblings" count is wrong
**Location:** batch 3 / card 9 **Issue:** Card 9 says to allowlist the new T10 guard in `tierpurity_test.go`'s `allowedSpawners` "as its four siblings do" (files resolving their scan root via `go env GOMOD`), but the current map has six such entries (`gitrepoboundary_test.go`, `boardguard_test.go`, `destructiveguard_test.go`, `uncontainedwrite_test.go`, `checkedcall_test.go`, `cwdmutation_test.go`). **Fix:** Say "six siblings" (or drop the count) so T10 inherits an accurate count.

### [NIT:scope] Fallback-path non-absence failure wrap is untested
**Location:** batch 1 / card 3 **Issue:** Card 2 gives the fallback tail its own error wrap, `%s config template: %w`, for a failure of `envsource.Build`/`yamlengine.Resolve` on the template bytes (e.g. an unset required `${env:NAME}` with no default), but card 3's test list has no case exercising that branch or asserting the wrap text. **Fix:** Add one `LoadOrTemplate` test seeding a template with an unset required env marker and no `_lyx/`, asserting the error is non-nil and contains `config template:`.

## Verdict

APPROVE
Plan is internally consistent, decisions are faithfully carried through every card, and source claims check out against the referenced files.
MILL_REVIEW_END
