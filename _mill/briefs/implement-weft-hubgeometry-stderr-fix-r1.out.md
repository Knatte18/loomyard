{"status":"success","commit_sha":"85d769d","session_id":"b55724be-06b4-4048-ae7a-705e3c8d7947"}

Summary of work:
- `internal/weftengine/sync.go` — Card 16: `pushUnpushed`'s final error path now reads `fmt.Errorf("push from %q failed (git exit %d) after rebase retry", weftPath, code)` instead of interpolating git's raw `stderr`.
- `internal/weftengine/sync_test.go` — added `TestPush_BrokenRemoteFailsWithoutStderrLeak`, which breaks the origin URL after a commit to force a non-retryable push failure, asserting the error contains the weft path (quoted) and no `"fatal:"` substring.
- `internal/hubgeometry/worktreelist.go` — Card 17: `List`'s non-zero-exit error now reads `fmt.Errorf("list git worktrees in %q failed (git exit %d)", sourceDir, exitCode)`; `stderr` is blanked (`_`) since it is now unused.
- `internal/hubgeometry/worktreelist_test.go` — added `TestList_NotAGitRepo`, calling `List` against a non-repo temp dir, asserting the error contains the source dir (quoted) and no `"fatal:"` substring.

Both cards committed and pushed (`4ae026b`, `85d769d`). Verify command `go test ./internal/weftengine/... ./internal/hubgeometry/...` and the integration-tagged variant both pass; working tree is clean aside from the untracked brief file.

{"status":"success","commit_sha":"85d769d","session_id":"b55724be-06b4-4048-ae7a-705e3c8d7947"}
