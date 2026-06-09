MILL_REVIEW_BEGIN
# Review: board-modul (rename fra wiki) + _mhgo-konfigurasjon — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-09
```

## Findings

### [BLOCKING] `wikiPath` survives in exported function parameters

**Location:** `internal/board/sync.go:28`, `internal/board/git.go:61,118,131`, `internal/board/store.go:96`
**Issue:** `Sync(wikiPath string)`, `AtomicWrite(wikiPath, ...)`, `Pull(wikiPath string)`, `CommitPush(wikiPath string, ...)`, and `Store.Save(wikiPath, relPath string)` are exported functions whose parameter is named `wikiPath`; Go docs and IDE completions surface parameter names, placing `wiki` on a developer-facing surface. The `rename-surface` Shared Decision mandates erasing `wiki` from every code- and developer-facing surface.
**Fix:** Rename the `wikiPath` parameter to `boardPath` (or `dir`) in all five exported functions and propagate the rename through the unexported callees (`commitDirty`, `pushUnpushed`, `ensureLockfilesIgnored`, `hasUnpushed`).

### [NIT] Stale `wiki` references in `bench_test.go` comments

**Location:** `internal/board/boardtest/bench_test.go:1,4,5,22,23,174`
**Issue:** File-header and benchmark comments say "core wiki commands", "wiki logic + file I/O", "wiki sizes", and "Wiki facade" — violating the `rename-surface` shared decision at the comment level.
**Fix:** Replace `wiki` with `board` in those comment lines.

### [NIT] `os.Unsetenv` instead of `t.Setenv` in config test

**Location:** `internal/board/config_test.go:196`
**Issue:** `os.Unsetenv("NONEXISTENT_VAR")` is called bare without `t.Setenv` (which saves and restores the original value), breaking test-isolation hygiene if the variable is ever set in the test environment.
**Fix:** Replace `os.Unsetenv("NONEXISTENT_VAR")` with `t.Setenv("NONEXISTENT_VAR", "")` then immediately `os.Unsetenv("NONEXISTENT_VAR")`, or simply rely on the fact that the name is unlikely to collide and document it — but the cleaner fix is to wrap it in a `t.Setenv`-based guard.

### [NIT] Stale local variable name `wikiPath` in `board_test.go`

**Location:** `internal/board/board_test.go:18,71,86`
**Issue:** Test-local variable `wikiPath` was not renamed to `boardPath` (or similar) during the batch 4 rename. Purely cosmetic but inconsistent with the rename-surface intent.
**Fix:** Rename `wikiPath` to `boardPath` in the three test functions.

## Verdict

REQUEST_CHANGES
Exported function parameters `wikiPath` in `git.go`, `sync.go`, and `store.go` violate the `rename-surface` blocking shared decision.
MILL_REVIEW_END
