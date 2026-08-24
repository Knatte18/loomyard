All 7 cards (4 through 10) of this batch were committed and verified. Summary of files touched:

- `internal/shedrecipe/recipe.go`, `internal/shedrecipe/fixture_test.go` — added the `PlanSpec`/`CommitPlan` Env seams (Card 4)
- `internal/shedrecipe/entries_planwrite.go`, `internal/shedrecipe/entries_planwrite_test.go` — new `planWriteEntry` Constructor and its tests (Card 5)
- `internal/shedrecipe/registry.go`, `internal/shedrecipe/registry_test.go`, `CONSTRAINTS.md`, `manifest/designs/shed-recipe.md` — registered `PlanWrite` as the fourteenth engine (Card 6)
- `internal/shedbuild/fixture_test.go`, `internal/shedbuild/build_engines_test.go` — widened the coverage fixture (Card 7)
- `internal/loomrecipe/fixture_test.go`, `resume_test.go`, `sequence_test.go`, `shape_test.go` — renamed `fakeDiscussionShuttle` → `fakeLoomShuttle`, generalized it to branch on `Spec.Role`, extracted `planFixtureCard`/`planFixtureOverview` (Card 8)
- `contracts/recipes/loom-recipe.yaml`, `internal/loomrecipe/coverage_guard_test.go`, `internal/loomrecipe/shape_test.go` — flipped the `Plan-Write` recipe row to the `PlanWrite` engine (Card 9)
- `internal/loomrecipe/sequence_test.go` — added the `commitPlanCalls == 1` assertion (Card 10)

Batch verify (`go test ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/loomrecipe/...`) passes, and a full `go build ./...` across the repo succeeds. Working tree is clean.

{"status":"success","commit_sha":"bbcf1c5b787a21164ec986cf52ba417b23d8e9ec","session_id":"7c5987cd-8d84-4426-ab44-f9147ce00b00","cards_done":[4,5,6,7,8,9,10]}
