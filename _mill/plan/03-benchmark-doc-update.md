# Batch: benchmark-doc-update

```yaml
task: Audit and overhaul engine test suites
batch: benchmark-doc-update
number: 3
cards: 1
verify: go test ./... && go test -tags integration ./...
depends-on: [1, 2]
```

## Batch Scope

Re-measures the whole suite's wall-clock now that both fixes (batches 1 and 2) are in place, and appends a new dated block to `docs/benchmarks/test-suite-timing.md`'s `## Linux baseline` section recording the before/after — per `_mill/discussion.md`'s "Benchmark doc update" Decision, following the doc's own established per-fix block convention (Machine/Method/Headline/Cause), since that doc is the artifact that documented the problem this task fixes. Depends on both batch 1 and batch 2 because the measurement is only meaningful once both real-time-wait tests are actually shrunk — measuring after only one fix would produce a number that doesn't match either the "before" or the true "after" state. No source code is touched in this batch; it is a pure documentation update driven by empirical re-measurement.

## Cards

### Card 4: re-measure and append the after-fix benchmark block

- **Context:**
  - `docs/benchmarks/test-suite-timing.md`
  - `_mill/discussion.md`
- **Edits:**
  - `docs/benchmarks/test-suite-timing.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Confirm machine identity before measuring: run `go version` and `nproc` and compare against the existing `### 2026-08-01 — githubclient + webstercli now the floor` block's stated machine line ("AMD Ryzen AI 7 445 w/ Radeon 840M, Ubuntu 26.04 LTS, `linux/amd64`, 12 logical CPUs", Go 1.26.0). If this is the same machine (same core count, same or compatible Go version), reuse that Machine: line in the new block, updating the Go version if it has changed. If the measured facts differ, gather fresh ones and state them instead — never copy the old block's Machine: line without checking it still applies.
  - Follow the doc's own documented method exactly: `go build ./...` first to warm the build cache, then median of 3 warm runs of `go run ./cmd/testtiming` (Tier 1) and median of 3 warm runs of `go run ./cmd/testtiming -full` (Tier 2), matching the "Method:" line format every existing block in this file uses.
  - Append a new block under `## Linux baseline`, positioned immediately above the current `### 2026-08-01 — githubclient + webstercli now the floor` block (newest-first ordering, per the file's own "Append-only... Newest first" convention stated in its `## History (trend log)` section). Title the new block `### 2026-08-01 — timeout/window seams shrunk` (a distinct descriptive suffix from the existing same-dated block, per `_mill/discussion.md`'s "Benchmark doc update" Decision — do not reuse the bare date as a heading, since that would collide with the existing `### 2026-08-01 — githubclient + webstercli now the floor` heading).
  - The new block must follow the same internal structure every other block in this file uses: a Machine/Go-version/Method preamble, a `#### Headline` table (Tier 1 / Tier 2 rows, wall-clock, and a comparison column against the immediately-preceding `2026-08-01 — githubclient + webstercli now the floor` block's numbers), and a `#### Cause` prose section explaining the delta — attribute it explicitly to this task's two fixes (the `ghAuthTokenTimeout` var-seam shrinking `TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout` from ~5s to ~10ms, and the `--wait 1ns` flag shrinking `TestAwaitBatchCmd_ReportPresenceEnvelope/NoReport_WindowElapses` from ~30s to near-instant), mirroring how the existing `2026-08-01` block's own Cause section named the two tests as the cause of the prior growth.
  - Expect (sanity-check, not a hard requirement to hit exactly) Tier 1 to drop back toward roughly the pre-regression `2026-07-13` Linux floor (~1.03s) and Tier 2 to drop back toward roughly that same block's Tier 2 floor (~4.97s), since both fixes remove the two dominant real-time waits the `2026-08-01 — githubclient + webstercli now the floor` block identified as "almost entirely" responsible for the prior growth. If the measured numbers deviate substantially from that expectation, investigate before writing the block rather than recording a number that doesn't match the fixes just made — the whole point of this card is an accurate before/after record, not a matching-expectation record.
  - Do not edit or remove any earlier block in the file — this file is append-only; every existing block (including the one being superseded) stays frozen exactly as it is, per the file's own documented convention ("Append-only: each block is the state at that revision and is frozen").
- **Commit:** `docs(benchmarks): record post-fix Linux timing after shrinking the githubclient/webstercli waits`

## Batch Tests

`go test ./...` (Tier 1) and `go test -tags integration ./...` (Tier 2) both run as this batch's `verify:`, confirming `RESULT: all packages passed` end-to-end across the whole repo now that both fixes have landed — this is the full-suite confirmation `_mill/discussion.md`'s Testing section calls for ("run full `go test ./...` ... and `go test -tags integration ./...` ... to confirm `RESULT: all packages passed` and no regressions elsewhere"), and it doubles as part of the empirical re-measurement this card's own doc update needs. No test file is edited in this batch (the card's only edit is the doc), so there is no narrower test-file scope to point `verify:` at instead.
