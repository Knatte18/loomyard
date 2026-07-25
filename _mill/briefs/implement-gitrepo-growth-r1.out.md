Whole module builds cleanly. Final status.

{"status":"success","commit_sha":"c21d024f","session_id":"ef34c301-260b-4ffa-810f-f651f6e6bf42"}

Summary: implemented both cards of batch `01-gitrepo-growth` (2 of 2 cards committed).

- Card 1 (`ae90b444`): added `internal/gitrepo/pull.go` (`Repo.Pull()`, fast-forward-only via `git pull --ff-only`, no-stderr-leak error style) and `internal/gitrepo/pull_test.go` (integration-tagged, reusing `push_test.go`'s `newBareRemote`/`newRepoWithRemote`/`cloneFromBare` fixtures), plus `internal/gitrepo/doc.go` updates (new "Pull surface" section, updated operation list and scope-boundaries paragraph).
- Card 2 (`c21d024f`): added `internal/gitrepo/reset.go` (`Repo.ResetHard(sha)`, SHA-validated via `validSHA` before any git spawn, same no-stderr-leak error style) and `internal/gitrepo/reset_test.go` (integration-tagged, reusing `gitrepo_test.go`'s `newRepo`/`writeFile`/`commitAll`/`runGit` helpers), plus further `doc.go` updates (new "ResetHard surface" section, SHA-validation paragraph, operation list, scope-boundaries paragraph).

Verify command `go test -tags integration ./internal/gitrepo` passes; `go build ./...` also succeeds for the whole module. No files outside the batch's declared scope were touched, and no uncommitted tracked changes remain.
