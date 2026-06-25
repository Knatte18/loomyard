MILL_REVIEW_BEGIN
# Review: Introduce warp: the host↔weft-coordinated git module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-25
```

## Findings

### [BLOCKING] internal/worktree package not deleted — Batch 3 Card 10 not executed

**Location:** `internal/worktree/` (entire directory)

**Issue:** Batch 3 Card 10 ("Move worktree test suite into warp and delete the package") explicitly required deleting all files under `internal/worktree` and verifying `go build ./...` compiles with no dangling references. The directory still contains 21 Go files: the production source files (`add.go`, `cli.go`, `config.go`, `list.go`, `launchers.go`, `portals.go`, `prune.go`, `remove.go`, `template.go`, `template.yaml`, `weft.go`, `worktree.go`) and old test files (`add_test.go`, `cli_test.go`, `config_test.go`, `list_test.go`, `launchers_test.go`, `portals_test.go`, `remove_test.go`, `weft_test.go`, plus integration-tagged ones). The test files still import `"github.com/Knatte18/loomyard/internal/worktree"` as an external test package (`package worktree_test`).

Consequences:
- `go build ./...` compiles two live implementations of the worktree lifecycle: `package worktree` and `package warp`.
- The old package's config tests call `LoadConfig(tmpDir, "worktree")` — referencing the removed `worktree` module name, not `warp`.

**Fix:** Execute Card 10 exactly as specified: move every `internal/worktree/*_test.go` file into `internal/warp` under the mapped names, update package clauses, rewrite `package worktree_test` external imports to `internal/warp`, then delete the entire `internal/worktree` directory. Update the test consumers per Card 10. Confirm `go build ./...` has no `internal/worktree` references.

---

### [BLOCKING] warp-checkout.cmd launcher missing branch argument

**Location:** `internal/warp/launchers.go` (the `writeLaunchers` function; `launchers_test.go` asserts `"lyx warp checkout\r\n"` with no branch)

**Issue:** `writeLaunchers` emits `warp-checkout.cmd` with content `@cd /d "%~dp0<path>" && lyx warp checkout\r\n` — no branch argument. `runCheckout` in `warp.go` unconditionally requires `len(args) >= 1` and returns a JSON error `"usage: lyx warp checkout <branch>"` when called with zero args. The launcher is therefore non-functional.

**Fix:** Either (a) make `runCheckout` handle zero args by performing an in-place re-checkout of the current branch (resolves branch from `git branch --show-current`), or (b) update `writeLaunchers` and its test to emit a branch placeholder. Option (a) preferred.

---

### [NIT] Stale internal/worktree references in doc comments

**Location 1:** `internal/lyxtest/doc.go:2` — "test suites across internal/worktree, internal/weft, and internal/paths"
**Location 2:** `internal/lyxtest/doc.go:9` — leaf invariant prose still says `worktree`
**Location 3:** `internal/paths/paths.go:344` — "seeders in internal/worktree."

**Fix:** Update to name `internal/warp` instead of `internal/worktree`.

---

### [NIT] Misleading comment in TestCleanup_ApplySkipsProtectedBranch

**Location:** `internal/warp/cleanup_test.go` (approx line 99–104)

**Issue:** Comment says "deletes a non-task orphan" but the test asserts `Protected=true` and `Deleted=false`.

**Fix:** Replace with an accurate description.

---

### [NIT] Inconsistent usage-string prefixes across RunCLI verb handlers

**Location:** `internal/warp/warp.go` — `runAdd`, `runRemove`, `runCheckout`

**Issue:** `runAdd` emits `"usage: warp add <slug>"` and `runRemove` `"usage: warp remove [--force] <slug>"` (no `lyx` prefix), while `runCheckout` emits `"usage: lyx warp checkout <branch>"`.

**Fix:** Standardise to canonical `lyx warp <verb>` form.

---

## Verdict

REQUEST_CHANGES

MILL_REVIEW_END
