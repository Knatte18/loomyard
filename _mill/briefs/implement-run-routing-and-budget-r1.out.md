7 commits match all 7 declared cards (5–11) exactly. All 7 of 7 cards committed, verified with a full green `go test ./internal/shedengine/...` run.

{"status":"success","commit_sha":"3bd6133401aeb4287281d351dd80042c8f572277","session_id":"086a8a90-f86b-410b-9c52-879e09834bbc","cards_done":[5,6,7,8,9,10,11]}

Summary: All 7 cards (5–11) in `_mill/plan/02-run-routing-and-budget.md` are complete and committed — 7 of 7 cards committed, this is genuinely all done for this batch.

Key files touched:
- `/home/knatte/Code/loomyard/wts/shedengine-segments-bounce-budget/internal/shedengine/run.go` — Done now routes via `OnDone`, `indexAfter` deleted, `findProducer` narrowed to `(ProducerDef, bool)`, `bouncesRemaining` replaced by `episodeStuckCount`/`effectiveMaxBounces` derived from `st.History`.
- `internal/shedengine/testsupport_test.go` — added `linearChain` helper.
- `internal/shedengine/run_routing_test.go`, `run_pause_test.go`, `run_persist_test.go` — re-wired onto explicit `OnDone` chains; added new tests for forward/backward/terminal `OnDone` routing and for episode-scoped budget properties (independence, inheritance, cross-invocation derivation, episode reset, attribution, block-path arithmetic, failure-path terminator).

`go test ./internal/shedengine/...` passes. `internal/loomshed` is deliberately left red per the plan's batch-local decision (batch 3 migrates it).

{"status":"success","commit_sha":"3bd6133401aeb4287281d351dd80042c8f572277","session_id":"086a8a90-f86b-410b-9c52-879e09834bbc","cards_done":[5,6,7,8,9,10,11]}