All 7 cards (13-19) are committed with commit subjects matching the batch file's declared `Commit:` messages exactly -- 7 of 7 cards complete. One additional self-fix commit (`c450acf4`) was needed during the Verify step to fix three genuine subdirectory-invocation integration tests that the mechanical struct-literal sweep couldn't reach (they drive real `lyxcwd.Resolve` calls, not synthetic literals).

Summary of work:
- `internal/fabricengine/*_test.go`, `internal/fabriccli/cli_test.go` -- card 13
- `internal/webstercli/*_test.go`, `internal/websterengine/*_test.go` -- card 14
- `internal/buildercli/*`, `internal/builderengine/spawn_test.go`, `internal/burlerengine/*`, `internal/loomengine/*`, `internal/lyxtest/lyxtest_test.go`, `internal/treadleengine/smoke_judge_test.go` -- card 15
- `internal/perchcli/*`, `internal/perchengine/*`, `internal/reedcli/*`, `internal/reedengine/*`, `internal/scoutengine/*`, `internal/shuttlecli/*`, `internal/shuttleengine/*` -- card 16 (includes fixing a real functional bug in `scoutengine/refs_integration_test.go` pointing at a stale file path)
- `cmd/lyx/*`, `internal/boardcli/*`, `internal/configcli/*`, `internal/configengine/config_test.go`, `internal/ideengine/*`, `internal/modelspec/*`, `internal/pattern/*`, `internal/tokenvocab/*`, `internal/vscode/color_test.go` -- card 17
- `CONSTRAINTS.md` + 29 comment-sweep files -- card 18
- `internal/lyxcwd/enforcement_test.go` -- card 19
- Verify-step fix: `internal/fabricengine/remove_junctions_integration_test.go`, `internal/loomengine/preflight_integration_test.go`, `internal/perchcli/cli_integration_test.go`

Full verify command passed: `go vet -tags "integration smoke scout" ./...` clean, `go test ./...` all green, `go test -tags integration ./...` all green.

{"status":"success","commit_sha":"c450acf4a52acb0c4c69187e0d54cd8a5d81a25d","session_id":"9c051bd2-9350-4396-8eda-c46dea8c9dd7","cards_done":[13,14,15,16,17,18,19]}