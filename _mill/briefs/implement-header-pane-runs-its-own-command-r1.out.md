{"status":"success","commit_sha":"bef2380d206a0f04ef9153f0016b131ffdcdb73a","session_id":"533ea1fa-1a09-498d-93f4-2af6b92cc169","cards_done":[1,2,3,4,5]}

All 5 of 5 cards in batch `01-header-pane-runs-its-own-command` are committed and pushed. `go test ./internal/reedengine/` passes in full.

Summary of work:
- `/home/knatte/Code/loomyard/wts/reed-header-pane-boot-noise/internal/reedengine/lock.go`: added unexported `Engine.suppressHeaderLaunch` field, initialised from `testing.Testing()` in `New`.
- `/home/knatte/Code/loomyard/wts/reed-header-pane-boot-noise/internal/reedengine/lifecycle.go`: `ensureHeaderPaneLocked` now computes `launchCmd` before splitting and passes it into `splitHeaderPaneAtTopLocked`/`splitPaneAboveLocked`, which append it as `split-window`'s trailing shell-command argument (both the first attempt and the even-vertical retry); the old `send-keys -l` / `send-keys Enter` pair is gone.
- `/home/knatte/Code/loomyard/wts/reed-header-pane-boot-noise/internal/reedengine/lifecycle_test.go`: added `enableHeaderLaunch` test helper plus `TestEnsureHeaderPaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys` (P1, confirmed red before card 3 and green after), `TestEnsureHeaderPaneLocked_DefaultUnderGoTestSplitsACommandlessShell`, and `TestEnsureHeaderPaneLocked_RetriedSplitAlsoCarriesTheLaunchCommand`.
- `/home/knatte/Code/loomyard/wts/reed-header-pane-boot-noise/internal/reedengine/doc.go`: updated the header-pane paragraph to describe the direct-command boot and the resulting working corpse-and-heal contract.

Final HEAD: `bef2380d206a0f04ef9153f0016b131ffdcdb73a`.

{"status":"success","commit_sha":"bef2380d206a0f04ef9153f0016b131ffdcdb73a","session_id":"533ea1fa-1a09-498d-93f4-2af6b92cc169","cards_done":[1,2,3,4,5]}
