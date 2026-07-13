1 of 1 card committed (Card 4), matching the batch file's single-card declaration.

Summary of work done for this batch (benchmarks-block, batch 2 of the "Restore the Tier 1 floor" task):

- Ran `go build ./...` to warm the cache, then 3 warm `go run ./cmd/testtiming` runs and 3 warm `go run ./cmd/testtiming -full` runs. All 6 runs reported `RESULT: all packages passed`.
- Tier 1 median: 9.95 s (spread 9.33–12.84 s), down from the superseded block's ~29 s (~66% faster).
- Tier 2 median: 131.7 s (spread 124.0–132.5 s), essentially flat vs. the superseded block's ~128 s (this task's levers don't touch `internal/warpengine`, still the Tier 2 floor at ~96 s this run).
- Edited `C:\Code\loomyard\wts\restore-tier1-floor\docs\benchmarks\test-suite-timing.md`: retitled the 2026-07-13 hermetic-git-env "Current best times" block into a frozen History entry (`### 2026-07-13 — hermetic git test environment (was "Current best times")`, content unchanged, internal headings demoted from `###` to `####` to match History nesting), and wrote a new "Current best times" section dated 2026-07-13 with descriptor "restore-tier1-floor: mousetrap disabled + lingering-child test re-tiered" — Headline table, Cause section (mousetrap 8.0s→0.46s, lingering-child re-tier ~12s moved, boardtest writes-shrink kept ~8.1s→0.45s), an explicit supersession note correcting the earlier cmd/lyx-guards/perchengine-tables attribution, a note on the tierpurity-guard evasion (no guard change), and the Tier 1/Tier 2 where-the-time-goes and slowest-10 tables from the median runs.
- Committed via the `git-commit` skill as `40e56fd` and pushed to `origin/restore-tier1-floor`.
- Ran the batch's `verify: go test ./... -count=1` — all 52 packages passed, no failures.
- Confirmed no tracked in-scope modifications remain (`git status --porcelain --untracked-files=no` empty).

{"status":"success","commit_sha":"40e56fd","session_id":"79930f3a-b204-46ec-b4ea-1c6d17b6a6a8"}
