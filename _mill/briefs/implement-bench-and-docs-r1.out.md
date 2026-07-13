Both cards for the `bench-and-docs` batch are fully committed, verify passes, and the working tree is clean.

**Card-count self-check:** 2 of 2 cards committed (Card 11: `7b8feb5 test(lyxtest): permanent CopyPaired/CopyPairedLocal benchmarks`; Card 12: `ed346e0 docs(benchmarks): fixture-copy analysis + hermetic-env before/after timing`). Additionally, two plan-edit commits (`c946238`, `3fa6189`) and one discovered-scope fix commit (`3555490`) were made per the brief's protocol, since recording an honest official Tier 1/Tier 2 timing run (required by Card 12) surfaced a real pre-existing compile break in `internal/buildercli` caused by a same-task commit (batch 1), which had to be fixed before a green suite could be measured.

Summary of what changed:
- `C:\Code\loomyard\wts\faster-git-fixture-tests\internal\lyxtest\bench_test.go` (new) — permanent `BenchmarkCopyPaired`/`BenchmarkCopyPairedLocal` (serial + `b.RunParallel`), integration-tagged.
- `C:\Code\loomyard\wts\faster-git-fixture-tests\docs\benchmarks\fixture-copy.md` (new) — ported discussion-phase benchmark report + Reproducing section with fresh output.
- `C:\Code\loomyard\wts\faster-git-fixture-tests\docs\benchmarks\test-suite-timing.md` — new 2026-07-13 "Current best times" block (Tier 1 ~29s, Tier 2 ~128s, was ~208s; warpengine floor ~152s → ~84s), prior block demoted to History unchanged.
- `C:\Code\loomyard\wts\faster-git-fixture-tests\docs\benchmarks\running-tests.md` — refreshed Tier 2 figures, added Hermetic Git Test Environment Invariant pointer.
- `internal\buildercli\testdata_test.go` (new, untagged), `internal\buildercli\pause_spawnbatch_test.go` (new, integration-tagged), and edits to `poll_test.go`, `status_test.go`, `pause_test.go`, `validate_test.go` — fixed a Tier 1 (`go test ./...`) compile break in `internal/buildercli` that batch 1's mechanical `//go:build integration` tagging introduced but no prior verify command (all pass `-tags integration`) could catch. No test assertions changed; helpers/tests were relocated by whether they actually spawn git.
- `_mill\plan\04-bench-and-docs.md` — two plan-edit commits recording the discovered-scope extension before the code changes, per protocol.
- Also removed (untracked, gitignored) `.scratch/fixbench/` debris that was tripping `internal/hubgeometry`'s geometry-literal enforcement guard during the Tier 1 run; not a git change since it was never tracked.

Both tiers verified green: `go test ./...` and `go test -tags integration ./...` both pass repo-wide (confirmed via 3 warm `cmd/testtiming` runs each, used for the recorded medians). Batch verify command (`go test -tags integration -run '^$' -bench BenchmarkCopy -benchtime 1x -count=1 ./internal/lyxtest`) passes.

{"status":"success","commit_sha":"ed346e0","session_id":"02a495ef-d25d-4555-9531-e5c82dc410fd"}
