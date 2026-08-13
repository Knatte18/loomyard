All 6 of 6 cards committed and verified. Summary of work:

- Card 13 — `internal/fabriccli/cli_test.go`: removed all 18 `t.Chdir` calls, migrated to `RunCLIIn`/`--into`, added a subprocess-based fallback-vs-injected-cwd test and two `--into` tests (commit `4f46da25`).
- Card 14 — `internal/perchcli/{cli_integration_test.go,run_integration_test.go}`: removed both chdirs, migrated to `RunCLIIn`, added `t.Parallel()` to the pause suite (commit `25c9f9b5`).
- Card 15 — `internal/configcli/configcli_integration_test.go`, `internal/webstercli/verbs_test.go`, `internal/idecli/cli_test.go`: removed all chdirs, migrated to `RunCLIIn`, stayed serial (commit `c15d513f`).
- Card 16 — `internal/reedcli/cli_integration_test.go`, `internal/loomengine/preflight_integration_test.go`: removed chdirs, added `t.Parallel()` throughout (commit `96a62692`).
- Card 17 — new `cmd/lyx/cwdmutation_test.go` guard plus `tierpurity_test.go`/`CONSTRAINTS.md` updates (commit `7b17bde9`).
- Card 18 — `docs/benchmarks/running-tests.md` and `docs/benchmarks/test-suite-timing.md` updated with measured timing and the `LYX_TRACE` disposition (commit `df25e32e`).

`verify:` (`go test ./cmd/lyx/...` and `go test -tags integration` over the seven CLI/loomengine packages) passes, `go vet -tags integration ./...` is clean, and `git status --porcelain` shows no dirty tracked files.

{"status":"success","commit_sha":"df25e32e0977a2d28e46be9e0e42cd621468af88","session_id":"74b7ac37-0d3f-4622-88eb-abed22e34fa3","cards_done":[13,14,15,16,17,18]}
