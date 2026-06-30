MILL_REVIEW_BEGIN
# Review: Harden the Path Invariant: close enforcement hole + fix geometry leaks

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-30
```

## Findings

### [GAP] lyxtest.go not exempted by the `_test.go` skip
**Section:** Decisions §"enforcement test: AST scan" / Scope "Out: Test-file geometry"
**Issue:** The scan skips only `*_test.go`, but `internal/lyxtest/lyxtest.go` (a non-test source file) builds `filepath.Join(tmpDir, base+"-weft")` at lines 185/475/541 — rule (b) `+`-operand would flag it, breaking the build, yet the discussion explicitly lists `lyxtest.go` as a fixture that must NOT be flagged.
**Fix:** Decide the exemption mechanism (allowlist `internal/lyxtest`, broaden the skip, or route its joins through `paths.WeftSiblingPath`) before planning.

### [NOTE] boardtest impact overstated
**Section:** Testing §"internal/boardengine / internal/boardcli"
**Issue:** `boardtest/concurrency_test.go` and `bench_test.go` construct `boardengine.New(Config{Path: filepath.Join(cwd,"board")})` directly and never call `LoadConfig`, so their board dir does not "move to `paths.BoardDir(hub)`" — no expectation change is needed; only the stale `seedWiki ... path: board` comment is affected.
**Fix:** Correct the testing note so the plan writer doesn't re-resolve these explicit-Path fixtures.

## Verdict

GAPS_FOUND
The `_test.go`-only skip fails to exempt `lyxtest.go`, contradicting stated scope and breaking the converted-tree build.
MILL_REVIEW_END
