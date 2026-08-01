# Batch: cli-pull

```yaml
task: 'fabric: warp-rebase / remote-reconcile recovery'
batch: cli-pull
number: 3
cards: 1
verify: go test -tags integration ./internal/fabriccli/
depends-on: [2]
```

## Batch Scope

This batch extends the existing `lyx fabric pull` subcommand (`internal/fabriccli/weft_verbs.go`) in place to drive the unified `Fabric.Pull` instead of the weft-only `PullWeft`, surfacing the `PullResult` through the existing JSON output envelope, and rewrites the command's `Short`/`Long` help to describe the new both-sides behaviour (a CLI/Cobra Invariant review obligation — stale help is review-blocking). No new verb name is introduced (`unified-pull-dispatch`, and deliberately NOT `reconcile`, which already names host↔weft topology repair in `fabric.go`). It depends on batch 2 for `Fabric.Pull`/`PullResult`.

## Cards

### Card 11: Extend `fabric pull` to drive Fabric.Pull

- **Context:**
  - `internal/fabricengine/pull.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabricengine/weftgit.go`
- **Edits:**
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabriccli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabriccli/weft_verbs.go`, change the `pullCmd` `RunE` (currently `fab.PullWeft(fabricengine.EnvSyncOptions())`) to call `fab.Pull(fabricengine.EnvSyncOptions())`. On error, keep the existing pattern: `clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))` and return nil (this covers `*PartialPullError`, `ErrWarpDivergedUnpushed`, and `ErrNoSurvivingAnchor` uniformly, since all are `error` values). On success, surface the `PullResult` via `output.Ok(out, ...)` as a `map[string]any` mirroring the struct's fields (`weft_pulled`, `warp_fetched`, `warp_advanced`, `new_warp_head`, `rewrite_detected`, `reconciled`, `anchor_warp_sha`, `anchor_weft_sha`, `reanchor_weft_sha`, and `pattern_residue` as a list of `{weft_sha, paths}` maps) — matching the `map[string]any` style the `status`/`commit` verbs already use, so the pull result reaches the caller through the same one-JSON-object-per-line envelope. Update `pullCmd.Short` from `"pull weft changes from remote"` to something naming the both-sides behaviour (e.g. `"pull warp and weft, reconciling a rebased warp"`), and add a `Long` describing: weft is fast-forwarded first, then warp is fetched and inspected; a clean warp fast-forward advances warp; a detected warp history rewrite with a clean local warp is auto-reconciled by re-anchoring weft's correspondence to the nearest surviving `Warp-SHA` and writing a new anchor commit; the result reports which post-anchor weft commits touch `_pattern/` and need review; and the double-conflict case (local unpushed warp commits + a diverged remote) and the no-surviving-anchor case both abort loudly with no changes. Do NOT rename the verb or reuse `reconcile`. In `internal/fabriccli/cli_test.go` (which is already `//go:build integration`-tagged), add or update a test asserting `pullCmd`'s `Short` is non-empty and its `Long` mentions the both-sides/reconcile behaviour (a help-accuracy assertion in the same style as the file's existing command-surface tests, building the command tree via the module's `Command()` seam — it needs no git spawn but rides this file's existing integration tag). Do not add a separate untagged file and do not re-drive `Fabric.Pull`'s runtime behaviour here (that is covered by batch 2's engine integration matrix).
- **Commit:** `feat(fabriccli): drive unified Fabric.Pull from fabric pull`

## Batch Tests

`verify: go test -tags integration ./internal/fabriccli/` runs the fabriccli integration suite (the `-tags integration` flag is required because `cli_test.go`, where the help-accuracy assertion lives, is integration-tagged), including the help-accuracy assertion for the extended `pull` verb. Command registration and the "`Short` on every command" guard are unaffected (no command added/removed) and are covered by the whole-repo `go test ./...` done-gate. `Fabric.Pull`'s runtime behaviour is verified by batch 2's engine integration matrix, not re-driven here.
