Summary of work completed for this batch (04-shuttle-told-strings):

- Card 11 (committed as `cc457cb0`): Converted `internal/shuttleengine`'s `Runner` to take told `anchorPath`/`worktreeRoot` strings instead of a `*lyxcwd.Location`, updated `NewRunner`, `runDirRoot`, `FindRun` signatures, dropped the `lyxcwd` import from `run.go`/`rundir.go`, added the doc.go paragraph on told geometry, and updated every caller in the same commit: `internal/websterengine/recoverbatch.go` and `runlevel.go` (pass `deps.Layout.AnchorPath()`), the four CLI construction sites (`burlercli/cli.go`, `perchcli/cli.go`, `webstercli/cli.go`, `shuttlecli/cli.go` — pass `reedGeom.AnchorPath, reedGeom.WorktreeRoot`), and all shuttle/websterengine/webstercli/shuttlecli/treadleengine/burlerengine test fixtures (with distinct anchor/worktree values where the plan calls for swap-detection).
- Card 12 (Commit: none, verification-only): confirmed by grep that `internal/lyxcwd` and `internal/fabricengine` are absent from non-test `.go` files in `tokenvocab`, `reedengine`, `shuttleengine`, and that `tokenvocab/leaf_enforcement_test.go`'s `allowedImports` contains exactly one entry, `internal/stencil`. All three structural facts hold.

Verify command (all sub-invocations) passed cleanly: untagged suite, `-tags integration` suite, and `-tags smoke` vet check.

Both cards declared in this batch are complete (2 of 2).

{"status":"success","commit_sha":"cc457cb0ee95ceff1e6c6494c7483d8af7d9e159","session_id":"deed3cbe-a3f9-48b3-8363-c124b192be50","cards_done":[11,12]}
