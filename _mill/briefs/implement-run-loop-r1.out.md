All 7 declared cards (7–13) are committed — count confirmed honestly by matching commit subjects against the batch's `## Cards` `Commit:` messages. Both verify sub-invocations passed.

{"status":"success","commit_sha":"51327e2efff83d223dcbedda24b1a045c80ed461","session_id":"1ab171c9-afa4-44a1-bec6-436942246ed8","cards_done":[7,8,9,10,11,12,13]}

Summary: all 7 cards of batch `run-loop` (cards 7-13) are committed — 7 of 7 complete, not a partial claim. Key files:

- `/home/knatte/Code/loomyard/wts/shed/internal/shedengine/run.go` — `(*Shed).Run`, `findProducer`, `(*Shed).persist`, `nowRFC3339`, `indexAfter`
- `/home/knatte/Code/loomyard/wts/shed/internal/shedengine/testsupport_test.go` — `funcProducer`, `fixedOutcomeProducer`, `newTestShed`, `seedStatus`, `readStatus`, `commonSeed`, `assertRFC3339UTC`, `assertHistoryNonDecreasing`
- `/home/knatte/Code/loomyard/wts/shed/internal/shedengine/run_routing_test.go` — happy path, completion terminal values, unconditional re-call, stuck+OnStuck, stuck+no-target, bounce-budget exhaustion, MaxBounces=0 default, producer error, unrecognised outcome
- `/home/knatte/Code/loomyard/wts/shed/_mill/plan/02-run-loop.md` — extended card 11's `Edits:` to include `testsupport_test.go` (discovered mid-implementation: `seedStatus` needed to create the status lock's parent directory since `state.WriteJSON` only creates the status path's own parent, not the lock path's)

Both verify sub-invocations passed: `go test ./internal/shedengine/...` and `go test -run 'TestTierPurity_|TestHermeticGitEnv_' ./cmd/lyx/`. Working tree is clean (confirmed via `git status --porcelain --untracked-files=no`).

{"status":"success","commit_sha":"51327e2efff83d223dcbedda24b1a045c80ed461","session_id":"1ab171c9-afa4-44a1-bec6-436942246ed8","cards_done":[7,8,9,10,11,12,13]}
