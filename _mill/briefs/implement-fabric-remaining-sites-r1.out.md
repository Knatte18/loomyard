{"status":"success","commit_sha":"58436304a238dca7a6db58e0b5badc532310d6ff","session_id":"765c3521-fcb2-4f34-9e69-299d8bcfa4a8","cards_done":[27,28,29]}

Summary: All 3 of 3 cards in batch `06-fabric-remaining-sites` were committed this turn.

- Card 27 (`internal/fabricengine/index.go`): migrated `weftGitDir`, `warpSeq` (plain two-message merges), and `scanWarpSHATrailers` (unborn-HEAD sniff moved onto `errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, ...)`). Commit `55ae6462`.
- Card 28 (`internal/fabricengine/pull.go`): migrated `weftHasUpstream` (mixed rev-parse probe via `errors.As` recovery), `warpUpstreamSHA`, and `patternResidueCommits` (plain merges). Commit `10c49bc1`.
- Card 29 (six single-site files: `dirtiness.go`, `gitexclude.go`, `hook.go`, `status.go`, `weftgit.go`, `worktreelist.go`): all plain two-message merges. Verified `worktreelist_test.go`'s `TestList_NotAGitRepo` (build-tagged `integration`, contrary to the batch text's "untagged" description, but its assertion is the one the batch calls out) still matches `"not a git repository"` through the merged `*GitError`. Commit `58436304`.

Verify ran and passed in full: `go build ./...`, `go test ./internal/fabricengine/...` (ok, 0.050s), `go test -tags integration ./internal/fabricengine/...` (ok, 15.277s).

Post-batch grep confirms exactly two `gitexec.RunGit` sites remain in non-test source files under `internal/fabricengine` (`weftwiring.go:73` and `weftwiring.go:90`), both already carrying their batch-4 `//gitexec:raw` markers — matching the count batch 8's guard pins.

No tracked uncommitted changes remain (`git status --porcelain --untracked-files=no` is empty). The only untracked file is the implementer brief itself (`_mill/briefs/implement-fabric-remaining-sites-r1.md`), outside this batch's file scope, left untouched.

{"status":"success","commit_sha":"58436304a238dca7a6db58e0b5badc532310d6ff","session_id":"765c3521-fcb2-4f34-9e69-299d8bcfa4a8","cards_done":[27,28,29]}
