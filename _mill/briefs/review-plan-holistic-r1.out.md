I have verified the plan comprehensively against the source. Let me summarize my findings.

Key verifications completed:
- Card 1: `TestInit_FirstRun` hardcodes `len(result.Modules) != 3` (lines 62-64); `Init` → `configsync.ReconcileAll` maps 1:1 to `configreg.Modules()` which now returns 7 modules — so the registry-derived fix is correct and turns the red package green.
- Card 2: `hubgeometry.BoardDir(l.Hub)` = `<Hub>/_board` matches the board dir the three red menu tests create; `boardcli/cli.go:103` is exactly the referenced pattern; `TestMenuHardErrorOnMissingBoard` fails earlier at `LoadConfig`, so it stays green.
- Cards 3-8: every test-move enumeration matches the actual test/helper set in each file, spawn-free remainders are genuinely token-free, and import-trim lists are accurate.
- Card 9 (the load-bearing one): I cross-referenced all 52 banned-token `*_test.go` files against their build tags. Every one is either tagged `integration`/`smoke`, inside batch-2's 5 packages (retagged/moved), or under `internal/proc` (allowlisted). So the guard passes after batch 2 with zero uncovered spawners — no DAG/guard gap.

MILL_REVIEW_BEGIN
# Review: Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-12
```

## Findings

### [NIT] "spawn-free" invariant/doc text overclaims
**Location:** Card 9 (CONSTRAINTS statement) + Card 10 (running-tests.md wording)
**Issue:** Untagged tests that reach `hubgeometry.Resolve` still spawn `git rev-parse` (e.g. perchcli `TestRunCLI_Pause_MissingRunID`, muxcli `TestRunCLI_NotAGitRepo`, configcli `TestReconcile_NotAGitRepo`, the `cmd/lyx` dispatch-to-warp/ide/weft/config tests all stay untagged yet spawn); the guard's token list deliberately excludes that cheap call, so "untagged test files spawn no processes / offline tier is spawn-free" is literally inaccurate.
**Fix:** Word the invariant and doc as "no expensive spawns — no `git init`/`worktree add`/fixture-tree copies," matching what the token guard actually enforces.

### [NIT] Card 9 Context omits crosscompile_test.go
**Location:** Batch 3 / Card 9
**Issue:** Requirements says resolve the module root "exactly as `crosscompile_test.go` does" (its `go env GOMOD` + `os.DevNull` idiom is load-bearing), but that file is not in `Context:`, forcing cold-start exploration.
**Fix:** Add `cmd/lyx/crosscompile_test.go` to Card 9's `Context:`.

### [NIT] Card 9 Windows path-sep normalization implicit
**Location:** Batch 3 / Card 9
**Issue:** `filepath.WalkDir` yields backslash paths on this Windows-primary repo; matching them against the "slash-separated module-relative" `allowedSpawners` prefixes (`internal/proc`) without normalizing would silently fail, falsely tripping the guard on the primary dev OS's `go test ./...`.
**Fix:** State explicitly that the walked relative path is `filepath.ToSlash`-normalized before allowlist-prefix / skip-dir matching.

## Verdict

APPROVE
Plan is well-grounded, complete, and guard-coverage is exhaustive; only minor wording/context NITs remain.
MILL_REVIEW_END
