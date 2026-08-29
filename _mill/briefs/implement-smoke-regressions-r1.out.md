All 6 cards (13-18) match commits since the batch-start commit `957b85440`. All 6 of 6 cards committed, verify passed.

{"status":"success","commit_sha":"e1460824cbc86a91cd96f0db17aee9ab55b67a6a","session_id":"67e6e76e-0fc6-4ba5-aa37-6d8789a967ac","cards_done":[13,14,15,16,17,18]}

Summary: all 6 cards for the smoke-regressions batch are committed (6 of 6), and the batch's `verify:` command (`go test -tags smoke -timeout 20m ./internal/reedcli/ -run '...'`) passed cleanly — all named tests pass except `TestSmokeRemoveLastStrandThenAddRunsTheNewCommand`, which correctly skips on this Linux host (Windows-only by design, listed in the filter only to catch compile breaks).

Files touched:
- `/home/knatte/Code/loomyard/wts/reed-pane-reap-consistency/internal/reedcli/smoke_lifecycle_test.go` — rewrote `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable` and `TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile`/`TestSmokeRemoveLastStrandThenAddRunsTheNewCommand` to drop stale adoption framing; added `TestSmokeForeignPaneIsReapedNotAdoptedByAdd` (M16) and `TestSmokeUpAfterScrubbedStateLeavesOnlyTheRebuiltHeader` (M22), plus a shared `pollProcessGone` helper; corrected `TestSmokeUpSurvivesAScrubbedStateFileWhileTheSessionIsUp`'s doc comment to note its `pane_top == 0` assertion is now vacuous.
- `/home/knatte/Code/loomyard/wts/reed-pane-reap-consistency/internal/reedcli/smoke_panecwd_test.go` — restated `TestSmokeStrandPaneSpawnsAtToldAnchorNotProcessCwd`'s two-case contrast now that both strands are splits.
- `/home/knatte/Code/loomyard/wts/reed-pane-reap-consistency/internal/reedcli/smoke_teardown_test.go` — dropped the stale "(adopts the initial pane)" parenthetical.
