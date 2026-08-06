All 7 cards (30-36) committed successfully. Full verify passed: `go vet -tags "integration smoke scout" ./...` clean, untagged `go test` clean across all 17 packages, and `go test -tags integration` clean across all 17 packages. Working tree is clean with no uncommitted tracked changes.

Card-count self-check: 7 of 7 cards committed (cards 30, 31, 32 were already done from a prior session per the resume range; this session completed cards 33, 34, 35, and 36). All commit subjects match the batch file's `Commit:` messages exactly.

Key files touched across this session (cards 33-36):
- `internal/lyxcwd/lyxcwd.go` -- portal/launcher/hub-structural/pattern accessors all removed
- `internal/lyxcwd/anchor.go` -- private `boardDir`/`boardDirName` retained for `readRecordedAnchor`
- `internal/fabricengine/portals.go`, `launchers.go` -- new home for portal/launcher surface
- `internal/fabricengine/junctionnames.go` -- new home for `BoardDir`, `HubPath`, `HubReservedNames`, `IsReservedHubName`
- `internal/fabricengine/portallauncher_test.go`, `junctionnames_test.go` -- relocated coverage
- `internal/fabricengine/pull.go` -- new private `patternDirName` const
- `internal/lyxcwd/enforcement_test.go` -- token-ownership map fully updated
- `_mill/plan/06-fabric-owns-the-illusion.md` -- extended with `perchcli/cli_integration_test.go` (discovered live call site outside the original Edits list)
- Roughly 30 other production/test files across `boardcli`, `boardengine`, `buildercli`, `configcli`, `configsync`, `fabriccli`, `ideengine`, `loomengine`, `perchcli`, `webstercli` retargeted to the new `fabricengine` accessors.

{"status":"success","commit_sha":"9013f16658bbae81431f80311770e2d09bbdfbff","session_id":"0ac50a79-6c67-4a32-9707-7ecec36e8f09","cards_done":[30,31,32,33,34,35,36]}