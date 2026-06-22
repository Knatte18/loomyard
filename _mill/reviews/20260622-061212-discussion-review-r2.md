Confirmed: the timing doc uses dated `### YYYY-MM-DD` blocks with `#### Before / after wall-clock`, `#### Equivalence guardrail` (superset note), and `#### Parallel safety` subsections. The discussion's instruction to append a new dated block mirroring the 2026-06-21 format is accurate and grounded.

I have verified all the major load-bearing claims. Let me synthesize. The discussion is thorough and round-1 gaps were resolved. The main substantive issue I found is the inaccurate characterization of the ide tests' spawn mechanism, which underpins the gate-only scope decision for ide. Let me also note the boardtest "integration/smoke gated" imprecision.

MILL_REVIEW_BEGIN
# Review: Optimise and slim the rest of the test suite

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-22
```

## Findings

### [GAP] ide tests mischaracterised as binary/TUI spawners
**Section:** Scope (In) / Technical context (Files to gate)
**Issue:** The discussion says `ide/cli_test.go` + `menu_test.go` "spawn the binary / drive the TUI via `exec.Command`", but verified both invoke the SUT (`RunCLI`, `Menu`) in-process and use `exec.Command` only to spawn **git** for repo/worktree fixtures (`git init`/`config`/`commit`/`worktree add`) — the exact per-test-git-spawn pattern lyxtest exists to eliminate.
**Fix:** Correct the mechanism description; the "no lyxtest-ification of ide, gate in place" decision rests on a false premise (they don't spawn the binary) — re-justify gate-only vs. lyxtest-fixture migration, or note ide spawns git (not the binary) and that `menu_test.go` uses `t.Setenv("BOARD_SKIP_GIT","1")` so it stays serial.

### [NOTE] boardtest is not wholly integration-gated
**Section:** Technical context (boardtest)
**Issue:** Discussion calls `internal/board/boardtest` "`integration`/`smoke` gated"; verified `bench_test.go` and `concurrency_test.go` are **untagged** (no-git, `BOARD_SKIP_GIT=1`) and run in the default loop — only `integration_test.go`/`bench_git_test.go` carry `//go:build integration`.
**Fix:** State that boardtest mixes untagged (no-git) and integration-gated files; confirm the moved gated files land alongside untagged ones without affecting Tier 1 (they only compile under `-tags integration`).

### [NOTE] git_test.go BOARD_SKIP_PUSH usage is partial
**Section:** Technical context (Board git seam)
**Issue:** Discussion implies `git_test.go` toggles `BOARD_SKIP_PUSH` at :110/:154/:229; verified `TestPull` and the "rebase retry" subtest do NOT skip push (`:229` sets `BOARD_SKIP_PUSH=""`, i.e. push enabled) — so not every moved git test is push-skipping/local-only.
**Fix:** Note `TestPull` does a real `push -u origin master` and the rebase-retry subtest pushes; only two subtests set `BOARD_SKIP_PUSH=1`. Affects fixture choice (these need a working upstream, reinforcing CopyWeft fit) and the local/offline framing.

## Verdict
GAPS_FOUND
ide scope decision rests on a mischaracterised spawn mechanism; resolve before plan writing.
MILL_REVIEW_END