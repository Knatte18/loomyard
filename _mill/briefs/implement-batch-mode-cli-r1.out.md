{"status":"success","commit_sha":"6b330faf9aec7c1ab263c29973b2168e3e616390","session_id":"1bd56d03-ebbc-4829-bc6d-243aa33b36ba","cards_done":[38,39,40,41,42]}

All 5 of 5 cards in the batch-mode-cli batch were committed this turn (cards 38, 39, 40, 41, 42), each with its own commit exactly matching the plan's `Commit:` messages. `verify: go test -count=1 ./internal/codeintelcli/...` passed cleanly on the first attempt — no self-fix needed.

Summary of changes, all in `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelcli/cli.go` and `cli_test.go`:

- Card 38 (`cli.go`): added `batchStatus`/`statusRank`, `classifyLookupError`, `classifySymbolError`, and the generic `runBatch` runner.
- Card 39 (`cli.go`): `refsCommand`/`definitionCommand` switched to `cobra.MinimumNArgs(1)`, branching on `len(args)` after the arg-count-independent cwd/registry preamble; batch mode wired through `runBatch` + `classifyLookupError`; both `Long` help texts updated with the batch-mode paragraph and example.
- Card 40 (`cli.go`): `symbolCommand` given the same treatment, using `classifySymbolError` (no ambiguous branch), with `--help` documenting the three-way status set.
- Card 41 (`cli_test.go`): renamed `TestRunCLI_Refs_RequiresExactlyOneArg` to `TestRunCLI_Refs_RequiresAtLeastOneArg` (dropped the stale `two_args` row), added `TestRunCLI_Refs_TwoArgsIsBatchMode`.
- Card 42 (`cli_test.go`): added `TestBatchRunner_WorstOutcomeWinsExitCode` (table-driven, direct `runBatch` test) and `TestClassifySymbolError_MultipleMatchesIsFoundNotAmbiguous`.

Pre-report self-check confirmed no uncommitted tracked changes (`git status --porcelain --untracked-files=no` empty), and the commit-log match against the batch's `## Cards` confirms all 5 declared cards are accounted for — 5 of 5 cards committed, all complete.
