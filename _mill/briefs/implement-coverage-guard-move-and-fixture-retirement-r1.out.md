All 3 cards (12, 13, 14) declared in this batch are committed — 3 of 3 cards committed, matching the batch's declared card count.

Summary of work:
- `/home/knatte/Code/loomyard/wts/loom-convert-to-shed-recipe/internal/loomrecipe/coverage_guard_test.go` — moved via `git mv` from `internal/shedrecipe/coverage_guard_test.go`, repackaged as `package loomrecipe`, repointed onto this package's own `New` via `shape_test.go`'s `testEnv(t)`, row table rekeyed off `loomshed.Name*` constants, and given the new fourth orphan-check half against `shedrecipe.Names()`.
- `/home/knatte/Code/loomyard/wts/loom-convert-to-shed-recipe/internal/shedrecipe/registry_test.go` — `TestRegistry_ShipsTwelveEntries` re-homed here verbatim.
- `/home/knatte/Code/loomyard/wts/loom-convert-to-shed-recipe/internal/shedbuild/equivalence_test.go` and `/home/knatte/Code/loomyard/wts/loom-convert-to-shed-recipe/internal/shedbuild/testdata/loom-recipe.yaml` — deleted; directory removed.

Verify command `go test ./internal/loomrecipe/... ./internal/shedrecipe/... ./internal/shedbuild/...` passed. `go build ./...` and `go vet ./...` both clean. Working tree is clean (no uncommitted tracked changes).

{"status":"success","commit_sha":"ddaf4dec6886050be2bd0ec2011f9ac594c2cdc3","session_id":"c37de519-b86a-40ba-8228-165826b7dcdb","cards_done":[12,13,14]}
