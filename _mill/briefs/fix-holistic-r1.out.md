HEAD (08fc1a94e0ed28ee33029c67dcc17bcf97a488a4) differs from the baseline (630d99f1cdd7a5dffd4f06a9216211297d92b44c), no uncommitted tracked changes, and both verify commands passed.

**Finding processed:** [BLOCKING:consistency] Stale `selvageAlive` references survive the rename — VERIFY: accurate (confirmed via grep). HARM CHECK: none. Action: FIX. Swept all 10 occurrences across `internal/reedengine/reconcile.go`, `internal/reedengine/doc.go`, `internal/reedengine/reconcile_test.go`, `internal/reedengine/spawn_test.go`, replacing `selvageAlive` with `policy.authorizesReap()` in comments.

Files touched:
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/reconcile.go`
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/doc.go`
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/reconcile_test.go`
- `/home/knatte/Code/loomyard/wts/reed-selvage-pane-extraction/internal/reedengine/spawn_test.go`

Commit: `08fc1a94e0ed28ee33029c67dcc17bcf97a488a4`, pushed to `reed-selvage-pane-extraction`.

{"status":"success","commit_sha":"08fc1a94e0ed28ee33029c67dcc17bcf97a488a4","session_id":"bb0655ac-2312-4aac-8eb1-74d5cd2623a0"}
