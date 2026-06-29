{"status":"success","commit_sha":"a1faf0a251bdb30669e2dff827c4a6fc4cfcaab6","session_id":"ab35b369-a4d2-4f5e-b216-5cdcc5d2431c"}

All 5 cards delivered and verified:

- `tools/sandbox/test-scheme.md` -- tracked test-scheme template with updated capture instructions (lyx ghissues create), black-box rule, fingerprint note, pre-conditions, and S0-S6 scenario spine
- `tools/sandbox/suite.go` -- suite launcher with binaryFingerprint, renderScheme, ensureGitExclude, runSuite, and testability seams (lookPath, launchAgent)
- `tools/sandbox/main.go` -- refactored to run(argv) dispatch with build/suite subcommands and back-compat -reset
- `tools/sandbox/main_test.go` -- extended with 6 new run() dispatch tests
- `tools/sandbox/suite_test.go` -- 9 new tests covering all suite functions
- `docs/sandbox-hub.md` -- new "Running the Suite Agent" and "Build subcommand (default)" sections

Verify: `go test ./tools/sandbox/...` passes all 22 tests (11 pre-existing + 11 new).

{"status":"success","commit_sha":"a1faf0a251bdb30669e2dff827c4a6fc4cfcaab6","session_id":"ab35b369-a4d2-4f5e-b216-5cdcc5d2431c"}
