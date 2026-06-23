I have enough to produce findings.

MILL_REVIEW_BEGIN
# Review: weft producers: _lyx/config, lyx config, codeguide — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-23
```

## Findings

### [BLOCKING] Batch DAG verify names a non-existent path
**Location:** 00-overview.md batches block (batch 1) + 01 card 4
**Issue:** Batch 1 `verify: go test ./internal/board/ ./internal/worktree/ ./internal/weft/` but no test files currently exist in `internal/weft/` or `internal/worktree/` outside the integration-tagged `weft_test.go`; running untagged `go test ./internal/weft/` is fine, but `./internal/worktree/` has ONLY `//go:build integration` test files, so an untagged run compiles zero tests there (passes vacuously) — acceptable — however the new `worktree/template_test.go` (card 2/4) is untagged and will run. This is fine; the real risk is `weft` package currently has no non-integration test and the card adds one. No blocker here on reflection.
**Fix:** None needed — withdraw; see corrected findings below.

### [BLOCKING] e2e sync test cannot use the injected commit cleanly
**Location:** 04 card 14 (e), `internal/weft/cli.go`
**Issue:** The injected sync `weft.RunCLI(io.Discard, []string{"commit"})` resolves its own cwd via `paths.Getwd()`+`Resolve` and re-loads weft config; the card relies on `t.Chdir` into the host worktree, but `weft commit` commits via `Commit(weftWorktree, pathspec, envSyncOptions())` where `envSyncOptions()` reads `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` — if either is set in the test env the commit becomes a no-op and the "tracked/committed in weft" assertion fails silently.
**Fix:** Card 14 must state the e2e explicitly unsets `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` (and must not `t.Parallel`, since it uses `t.Chdir`).

### [BLOCKING] editOne sync failure message swallows the actual error
**Location:** 04 card 11 (`editOne`)
**Issue:** On `sync()` non-zero the card prints a fixed `edited ... but weft sync failed` to `out`, but `weft.RunCLI` sends its real error JSON to `io.Discard` and its usage/unknown errors to `os.Stderr`, so the operator gets a code-1 with no diagnosable cause for the sync failure.
**Fix:** Require `editOne` to capture the sync writer (not `io.Discard`) or surface a captured stderr/message so the failure is diagnosable, not a bare sentence.

### [NIT] Abort message inaccurate after fresh scaffold
**Location:** 04 card 11 (`editOne`) / 03 card 8 (`Edit`)
**Issue:** When `Edit` scaffolds a missing file then the editor errors, `Edit` returns `ErrAborted` but a default-commented file now exists; `editOne` prints `aborted: _lyx/config/<module>.yaml left unchanged`, which is false (it was just created).
**Fix:** Reword the abort message to not assert "left unchanged", or have `Edit` remove a just-scaffolded file on abort.

### [NIT] weft.ConfigTemplate parse round-trip claim is loose
**Location:** 01 card 3 / card 4(b)
**Issue:** Card requires the uncommented `pathspec` line to "round-trip through `config.Load(..., "weft", ...)` to `_lyx`", but `config.Load` needs a real `<baseDir>/_lyx/` and reads `_lyx/config/weft.yaml`; the simpler card-4(b) assertion (uncomment + `yaml.Unmarshal` → `_lyx`) is what is actually testable without a fixture.
**Fix:** Drop the `config.Load` round-trip phrasing from card 3; keep the yaml.Unmarshal assertion in card 4(b).

### [NIT] Batch 1 verify runs vacuously for worktree integration files
**Location:** 00-overview.md batch 1 verify
**Issue:** `go test ./internal/worktree/` (untagged) skips the `//go:build integration` files; only the new untagged `template_test.go` runs there — correct but worth confirming the relocation guard for worktree truly lives in the untagged template test (it does, card 4a).
**Fix:** None required; informational.

## Verdict

REQUEST_CHANGES
e2e sync env-skip hazard and swallowed sync-failure diagnostics must be fixed.
MILL_REVIEW_END