Good. I have comprehensive coverage now. Producing the final review.

MILL_REVIEW_BEGIN
# Review: Build builder - the batch-implementation loop — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-11
```

## Findings

### [BLOCKING] `--restart-chain` refused by the very report it exists to clear
**Location:** `internal/builderengine/spawn.go:210-227`
**Issue:** `SpawnBatch` stats `reportPath` and returns `"batch report already exists"` (lines 215-217) *before* it ever reaches the `opts.RestartChain` branch (lines 221-227) that calls `RestartChain` to delete that same stale report. The orchestrator template (`orchestrator-template.md`, "A stuck chain member" bullet) and the module doc both instruct `spawn-batch <NN> --restart-chain` on the *same* `NN` that just reported `stuck` — i.e. the batch whose report is present. Every real invocation of this feature therefore fails with the pre-existing-report error, never reaching the reset it exists to perform. The error message itself ("remove it (or let --restart-chain clear it)") contradicts the code's own ordering. `spawn_test.go`/`chain_test.go`/buildercli's `spawnbatch_test.go` never combine a pre-existing report with `RestartChain: true`, so this is untested at every layer.
**Fix:** Reorder so the restart-chain reset (and its report deletion) runs before the stale-report check, or skip the pre-existing-report check entirely when `opts.RestartChain` is true.

### [BLOCKING] Global utility duplication: private builderengine helpers reimplemented in buildercli
**Location:** `internal/builderengine/poll.go:121-161` (`turnEnded`, `strandLive`) vs `internal/buildercli/poll.go:48-83` (`pollTurnEnded`, `pollStrandLive`); `internal/builderengine/spawn.go:124-126` (`batchReportFileName`) vs `internal/buildercli/status.go:23-29` (duplicate `batchReportFileName`)
**Issue:** Batch 4 (poll.go) and batch 5 (spawn.go) leave these three helpers unexported, and batch 7 (buildercli) then reimplements each one byte-for-byte instead of exporting and reusing them -- buildercli's own comments admit this ("reimplemented here since that helper is package-private"). This is exactly the review criteria's named BLOCKING case: two batches independently carrying the same logic. A future change to the event-Stop-scan or the report filename convention has to be made twice and can silently drift.
**Fix:** Export `TurnEnded`/`StrandLive`/`batchReportFileName`-equivalent from `builderengine` (or an internal package both can import) and delete the buildercli-side copies.

### [NIT] `state.CurrentBatch` not reset to 0 after poll's terminal classification
**Location:** `internal/buildercli/poll.go:226-232`
**Issue:** state.go's own doc says `CurrentBatch` is "0 when none is [in flight]," but poll only sets `bs.Terminal`/`bs.Status` and saves -- `st.CurrentBatch` keeps pointing at the just-finished batch until the next `spawn-batch` overwrites it. `lyx builder status` called in that window reports a misleading `current_batch`.
**Fix:** Set `st.CurrentBatch = 0` alongside `bs.Terminal = true` before `SaveState`.

### [NIT] Sandbox scenario hand-writes a plan instead of copying the plan-valid fixture
**Location:** `tools/sandbox/SANDBOX-CORE-SUITE.md:297-362`
**Issue:** Card 31 specified copying the `plan-valid` testdata fixture into the sandbox worktree; the scenario instead inlines a fresh trivial one-batch plan. Functionally equivalent coverage, but a literal deviation from the card's instruction and forgoes exercising the fixture's richer chain/oversized shapes end-to-end via the CLI.
**Fix:** None required functionally; note for future alignment between plan text and sandbox content.

## Verdict

REQUEST_CHANGES
`--restart-chain`'s core recovery path is unreachable due to a check-ordering bug; fix that and the exported-helper duplication before landing.
MILL_REVIEW_END
