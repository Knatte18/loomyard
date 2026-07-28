HEAD (718eb13c) differs from the baseline (75173022...), no tracked modifications remain uncommitted, and all verify commands passed. All three findings were doc-only corrections (no new test was mandated by the review).

{"status":"success","commit_sha":"718eb13cf38c539c8cc3af1e011f777cbe5fece6","session_id":"b4deb0dd-00bc-4bb6-ace0-583079711319"}

## Summary

Fixed all three BLOCKING findings from the holistic review — all describing the same underlying inconsistency: `hasUnpushed` was migrated to a go-git ancestry walk in batch 4 but measured and reverted to the CLI per card 21's reversal criterion, and three doc sites hadn't caught up.

1. **`/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/CONSTRAINTS.md`** (line ~160) — added `hasUnpushed` to the gitrepo Client Boundary Invariant's named CLI-bound set. Commit `35603643`.
2. **`/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/internal/gitrepo/doc.go`** (lines 16-23) — moved `hasUnpushed` from the go-git read-surface list to the CLI-bound list in the package doc's opening section. Commit `35603643`.
3. **`/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/manifest/roadmap.md`** (line 65) — removed `hasUnpushed` from the "migrated to go-git" list in the task's Done entry and added a clause noting it was measured and reverted to the CLI. Commit `718eb13c`.

All three files were already within batch 9's (`docs-and-invariants`) declared `Edits:` scope, so no plan extension was needed.

Verify commands run (in plan order, from `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients`), all green:
- `go test -tags integration -race -count=1 ./internal/gitrepo/...` (covers batches 1-5, identical command)
- `go test -race -count=1 ./internal/githubclient/...` (batch 6)
- `go test -race -count=1 ./internal/selfreportengine/... ./internal/selfreportcli/...` (batch 7)
- `go test -count=1 ./cmd/lyx/... ./tools/sandbox/... ./internal/gitrepo/...` (batch 8)
- `go test -count=1 ./cmd/lyx/... ./internal/gitrepo/... ./internal/githubclient/...` (batch 9)

Both commits were pushed to `origin/native-clients-migration`. Working tree has no uncommitted tracked modifications; only pre-existing untracked `_mill/briefs/*` scratch files remain (not part of this task's scope).

{"status":"success","commit_sha":"718eb13cf38c539c8cc3af1e011f777cbe5fece6","session_id":"b4deb0dd-00bc-4bb6-ace0-583079711319"}
