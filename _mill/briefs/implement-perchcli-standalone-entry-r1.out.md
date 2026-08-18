6 of 6 content-commit cards committed (cards 20, 21, 22, 23, 24, 25), plus card 26 (Commit: none) verified this turn. All 7 cards in the batch are complete.

{"status":"success","commit_sha":"2e46a57210f232971be08e3a76f7a949e9a5f547","session_id":"b4538c06-60d2-43cc-a49f-13067ab1c6f4","cards_done":[20,21,22,23,24,25,26]}

All 7 cards in batch `perchcli-standalone-entry` are complete: 6 cards produced real commits (cards 20-25), and card 26 (Commit: none, verification-only) was re-performed this turn with no findings. Both verify sub-invocations passed: `go test ./internal/perchcli/...` and `go test -tags integration ./internal/perchcli/...`.

Key files touched:
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/perchcli/wiring.go` (new) — `wire`/`wireHub`/`wireStandalone`/`resolveStandaloneTarget`
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/perchcli/cli.go` — dropped the `layout *lyxcwd.Location` field, added `stencilsDir`/`anchorRel`/`openFabric`, extracted `resolvePersistentPreRun`, added `--stencils-dir`/`--target-dir` flags
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/perchcli/run.go` — rerouted the three `c.layout` reads onto `c.stencilsDir`, `c.anchorRel`, `c.openFabric()`; added `mode`/`stateDir`/`stencilsDir` to the success envelope
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/perchcli/cli_test.go`, `run_test.go` — redirected `XDG_STATE_HOME`/`LOCALAPPDATA` in the three tests now reaching `wireStandalone`, updated stale doc comments
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/perchcli/wiring_test.go` (new) — tier-1 truth table, standalone pinned values, two-consumer stencils-dir rule
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/perchcli/cli_integration_test.go` — added `TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate` and `TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged`

Final HEAD: `2e46a57210f232971be08e3a76f7a949e9a5f547`

{"status":"success","commit_sha":"2e46a57210f232971be08e3a76f7a949e9a5f547","session_id":"b4538c06-60d2-43cc-a49f-13067ab1c6f4","cards_done":[20,21,22,23,24,25,26]}