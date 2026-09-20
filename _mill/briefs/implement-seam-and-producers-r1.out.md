All 8 cards (9–16) are committed: 7 commits total, since cards 9 and 10 (both touching `internal/burlerengine/engine.go`) were combined into a single commit per the plan's combined-commit allowance. The batch's `verify:` command passed on the final state, and `git status --porcelain --untracked-files=no` is clean.

Card-count self-check: 8 of 8 cards committed (cards 9–16), matching the batch's declared count of 8.

Key files touched this batch:
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/burlerengine/profile.go` — `RunOpts.Gate`
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/burlerengine/engine.go` — `Shuttle.RunGated`, `Result.Gate`, gated `Run`, `repairReportBeforeGate`
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/burlerengine/engine_test.go` — gate test coverage
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/shedadapters/singlellm.go` — gated `Shuttle` seam, `NewSingleLLMProducerGated`, gated `mapOutcome`
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/shedadapters/burler.go` — gated `Call` spawn/attach paths
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/shedadapters/singlellm_test.go`, `internal/shedadapters/burler_test.go` — producer gate-mapping tests
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/shedrecipe/fixture_test.go`, `internal/shedrecipe/entries_bouncer_test.go`, `internal/shedbuild/fixture_test.go`, `internal/loomrecipe/fixture_test.go` — downstream fake seam coverage

{"status":"success","commit_sha":"91a0b7731cc168caa6a99fef20c33283c6a29f26","session_id":"3e1f9530-795a-4873-9067-d51b716c6e00","cards_done":[9,10,11,12,13,14,15,16]}
