3 of 3 cards committed — all complete. Card 9, Card 10, and Card 11 all have matching commit subjects in the log, confirming full completion of this batch.

Summary of work:
- `/home/knatte/Code/loomyard/wts/loom-plan-review-producer/internal/loomshed/loomshed.go` — replaced `NamePlanReview` with `NamePlanBouncer`, `NamePlanBurler`, `NamePlanRevalidate`.
- `/home/knatte/Code/loomyard/wts/loom-plan-review-producer/contracts/recipes/loom-recipe.yaml` — replaced the stubbed `Plan-Review` row with the `Plan-Bouncer`/`Plan-Burler` perch plus `Plan-Revalidate`.
- `/home/knatte/Code/loomyard/wts/loom-plan-review-producer/internal/loomrecipe/fixture_test.go` — added `loom-rubric-plan-review` to `seedBouncerStencils`, added `corruptPlanOverview` to `fakeLoomBurler`, updated doc comments.
- `/home/knatte/Code/loomyard/wts/loom-plan-review-producer/internal/loomrecipe/coverage_guard_test.go`, `shape_test.go`, `recipe_test.go`, `sequence_test.go` — updated row tables/counts from 14→16 rows, updated `TestSequence_FullRunBlocksAtPublish` counters (`commitPlanCalls`=2, `bouncerJudgeCalls`=2, `loomBurler.calls`=2).
- `/home/knatte/Code/loomyard/wts/loom-plan-review-producer/internal/loomshed/stub.go`, `doc.go` — retargeted doc comments to Webster-Review alone.
- `/home/knatte/Code/loomyard/wts/loom-plan-review-producer/internal/loomrecipe/revalidate_test.go` (new) — proves Plan-Revalidate catches a post-segment format regression and bounces to Plan-Write.

All commits pushed; `verify: go build ./... && go test ./internal/loomshed/... ./internal/loomrecipe/... ./internal/shedbuild/...` passes.

{"status":"success","commit_sha":"8bfe15710c7b351570a505c3c2a41618d31e3c19","session_id":"4f8077f2-d219-4df6-b809-faa9ea999b1d","cards_done":[9,10,11]}
