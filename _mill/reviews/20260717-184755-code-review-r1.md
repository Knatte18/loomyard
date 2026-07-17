MILL_REVIEW_BEGIN
# Review: Extend codeintel lookup to non-Go languages via LSP — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-17
```

## Findings

### [NIT] Integration test skips an ErrServerNotFound case that doesn't need gopls
**Location:** `internal/codeintelengine/refs_integration_test.go:72-75`
**Issue:** `TestReferences_Integration`'s top-level `t.Skip` on missing `gopls` also skips the `t.Run("non-existent server binary yields ErrServerNotFound", ...)` subtest at line 111, which never launches gopls and could run standalone.
**Fix:** Move the gopls-presence check inside the first `t.Run` only, so the `ErrServerNotFound` subtest runs unconditionally. Non-blocking: batch 4 installs gopls, so this is only a coverage gap on a gopls-less machine.

### [NIT] No untagged unit test exercises References()'s ErrServerNotFound mapping
**Location:** `internal/codeintelengine/refs.go:81-87`, `internal/codeintelcli/cli_test.go`
**Issue:** The `exec.LookPath` failure → `*ErrServerNotFound` translation in `References` is only exercised by the integration-tagged test (gated behind gopls presence per the finding above); no offline/untagged test in either `codeintelengine` or `codeintelcli` covers it, even though it needs no real server (a bogus registry `Command` is enough, no subprocess spawn).
**Fix:** Add an untagged test (e.g. in `refs_test.go` or `codeintelcli/cli_test.go`) that points a registry entry's `Command` at a nonexistent binary and asserts `errors.Is(err, codeintelengine.ErrServerNotFoundSentinel)`.

## Verdict

APPROVE
All four batches align end-to-end with the plan, shared decisions, and constraints; only two low-severity coverage NITs found.
MILL_REVIEW_END
