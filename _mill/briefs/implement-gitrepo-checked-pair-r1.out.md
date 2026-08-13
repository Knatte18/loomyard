All 8 cards (5 through 12) are committed with distinct commits matching the batch's declared `Commit:` messages, verify passed fully, and the working tree is clean.

Card-count self-check: 8 of 8 cards committed (cards 5, 6, 7, 8, 9, 10, 11, 12 — all with matching commit subjects in the log above). All complete.

Summary of changes (all under `internal/gitrepo/` and `cmd/lyx/`):
- `internal/gitrepo/gitrepo.go`: added `runChecked` beside `run`, migrated `StageAndCommit`, `CommitEmpty`, `StageAllAndCommit`, `CheckoutDetached`, `RestoreBranch`.
- `internal/gitrepo/push.go`: migrated `pushWithRebaseRetry`, `PushRebaseFree`, `HasUnpushed`, preserving the rebase-retry stderr sniff and the bare `ErrPushRejected` sentinel.
- `internal/gitrepo/ancestry.go`: migrated `IsAncestor`'s tri-state.
- `internal/gitrepo/reset.go` + `internal/gitrepo/doc.go`: migrated `ResetHard`, dropped its unpinned suppression claim.
- `internal/gitrepo/pull.go`: added `//gitexec:raw` markers on `Pull`/`Fetch`, left them raw and unchanged otherwise.
- `internal/gitrepo/doc.go`: corrected the two-backend boundary prose (RunGit/Run pair, re-derived call-site count).
- `cmd/lyx/gitrepoboundary_test.go`: repaired the Client Boundary guard to key on both `run` and `runChecked`, replaced the substring `gitexec.` count with an AST call-expression count asserting exactly 2 (one in each chokepoint's body).

`verify:` (`go build ./...`, `go test ./internal/gitrepo/... ./cmd/lyx/...`, `go test -tags integration ./internal/gitrepo/...`) all passed.

{"status":"success","commit_sha":"870dec82d43a8476ac3f1035bb4f8e011111d10b","session_id":"83620bdd-0ab8-44e6-8d70-538e0428edee","cards_done":[5,6,7,8,9,10,11,12]}
