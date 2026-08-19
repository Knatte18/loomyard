All 6 cards match the batch file's Commit: messages exactly (Cards 26-31). 6 of 6 cards committed — all complete.

{"status":"success","commit_sha":"67bb209f95cef260afbd5c1a02d1647504b110e5","session_id":"d88f9182-1a99-4f52-bd32-92545b09ddb9","cards_done":[26,27,28,29,30,31]}

Summary: All 6 cards of batch 06-registration-and-guards were implemented and committed (6 of 6 cards committed — genuinely complete, not a partial claim). Files touched:

- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/cmd/lyx/main.go` — registered `loomcli.Command()` and `loomcli.RunAliasCommand()` in the root, appended `loom`/`run` to the Long prose
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/cmd/lyx/seamsignature_test.go` — added `loomcli.RunCLI`/`RunCLIIn` to the pinned slices, updated counts to thirteen/twelve
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/cmd/lyx/helptree_test.go` — added `loom`/`run` to required modules, added loom's four-subcommand case
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/cmd/lyx/sandbox_coverage_test.go` — allowlisted `run` as loom's alias
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/tools/sandbox/SANDBOX-CORE-SUITE.md` — added scenario S8 (status/pause over a hand-written fixture) and updated the session-log format block
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/CONSTRAINTS.md` — moved CLI/Cobra Invariant seam counts to thirteen/twelve, added the Alias shape bullet, named loom's two interactive-handoff exception holders
- `/home/knatte/Code/loomyard/wts/loom-session-bootstrap/docs/overview.md` — added `internal/loomcli` to the package tree, fixed the stale loomshed row count (12->13), moved the seam-count sentence, rewrote the loom modules-list bullet as implemented

`verify: go test ./cmd/lyx/ ./internal/loomcli/` passes, gofmt is clean, and the working tree has no uncommitted tracked changes.

{"status":"success","commit_sha":"67bb209f95cef260afbd5c1a02d1647504b110e5","session_id":"d88f9182-1a99-4f52-bd32-92545b09ddb9","cards_done":[26,27,28,29,30,31]}
