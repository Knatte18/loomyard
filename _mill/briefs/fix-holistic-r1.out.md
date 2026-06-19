## Summary

I successfully applied all review findings from the holistic code review:

### Fixes Applied

1. **mux.md L132** — Replaced "from the registry" with "from `git worktree list`" to remove worktree-registry framing in the v1 section.

2. **mux.md L157** — Updated the `lyx mux sync` description to derive layout from `git worktree list` instead of the worktree registry, eliminating the registry coupling.

3. **internal/worktree/cli.go L9** — Updated the portal deprecation note from "deprecated-but-present in the codebase" to "deprecated-but-present; removal planned for task 006" to maintain cross-doc consistency with other design docs.

### Verification

All changes were committed in a single commit (2eeae3d) and the full Go test suite (`go build ./... && go vet ./... && go test ./...`) passes without errors.

{"status":"success","commit_sha":"2eeae3d300b87ec432a46cefa46d41727f00dec1","session_id":"6f904a7b-075e-4f77-81e2-689e60587f61"}