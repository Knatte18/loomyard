No dirty tracked files. 4 of 4 cards committed, matching all 4 `Commit:` messages in the batch file. Verify passed.

{"status":"success","commit_sha":"e271dd7c8274b314ca856c351e029b76c9347e46","session_id":"32dd9167-a0f5-49ee-8f2f-8597412693f3","cards_done":[1,2,3,4]}

Summary: All 4 cards in batch `01-engine-fields-and-validation` were completed and committed (4 of 4). `verify: go test ./internal/shedengine/...` passes.

Files touched:
- `/home/knatte/Code/loomyard/wts/shedengine-segments-bounce-budget/internal/shedengine/producer.go` — added `OnDone`, `Segment`, `MaxBounces` fields to `ProducerDef` with doc comments; rewrote struct-level comment.
- `/home/knatte/Code/loomyard/wts/shedengine-segments-bounce-budget/internal/shedengine/shed.go` — rewrote `Shed.MaxBounces` doc comment and file header for the two-level inherited-default semantics, naming the inversion explicitly.
- `/home/knatte/Code/loomyard/wts/shedengine-segments-bounce-budget/internal/shedengine/validate.go` — added `p.MaxBounces < 0` check and `segmentByName` map in the first loop; added `OnDone` existence/self-reference checks and `OnStuck` same-Segment check in the second loop, correctly ordered; extended function doc comment.
- `/home/knatte/Code/loomyard/wts/shedengine-segments-bounce-budget/internal/shedengine/validate_test.go` — added failing cases for negative `ProducerDef.MaxBounces`, `OnDone` naming no producer, `OnDone` self-reference, `OnStuck` cross-Segment; passing cases for forward `OnDone`, self-referencing `OnStuck`, and same-Segment `OnStuck`.

Commits (oldest to newest): d2967e8b, ed434619, eb457838, e271dd7c — all pushed to `origin/shedengine-segments-bounce-budget`.

Pre-existing `golangci-lint` findings in `run.go`/`run_persist_test.go` (unchecked `Release()` error returns) are outside this batch's scope (files not touched by any card) and were left as-is.

{"status":"success","commit_sha":"e271dd7c8274b314ca856c351e029b76c9347e46","session_id":"32dd9167-a0f5-49ee-8f2f-8597412693f3","cards_done":[1,2,3,4]}