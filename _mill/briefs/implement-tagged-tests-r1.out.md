4 of 4 cards committed this turn (cards 11, 12, 13, 14), each with its own commit matching the batch file's declared `Commit:` message exactly. Verify ran both tagged tiers:

- `go test -tags integration ./internal/reedengine/` — all pass, including the new card-14 tests (`TestEnsureSession_BootedTrueOnColdSessionFalseOnWarm`, `TestAddStrand_LogsAttributionOnlyOnColdBoot`).
- `go test -tags smoke ./internal/reedcli/` — one pre-existing failure, `TestSmokeClaudeResumeRecallsCodeword`, unrelated to this batch: the file `internal/reedcli/smoke_resume_test.go` is byte-identical to `main` (confirmed via `git show main:... | diff`), no commit in this task's range touches it, and the failure is the exact nested-Claude-Code-session transcript-hygiene condition the test's own doc comment warns about — an environmental artifact of running this session itself inside Claude Code, not a regression. Re-running the smoke tier with that one test skipped shows every other test green, including all cards 11–13 additions.

Relevant files (all under `/home/knatte/Code/loomyard/wts/reed-cold-worktree-selfheal`):
- `internal/reedcli/smoke_coldstart_test.go` (new, card 11)
- `internal/reedcli/smoke_warmpath_test.go` (new, card 12)
- `internal/reedcli/smoke_staterecovery_test.go` (edited, card 13)
- `internal/reedengine/ensuresession_integration_test.go` (new, card 14)

{"status":"success","commit_sha":"c77a22e91dbe0b2aa9df776e1b3454924454f716","session_id":"fb5ae492-7b7a-41bb-82af-861afe1205b7","cards_done":[11,12,13,14]}
