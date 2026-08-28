{"status":"success","commit_sha":"7d5d926f9896c90698814aa388d66bdc210d51df","session_id":"b4837eeb-ba0e-4b7e-a748-06bf14e8c6cf","cards_done":[6,7,8,9,10,11,12,13,14]}

Summary: all 9 cards (6-14) of batch 02-engine-reapply-op were implemented, tested, and committed — 9 of 9 cards committed, no partial work. Verify (`go test ./internal/reedengine/...`) passes clean.

Files touched:
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/windowsize.go` — widened `liveBoxLocked`, extended `pinGeometryOptionsLocked` with the hook lifecycle, added `removeResizeSignalFileLocked`.
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/apply.go` — added `applyOpts`/`applyResult`/`applyLayoutLockedOpts`, reduced `applyLayoutLocked` to a wrapper.
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/lock.go` — added `withTryOpLock`.
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/reapply.go` — new file: `ReapplyResult`, `hookInstalledLocked`, `reapplyLayout`.
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/lifecycle.go` — boot-path `watchdogOption` validation.
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/apply_test.go`, `windowsize_test.go`, `lock_test.go`, `lifecycle_test.go` — extended.
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/reapply_test.go` — new file.

{"status":"success","commit_sha":"7d5d926f9896c90698814aa388d66bdc210d51df","session_id":"b4837eeb-ba0e-4b7e-a748-06bf14e8c6cf","cards_done":[6,7,8,9,10,11,12,13,14]}
