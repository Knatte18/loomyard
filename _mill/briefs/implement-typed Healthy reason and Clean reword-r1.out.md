Both commit subjects match Card 5 and Card 6's `Commit:` messages exactly. 2 of 2 cards committed — all complete.

{"status":"success","commit_sha":"4354279da03c3ad57ffbb9addbcbf84de4f28e7a","session_id":"eb654fde-e391-4386-9284-99ed5470c499","cards_done":[5,6]}

Summary: 2 of 2 cards committed (card 5 and card 6, both matched by commit subject in the post-batch-start log). Verify command `go test -tags integration ./internal/fabricengine/ ./internal/loomengine/` passes. Working tree is clean (only the untracked brief file remains, which is not in scope).

Key files touched:
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/drift.go` — added `HealthCause`/`HealthReason` types, `Healthy` now returns typed reason.
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/loomengine/preflight.go` — switches on `reason.Cause`, adopts `fabricengine.Ready(l)`.
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/loomengine/report.go` — `CheckFabricReady`/`CheckFabricSync` renames.
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/loomengine/status.go` — comment reword.
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/healthreason_integration_test.go` (new) — one case per `HealthCause`.
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/loomengine/preflight_integration_test.go` — CheckID renames, new `TestPreflight_ConfigLoadFailed` equivalence test, `WeftWorktree`→`f.WeftPrime` per requirement (f).
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/{junction_pattern_integration_test.go,reconcile_stale_removal_test.go,config_driven_junctions_integration_test.go,reconcile_stale_registration_test.go,boardjunction_integration_test.go}` — migrated to typed `Cause`/`Detail`.
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/hostclean.go` — `Clean` reason reworded to code-side/state-side.
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/cleanreason_integration_test.go` (new) — three-shape coverage of `Clean`'s reworded reason.

{"status":"success","commit_sha":"4354279da03c3ad57ffbb9addbcbf84de4f28e7a","session_id":"eb654fde-e391-4386-9284-99ed5470c499","cards_done":[5,6]}
