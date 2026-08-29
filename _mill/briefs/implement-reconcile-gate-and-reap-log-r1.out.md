{"status":"success","commit_sha":"b5d165ebf97505c340532ca0407a26cfbb22406c","session_id":"c3c73c59-00ca-412a-9d3d-378a81b4469d","cards_done":[1,2,3]}

All 3 of 3 cards in this batch are committed. Card 1 (the `reconcilePlan` struct refactor) and Card 2 (the `headerAlive` disjunct) were combined into one commit (`ddb95b14f`, named per Card 2's message, since both touched the same lines of `reconcile.go`/`reconcile_test.go` and doing them separately would have meant writing an intermediate shape nothing consumes). Card 3 (the reap log line + `logcapture_test.go`) is its own commit (`b5d165ebf`). The batch's `verify:` command passed with all cases green, and the working tree has no uncommitted tracked changes.

Key files touched:
- `/home/knatte/Code/loomyard/wts/reed-pane-reap-consistency/internal/reedengine/reconcile.go` — `reconcilePlan` struct, `headerAlive` disjunct, and the `defer`-based `logger.Info` reap log line in `reconcileLocked`.
- `/home/knatte/Code/loomyard/wts/reed-pane-reap-consistency/internal/reedengine/reconcile_test.go` — rewritten `TestPlanReconcile` table (new struct fields, new `headerAlive` cases) and the new `TestReconcileLocked_LogsTheUntrackedPanesItReaps` test.
- `/home/knatte/Code/loomyard/wts/reed-pane-reap-consistency/internal/reedengine/logcapture_test.go` — new `captureLogOutput` test helper.

{"status":"success","commit_sha":"b5d165ebf97505c340532ca0407a26cfbb22406c","session_id":"c3c73c59-00ca-412a-9d3d-378a81b4469d","cards_done":[1,2,3]}
