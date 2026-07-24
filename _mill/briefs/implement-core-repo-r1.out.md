4 of 4 cards committed, matching all four `Commit:` messages in `## Cards` of `01-core-repo.md`. The batch's `verify:` command (`go test -tags integration ./internal/gitrepo/`) passes.

Summary of work:
- `/home/knatte/Code/loomyard/wts/gitrepo/internal/gitrepo/doc.go` — package doc comment for `internal/gitrepo`.
- `/home/knatte/Code/loomyard/wts/gitrepo/internal/gitrepo/gitrepo.go` — `Repo` type, `New`, unexported `run` helper over `gitexec.RunGit`, `ErrNoCommits`, `CurrentSHA`, `StageAndCommit`, `ChangedFilesSince`, `SHAExists`.
- `/home/knatte/Code/loomyard/wts/gitrepo/internal/gitrepo/testmain_test.go` — hermetic `TestMain` mirroring `internal/gitexec`'s.
- `/home/knatte/Code/loomyard/wts/gitrepo/internal/gitrepo/gitrepo_test.go` — integration-tagged tests covering all four primitives per the batch's scenario list.

Commits (in order): `ae850bbe`, `c587f65b`, `0b353128`, `918de263`, all pushed to `origin/gitrepo`.

4 of 4 cards committed — all complete.

{"status":"success","commit_sha":"918de263","session_id":"65edf1e0-63cb-4aa7-8772-6cece6ff5e50"}
