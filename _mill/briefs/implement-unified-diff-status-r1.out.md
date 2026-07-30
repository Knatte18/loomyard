All 3 cards in the batch are committed and verified:

- Card 11 (commit `eb95ee41`): `internal/gitrepo/worktree.go`, `internal/gitrepo/worktree_test.go`, `internal/gitrepo/gogit.go` — adds `WorktreeChangedFiles`.
- Card 12 (commit `f6f39c7b`): `internal/fabricengine/diff.go` — adds `weftAnchorForWarpSHA`, `ChangeSide`/`ChangeEntry`/`DiffResult`, `Fabric.Diff`, `Fabric.Status`.
- Card 13 (commit `57ce35ab`): `internal/fabricengine/diff_integration_test.go` plus a fix to `internal/gitrepo/worktree.go`.

Notable finding fixed along the way: go-git v5.19.1's `Worktree.Status()` does not actually honor `.git/info/exclude` despite the plan's stated (source-traced) assumption — its `gitignore.ReadPatterns` call runs against the worktree filesystem, which is chrooted and refuses any path containing a `.git` component ("invalid path component"), and that open error is silently swallowed, so zero exclude-file patterns end up applied. I confirmed this empirically with a minimal repro, then fixed `WorktreeChangedFiles` to read `info/exclude` through the git-dir filesystem (`readGitDirExcludePatterns`) and assign it to `Worktree.Excludes` before calling `Status()`. This is covered by `TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts`, which explicitly creates the `.weft/` lock file and `.gitrepo-push.lock` and asserts neither surfaces in `Fabric.Status()`'s weft-side result — a genuine regression guard, not a vacuous assertion.

Batch verify command passed: `go test -tags integration ./internal/gitrepo/ ./internal/fabricengine/` — both packages `ok`. Full `go build ./...`, `go vet ./...`, and `gofmt -l` on touched files are clean. `git status --porcelain --untracked-files=no` shows no dirty tracked files (only the pre-existing untracked brief file remains, out of scope).

Card-count self-check: 3 of 3 declared cards committed (Cards 11, 12, 13), confirmed via `git log 723c5b09..HEAD --oneline` matching all three `Commit:` subjects. All cards complete — this is a verified, unqualified completion claim.

{"status":"success","commit_sha":"57ce35ab","session_id":"2c201db5-cc26-4350-b3ad-9b611322f80b","cards_done":[11,12,13]}