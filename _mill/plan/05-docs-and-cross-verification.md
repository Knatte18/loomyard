# Batch: docs-and-cross-verification

```yaml
task: "Optimise and slim the test suite"
batch: "docs-and-cross-verification"
number: 5
cards: 1
verify: go test ./... && go test -race -tags integration -count=1 ./internal/lyxtest/... ./internal/weft/... ./internal/worktree/... ./internal/paths/...
depends-on: [2, 3, 4]
```

## Batch Scope

Capstone: with all three packages migrated, gated, and parallelised, document how to run the two test tiers and record the measured before/after timings + the equivalence-guardrail result, and run the final cross-package verification (offline default loop + `-race` integration suite). Depends on batches 2, 3, 4 so every package is in its final state. One card.

## Cards

### Card 14: document the two tiers + record before/after timings

- **Context:**
  - `docs/benchmarks/board-performance.md`
  - `internal/board/boardtest/doc.go`
  - `internal/lyxtest/doc.go`
  - `internal/weft/sync.go`
  - `internal/worktree/add.go`
- **Edits:**
  - `docs/benchmarks/test-suite-timing.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add a new dated `### 2026-06-… — after optimize-test-suite` block to `test-suite-timing.md` (do not edit prior blocks — the file's convention is one dated block per revision). The block must: (a) document the two tiers — the offline default `go test ./...` (no git subprocesses) and the gated `go test -tags integration ./...` suite, with the exact commands, mirroring `board-performance.md`'s `-tags integration` documentation; (b) record the measured **before/after wall-clock** for `internal/worktree`, `internal/weft`, `internal/paths` (run `go test -tags integration -count=1 ./internal/{worktree,weft,paths}/...` for the "after" integration numbers and `go test -count=1 ./internal/{worktree,weft,paths}/...` for the "after" untagged numbers; cite the prior block's numbers as "before"); (c) state the equivalence-guardrail outcome (post test-name set is a superset of the pre set, listing any intentionally-folded table-driven cases); (d) note whether the success bar was met (default `< ~5s`, integration `< ~45s`) and, per the discussion, treat `< ~5s` as a target confirmed against the measured untagged baseline rather than a hard precondition. Update the "Reducing wall-clock" / floor commentary to reflect the new shared-fixture + parallel reality.
- **Commit:** `docs(benchmarks): record optimize-test-suite tiers and timings`

## Batch Tests

`verify` is intentionally cross-cutting and justified for this capstone: `go test ./...` confirms the **entire** repo's default loop is offline and green (the headline guarantee — no integration-tagged test runs, no git subprocess in the default loop), and `go test -race -tags integration -count=1 ./internal/lyxtest/... ./internal/weft/... ./internal/worktree/... ./internal/paths/...` confirms the four migrated packages' integration suites pass under the race detector with fresh (uncached) runs — the final parallel-safety gate across all the new `t.Parallel()` usage. This batch's deliverable (the timings block) is produced by running exactly these suites, so the verify command and the card's measurement step are the same work.
