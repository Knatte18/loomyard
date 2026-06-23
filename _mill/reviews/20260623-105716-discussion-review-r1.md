Both referenced docs exist. I have verified all the load-bearing claims. Let me compose the review.

Summary of verification:
- Workstream A: integration_test.go and bench_git_test.go confirmed, both use testRepoURL; git_test.go covers the logic locally. Solid.
- Workstream B: all 4 env consumption sites confirmed; Config has the 4 fields; concurrency_test.go uses board.New(cfg). Solid, with a minor env-fallback subtlety under parallelism.
- Workstream C: **FLAW** — host push at add.go:172 is unconditional; SkipPush only gates the weft push. Host bare is NOT dead weight.
- One observation: concurrency_test.go has no `//go:build integration` tag.

MILL_REVIEW_BEGIN
# Review: Speed up and stabilize the integration test tier

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-23
```

## Findings

### [GAP] Host bare is not dead weight under SkipPush
**Section:** Scope C / Decision C
**Issue:** The premise "with `SkipPush:true` the push never runs, so the bare repos are pure dead weight" is false — `add.go:172` does an unconditional host `git push -u origin <branch>` (step 13); `SkipPush`/`SkipGit` only gate the *weft* push (`weft.go:203`, `add.go:21` doc: "disables only the weft-branch push"). The happy-path `Add(SkipPush:true)` tests in `add_test.go`/`weft_test.go`/`remove_test.go` assert `err==nil`, so they require the host bare to exist.
**Fix:** Restate C: only the **weft-bare** can be dropped from `CopyPaired`'s lean variant; the host bare must stay. Recompute the I/O claim — cutting one of four repos (not two) is ~25%, not "roughly halves."

### [NOTE] Env-fallback ambient leakage under t.Parallel
**Section:** Decision B / Testing (workstream B)
**Issue:** With "skip if flag OR env" semantics retained, a parallel-converted test that sets no flag (e.g. `sync_test.go:newSyncRepo` currently does `t.Setenv("BOARD_SKIP_GIT","")` precisely to neutralize ambient env) will honour an ambient `BOARD_SKIP_GIT=1` once `t.Setenv` is removed, silently no-opping the Sync it means to exercise.
**Fix:** Note that flag-converted tests must pass the flag explicitly (e.g. `cfg.SkipGit=false`) AND state how ambient env is neutralized, since `t.Setenv` (the current guard) is being removed.

### [NOTE] concurrency_test.go is not integration-tagged
**Section:** Decision B / Technical context (boardtest)
**Issue:** `concurrency_test.go` has no `//go:build integration` constraint, so it is a Tier 1 test, yet it is listed among the boardtest local tests to convert in B. Its `BOARD_SKIP_GIT` env usage and goroutine fan-out are unaffected by the integration tag.
**Fix:** Acknowledge that B touches a Tier-1-tagged file; confirm the conversion (env→`cfg.SkipGit`) is still in scope there even though it does not affect Tier 2 wall-clock.

## Verdict
GAPS_FOUND
Decision C rests on a false dead-weight premise; the host bare is a live push target.
MILL_REVIEW_END