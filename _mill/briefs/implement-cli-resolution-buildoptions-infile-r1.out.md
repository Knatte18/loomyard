All 3 cards in this batch (Cards 7, 8, 9) are committed and match the batch file's declared `Commit:` messages exactly. 3 of 3 cards committed. Verify (`go test ./internal/codeintelcli/...`) passes.

Summary of work done — file: `/home/knatte/Code/loomyard/wts/codeintel-daemon-persistence/internal/codeintelcli/cli.go` and `/home/knatte/Code/loomyard/wts/codeintel-daemon-persistence/internal/codeintelcli/cli_test.go`:

- Card 7: added the `"resolution":"complete"` trust marker to `emitLookupResult`'s success branch and `classifyLookupError`'s `statusFound` branch; extended refs/definition `Long` help text to document it.
- Card 8: added `buildOptions(...)` (single `codeintelengine.Options` construction site) and `resolveWorktreeRoot(cwd, targetDir)` (falls back to absolute `--target-dir` outside a lyx hub instead of leaving `WorktreeRoot` empty); replaced all six `Options{...}` literals across refs/definition/symbol's single-arg and batch paths.
- Card 9: added `--in-file <path>` flag to refs and definition (not symbol) and `inFileQuery(inFilePath, name)`; wired both commands' single-arg and batch-mode paths through a local `buildQuery` closure that routes to `inFileQuery` when `--in-file` is set; updated `Long` help with examples.

Commits (all pushed to `codeintel-daemon-persistence`): `0bc92a1a`, `f2dff01a`, `fa779d69`.

{"status":"success","commit_sha":"fa779d69","session_id":"5b31419b-f7e7-4c8f-af32-cdf1a2b9a46c","cards_done":[7,8,9]}
