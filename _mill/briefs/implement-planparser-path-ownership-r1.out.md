Both card commits are present, matching the batch's 2 declared cards exactly. 2 of 2 cards committed — all complete.

{"status":"success","commit_sha":"067c435aa337018407d4b21ab0c19ecf69d8d7cf","session_id":"c85a01f6-1213-44c0-9704-ac9c5896fa32","cards_done":[1,2]}

Summary: 2 of 2 cards committed (all complete). Files touched:
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/internal/planparser/parse.go` — added `PlanDir(anchorPath string) string` and `PlanOverview(anchorPath string) string` after `PlanDirRel`.
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/internal/planparser/planpath_test.go` — new untagged unit test (`TestPlanDir`, `TestPlanOverview`), written before the implementation to confirm compile failure first.
- `/home/knatte/Code/loomyard/wts/planparser-plan-dir/internal/planparser/doc.go` — added a "Path ownership" doc section.

Verify command `go test ./internal/planparser/...` passes. Working tree is clean; both commits pushed to `planparser-plan-dir`.

{"status":"success","commit_sha":"067c435aa337018407d4b21ab0c19ecf69d8d7cf","session_id":"c85a01f6-1213-44c0-9704-ac9c5896fa32","cards_done":[1,2]}
