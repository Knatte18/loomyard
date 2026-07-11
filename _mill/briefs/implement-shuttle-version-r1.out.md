All 3 of 3 cards committed, matching the batch file's `## Cards` Commit messages exactly. No tracked dirty files. Verify (`go test ./internal/shuttleengine/...`) passes.

{"status":"success","commit_sha":"fabaf6d","session_id":"5fd2ba1a-7e81-4935-ae9c-67b3099ce63c"}

Summary: implemented all 3 cards of batch 03-shuttle-version.

- Card 11 (`84b128f`): added `Version string` to `internal/shuttleengine/spec.go` (Spec struct), doc comment parallel to `Effort`, plus `TestSpec_Validate_VersionUntouched` in `internal/shuttleengine/spec_test.go` proving `validate` never inspects it.
- Card 12 (`598147b`): added `resolveModelID(model, version string) (string, error)` in `internal/shuttleengine/claudeengine/command.go`, wired into `Prepare` in `internal/shuttleengine/claudeengine/claudeengine.go` (resolved id passed to `buildLaunchCmd`, unchanged itself), with table tests in `internal/shuttleengine/claudeengine/command_test.go` (`TestResolveModelID`) and wiring cases in `internal/shuttleengine/claudeengine/prepare_test.go`.
- Card 13 (`fabaf6d`): generalized the pinning-rule sentence in `docs/reference/model-spec.md` and added the `Spec.Version` sentence to the shuttle bullet in `docs/overview.md`.

Verify command `go test ./internal/shuttleengine/...` passes. All commits pushed to `origin/modelspec`. 3 of 3 cards committed — full batch complete.

{"status":"success","commit_sha":"fabaf6d","session_id":"5fd2ba1a-7e81-4935-ae9c-67b3099ce63c"}
