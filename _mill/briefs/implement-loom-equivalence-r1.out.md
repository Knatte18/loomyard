Both cards (12 and 13) committed, matching the batch's two `Commit:` messages exactly. 2 of 2 cards committed — all complete.

{"status":"success","commit_sha":"d8f9a6d9a0ff58ca697fb42526b9f1564abf1a10","session_id":"be0e3780-0659-4685-b95f-318afbbc9c4c","cards_done":[12,13]}

Summary of work:

- Created `/home/knatte/Code/loomyard/wts/shed-recipe-loader-builder/internal/shedbuild/testdata/loom-recipe.yaml` — a hand-authored recipe fixture expressing loom's current thirteen-row producer list (card 12, commit `ad51dd74`).
- Created `/home/knatte/Code/loomyard/wts/shed-recipe-loader-builder/internal/shedbuild/equivalence_test.go` — the paired-fixture builder (`newEquivalencePair`) and `TestLoomEquivalence_ThirteenRowsMatch`, which parses/builds the fixture, builds loom's real list via `loomshed.New`, and asserts all thirteen rows agree on `Name`/`OnDone`/`OnStuck`/`Segment`/`MaxBounces` plus concrete producer type via `reflect.TypeOf`, then asserts `Check` reports no findings (card 13, commit `d8f9a6d9`).

Both cards committed via the `git-commit` skill (gofmt/vet clean, no codeguide present in this repo). `go test ./internal/shedbuild/...` passes. Working tree is clean of any in-scope tracked modifications — 2 of 2 cards committed, all complete.

{"status":"success","commit_sha":"d8f9a6d9a0ff58ca697fb42526b9f1564abf1a10","session_id":"be0e3780-0659-4685-b95f-318afbbc9c4c","cards_done":[12,13]}
