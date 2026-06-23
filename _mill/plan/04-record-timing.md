# Batch: record-timing

```yaml
task: "Speed up and stabilize the integration test tier"
batch: record-timing
number: 4
cards: 1
verify: null
depends-on: [1, 2, 3]
```

## Batch Scope

After batches 1-3 land, measure the new Tier 2 wall-clock and record it. This is the
"record a fresh timing history block" deliverable. It also verifies the equivalence
guardrail (the test-name set changed only by the two deleted network tests plus the one
added weft-pushing test) and corrects the now-stale "real GitHub push" annotation in the
timing tables. Doc/measurement only — no code; depends on all three implementation batches.

## Cards

### Card 14: Measure and record the post-change Tier 2 timing block

- **Context:**
  - `_mill/discussion.md`
  - `docs/benchmarks/running-tests.md`
  - `cmd/testtiming/main.go`
- **Edits:**
  - `docs/benchmarks/test-suite-timing.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Run `go run ./cmd/testtiming -full` at least 3 times warm (`-count=1` is
  built in) and take the **median** per-package and wall-clock. Append a new append-only
  history block to `docs/benchmarks/test-suite-timing.md` dated `2026-06-23 — after
  optimize-integration-tier`, containing: (1) the new Tier 2 headline wall-clock and the new
  floor package; (2) a per-package table showing `boardtest` now parallelized (its local git
  tests no longer serial) and `worktree` on the lean fixture; (3) the **equivalence
  guardrail** as a *justified subset* — removed names `TestIntegrationCommitPush` /
  `TestIntegrationPull` map to `git_test.go:TestCommitPush` / `git_test.go:TestPull`, and one
  name was added (`TestWeftSpawnPushesWeftBranch`); (4) a one-line note that the real-GitHub
  network test was deleted (noise source removed). Then update the top "Current best times"
  section: refresh the Tier 2 headline number and change the `internal/board/boardtest` row's
  "Where the Tier 2 cost is" cell so it no longer says "incl. one real GitHub push" (it is now
  local-only, parallelized). Confirm the name-set delta by running
  `go test -tags integration ./... -list '.*'` and checking the two network names are absent
  and `TestWeftSpawnPushesWeftBranch` is present; note the result in the block. Do not invent
  numbers — every figure must come from an actual run on the implementer's machine.
- **Commit:** `docs(benchmarks): record post-optimize-integration-tier Tier 2 timing`

## Batch Tests

`verify: null` — this is a pure documentation/measurement batch. Its "test" is the
`go run ./cmd/testtiming -full` invocations the card performs to source the numbers, which
also serve as a final green-suite check across the whole integration tier; there is no
separate runnable assertion surface, and a fixed `verify:` command would only re-run the
suite a fourth time. The equivalence `-list` diff named in the card is the correctness check.
