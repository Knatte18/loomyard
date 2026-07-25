All 6 cards (15-20) committed as 6 separate commits, one per card, matching the batch file's declared `## Cards` count exactly. Verify (`go test -tags integration ./internal/websterengine/...`) passes.

{"status":"success","commit_sha":"37451217","session_id":"6b4d5dc8-4f87-45a7-81dc-89fb60dc717b","cards_done":[15,16,17,18,19,20]}

Summary of work (6 of 6 cards committed, verified honestly against the commit range above):

- `internal/websterengine/gitwrap.go` / `gitwrap_test.go` (card 15) - `headSHA`/`changedFiles` re-point to `internal/gitrepo`; `dirty` wraps `gitexec.RunGit` directly (the one carve-out). Integration-tagged tests.
- `internal/websterengine/fingerprint.go` / `fingerprint_test.go` (card 16) - webster-local copy of `builderengine.Fingerprint`.
- `internal/websterengine/pause.go` / `pause_test.go` (card 17) - exported `PauseFlagName`/`RequestPause`/`PauseRequested`/`ClearPause`.
- `internal/websterengine/archive.go` / `archive_test.go` (card 18) - `firstFreeArchivePath`, `archiveStateFile`, `archiveReportsDir`, reusing the existing `stateFileName` const from `state.go`.
- `internal/websterengine/outcome.go` / `outcome_test.go` (card 19) - webster's own `outcome` type/vocabulary, `parseOutcome`, `archiveStaleOutcome`.
- `internal/websterengine/strand.go` / `strand_test.go` (card 20) - exported `StrandLive`/`TurnEnded`, the `Starter`/`OrchestratorStarter`/`OrchestratorHandle` spawn-seam interfaces, and unexported `removeStrandIfLive`.

All files are new additions only (no existing file was edited), matching the batch's "adds new files, doesn't retarget call sites yet" scope. `go test -tags integration ./internal/websterengine/...` and `go build ./...` both pass. `git status --porcelain --untracked-files=no` is clean (only the brief file itself is untracked, which is expected orchestration input, not project source).
