7 of 7 cards committed (cards 17-23), matching the batch's declared 7 cards. All committed, verify passed, working tree clean.

{"status":"success","commit_sha":"fb51fa871f61ccd542e03b85f7af23a2620d3bb0","session_id":"51be76e3-c135-4e3c-a4d3-3c980926c63b","cards_done":[17,18,19,20,21,22,23]}

Summary: All 7 cards of batch `specs-dir-marker-plumbing` completed and committed (7 of 7 - full completion, verified honestly against the batch's declared card list).

Files touched (all absolute paths under `/home/knatte/Code/loomyard/wts/deploy-specs-like-stencils`):
- `internal/shedadapters/rubric.go` (new), `internal/shedadapters/rubric_test.go` (new), `internal/shedadapters/bouncer.go`
- `internal/shedrecipe/recipe.go`, `entries_bouncer.go`, `entries_burler.go`, `fixture_test.go`, `entries_bouncer_test.go`, `entries_burler_test.go`
- `internal/loomrecipe/fixture_test.go`, `internal/loomrecipe/shape_test.go`
- `internal/loomengine/plan.go`, `internal/loomengine/plan_test.go` (`internal/loomengine/config_test.go` needed no change - it never calls `PlanSpec`/`composePlanPrompt`)
- `internal/websterengine/render.go`, `beginbatch.go`, `recoverbatch.go`, `template_test.go`
- `internal/loomcli/wiring.go`

The full batch verify command passed:
`go test ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/loomrecipe/... ./internal/loomengine/... ./internal/websterengine/... ./internal/loomcli/... ./internal/burlercli/... ./internal/burlerengine/...`

`go build ./...` also passes cleanly across the whole repo. Working tree is clean (no uncommitted tracked changes). Pre-existing `gofmt`/`goimports` drift exists in a handful of unrelated files (`internal/lyxcwd/docslink_test.go`, `internal/shedengine/run_commitstatus_test.go`, `internal/shell/posix.go`, `internal/shuttlecli/cli_test.go`, `internal/webstercli/verbs_test.go`, `internal/websterengine/beginbatch_test.go`/`recordbatch_test.go`/`recoverbatch_test.go`/`strand_test.go`) - none of these were touched by this batch and none are in this batch's declared file scope, so they were left alone.

{"status":"success","commit_sha":"fb51fa871f61ccd542e03b85f7af23a2620d3bb0","session_id":"51be76e3-c135-4e3c-a4d3-3c980926c63b","cards_done":[17,18,19,20,21,22,23]}