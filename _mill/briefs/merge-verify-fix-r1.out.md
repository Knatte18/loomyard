## Summary

I successfully diagnosed and fixed the test failures in the merge. The issue was straightforward:

**Root Cause:**
The merge renamed `console-header.md` to `status-line.md` and updated reed command references. The `StatusLineText()` method in `internal/reedengine/statusline.go` renders the status-line template using `tokenvocab.Render()`, which requires three tokens to be provided via a context struct: `repo`, `hub`, and **`worktree`**.

The two failing integration tests (`TestEnsureSession_BootedTrueOnColdSessionFalseOnWarm` and `TestAddStrand_LogsAttributionOnlyOnColdBoot`) were failing with the error: `stencil: unfilled top-level marker(s): worktree`

**The Fix:**
The test fixture `newColdScratchEngine` in `/home/knatte/Code/loomyard/wts/reed-cold-worktree-selfheal/internal/reedengine/ensuresession_integration_test.go` was not setting the `WorktreeName` field when constructing the `Geometry` struct. I added this single line:

```go
WorktreeName: filepath.Base(tmpDir),
```

This provides the missing "worktree" token value to the template renderer, following the same pattern used in other test files in the same package.

**Verification:**
- Both smoke and integration tests now pass
- The verify command passes: `go test -tags smoke ./internal/reedcli/ -skip '^TestSmokeClaudeResumeRecallsCodeword$' && go test -tags integration ./internal/reedengine/`
- Change committed as: `d576aa644`

{"status":"success"}
