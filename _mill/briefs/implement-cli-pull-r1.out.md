1 of 1 card committed (card 11), the second commit (4c3a4a17) is a same-turn follow-up fix folded into the same card, not a separate card. All work complete for this batch.

{"status":"success","commit_sha":"4c3a4a177f962cfc9e51873aae0fa96d1f8934b6","session_id":"7fc21325-684b-4f68-ad47-376ecda198c2","cards_done":[11]}

Summary: 1 of 1 card committed (Card 11). `internal/fabriccli/weft_verbs.go` — `pullCmd`'s RunE now calls `fab.Pull(fabricengine.EnvSyncOptions())` instead of `fab.PullWeft`, surfaces the `PullResult` via a new `pullResultMap` helper into the existing `map[string]any` JSON envelope, and `Short`/`Long` were rewritten to describe the both-sides pull/reconcile behaviour including the double-conflict and no-surviving-anchor abort cases. `internal/fabriccli/cli_test.go` — added `TestRunCLI_PullHelp` (asserts `--help` documents the both-sides/reconcile/rewrite behaviour) and `TestRunCLI_PullShortNonEmpty` (asserts `pullCmd.Short` is non-empty and mentions reconcile, via the `Command()` seam). Verify command `go test -tags integration ./internal/fabriccli/` passes. Working tree is clean; both commits pushed to `fabric-rebase-reconcile`.

{"status":"success","commit_sha":"4c3a4a177f962cfc9e51873aae0fa96d1f8934b6","session_id":"7fc21325-684b-4f68-ad47-376ecda198c2","cards_done":[11]}
