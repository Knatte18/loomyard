Card-count self-check: 1 of 1 cards committed (Card 10, commit `d4716bd`, message matches the batch file's declared `Commit:` exactly). `verify: null` for this batch — no runnable verify was required; the batch's own gate (three clean Tier 1 runs + three Tier 2 runs, all `RESULT: all packages passed`, after `go build ./...`) was executed as specified. Working tree has no uncommitted tracked changes.

One note for the record: the first Tier 2 measurement attempt hit a spurious `internal/muxengine` FAIL (run 3 of the raw attempts). I investigated before treating any run as valid: re-ran `internal/muxengine`/`internal/muxengine/render` in isolation and both passed cleanly, and `git log main..HEAD -- internal/muxengine` showed this task never touched that package — so it was pre-existing contention-driven flakiness on this box, not a regression from batches 1–3. I discarded that run and took a 4th clean Tier 2 run to get three valid `all packages passed` measurements (164.36 s / 207.59 s / 235.81 s, median ~208 s) rather than papering over or hiding the anomaly.

```json
{"status":"success","commit_sha":"d4716bd","session_id":"75428762-e83f-4064-86c6-853eb063a356"}
```

Files touched:
- `C:\Code\loomyard\wts\test-suite-regression\docs\benchmarks\test-suite-timing.md`
- `C:\Code\loomyard\wts\test-suite-regression\docs\benchmarks\running-tests.md`

```json
{"status":"success","commit_sha":"d4716bd","session_id":"75428762-e83f-4064-86c6-853eb063a356"}
```
