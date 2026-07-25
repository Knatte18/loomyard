All 4 cards (Cards 4, 5, 6, 7) are committed, matching the batch file's four `Commit:` messages exactly. Verify passed. Final report follows.

{"status":"success","commit_sha":"d1ee85e6","session_id":"f02676d5-7ee1-4db8-b60a-efd9a154cd7a","cards_done":[4,5,6,7]}

Summary: 4 of 4 cards committed (Cards 4, 5, 6, 7), verified honestly via `git log <start>..HEAD --oneline` against the batch file's `### Card N:` commit messages.

Key files:
- `/home/knatte/Code/loomyard/wts/git-native-library/internal/gitnativepoc/read.go` — new file: `CurrentSHA`, `SHAExists`, `ChangedFilesSince`, `SnapshotSHA`, `remoteName`, `hasUnpushed`, `isStrictDescendant`, plus their sentinel errors (`ErrNoCommits`, `ErrInvalidSHA`, `ErrInvalidSnapshotKey`) and validation helpers, all over go-git's object model.
- `/home/knatte/Code/loomyard/wts/git-native-library/internal/gitnativepoc/read_test.go` — new file: differential parity tests against `internal/gitrepo.Repo` (the CLI oracle) plus direct git-fixture truth-table tests for gitrepo's unexported helpers (`remoteName`, `hasUnpushed`, `isStrictDescendant`), each carrying its MIGRATE/CLI-BOUND verdict as a comment.

Notable findings recorded in code comments (per the `cli-bound-is-a-recorded-outcome` Shared Decision):
- go-git's `object.DiffTree` (unlike `Tree.Diff`) performs no rename detection, giving the `--no-renames` equivalent for free — MIGRATE.
- go-git's revision parser recognizes `@{u}` syntax, but `Repository.ResolveRevision` never implements the resulting `AtUpstream` case (verified empirically against a real upstream-tracking clone) — CLI-BOUND for that literal syntax; `hasUnpushed` reaches the same behavioral result (including on diverged history) by resolving the upstream ref from branch config and walking commits with `object.NewCommitPreorderIter`'s ignore-list seeded with the upstream hash.
- go-git's custom-refspec `Fetch` (`+refs/loomyard/snapshot/*:refs/loomyard/snapshot/*`) works correctly against a real bare remote — MIGRATE, confirmed via a manual probe against a bare+clone fixture before writing `SnapshotSHA`.

All four commits are pushed to `git-native-library`. Batch verify command ran clean: `go test -tags integration ./internal/gitnativepoc/` and the two scoped `cmd/lyx` guard tests both passed.
