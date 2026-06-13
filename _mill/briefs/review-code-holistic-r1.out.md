Good — "Open questions" is gone; replaced with "Resolved decisions". The plan requirement is met.

Now let me do a final summary of what I've found:

1. **`mustRun` bug** — `cmd.Run()` at line 19 then `cmd.CombinedOutput()` at line 21 in `helpers_test.go`. `CombinedOutput()` starts the process and must not be called after `Run()`. The output will always be empty, making test failures hard to diagnose. This should use `cmd.CombinedOutput()` or capture stderr/stdout before calling `Run()`. This causes incorrect test diagnostics but doesn't break test pass/fail outcome (the fatalf still fires). Severity: NIT (diagnostics-only impact, doesn't affect test correctness).

2. **`list.go` `Main` flag assignment** — `Main: blockIdx == 0` uses the raw block index from `strings.Split`, so if git ever emits output with a leading empty block, the first non-empty block would incorrectly get `Main: false`. In practice, git's porcelain output always starts with the main worktree (no leading blank), so this is latent. Severity: NIT.

3. **`links.go` path construction with `/`** — `fullPath := dir + "/" + entry.Name()` at line 20. On Windows, this works because Windows accepts forward slashes in paths at Go/OS API level. However, the canonical Go way is `filepath.Join(dir, entry.Name())`. Not a functional bug on Windows with Go, but non-idiomatic. Severity: NIT.

4. **`cli_test.go` redundant `defer t.Chdir`** — Each test function has a `defer func() { t.Chdir(filepath.Dir(hub)) }()` before calling `t.Chdir(hub)`. Since `t.Chdir` already registers cleanup to restore the directory, this defer is redundant and could interfere (it changes cwd during test cleanup). Severity: NIT.

5. **`worktree.md` stale naming convention** — Line 46-47 still describes the old naming convention "named after their branch name with `/` replaced by `-`" which contradicts the implemented behavior (dir = slug only). Card 19 explicitly said "keep the rest of the document intact," so the implementer followed the plan, but the resulting doc is self-contradictory. Severity: NIT (doc inconsistency, no code impact).

None of the findings are BLOCKING. The implementation is architecturally sound, all plan cards are realized, shared decisions are consistently applied, cross-batch contracts hold, and test coverage is adequate.

MILL_REVIEW_BEGIN
# Review: Build mhgo worktree module — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-13
```

## Findings

### [NIT] mustRun calls CombinedOutput after Run — output always empty
**Location:** `C:\Code\mhgo\wts\mhgo-worktree-module\internal\worktree\helpers_test.go:19-22`
**Issue:** `cmd.Run()` is called first (line 19), then `cmd.CombinedOutput()` on the same already-run command (line 21); `CombinedOutput` returns an error and empty bytes because the process already exited, so failure messages always print blank output.
**Fix:** Replace `cmd.Run()` + deferred `cmd.CombinedOutput()` with a single `out, err := cmd.CombinedOutput()` and fatalf on non-nil `err`.

### [NIT] list.go Main flag uses raw block index, not first-non-empty index
**Location:** `C:\Code\mhgo\wts\mhgo-worktree-module\internal\worktree\list.go:64`
**Issue:** `Main: blockIdx == 0` relies on the raw `strings.Split` index; if git ever emits a leading blank block, the skip-continue path would leave `blockIdx == 0` consumed by an empty block, marking the first real entry as `Main: false`.
**Fix:** Track a separate `firstBlock bool` that flips to false after the first non-empty block is appended, or reset `Main` based on `len(entries) == 0` at append time.

### [NIT] links.go uses string concatenation instead of filepath.Join
**Location:** `C:\Code\mhgo\wts\mhgo-worktree-module\internal\worktree\links.go:20`
**Issue:** `fullPath := dir + "/" + entry.Name()` uses a hardcoded forward slash instead of `filepath.Join`; not a functional bug on Windows (Go passes forward slashes through), but non-idiomatic and inconsistent with the rest of the codebase.
**Fix:** `fullPath := filepath.Join(dir, entry.Name())` (add `path/filepath` import).

### [NIT] cli_test.go redundant defer t.Chdir before t.Chdir call
**Location:** `C:\Code\mhgo\wts\mhgo-worktree-module\internal\worktree\cli_test.go:17-19`, `:70-72`, `:114-116`
**Issue:** Each test registers `defer func() { t.Chdir(filepath.Dir(hub)) }()` immediately before `t.Chdir(hub)`; since Go 1.21 `t.Chdir` already registers its own cleanup to restore the prior directory, the manual defer is redundant and fires an extra cwd change during teardown.
**Fix:** Remove the three `defer func() { t.Chdir(...) }()` blocks; rely on `t.Chdir`'s built-in cleanup.

### [NIT] worktree.md naming-convention paragraph contradicts implemented behaviour
**Location:** `C:\Code\mhgo\wts\mhgo-worktree-module\docs\modules\worktree.md:46-47`
**Issue:** The "Naming conventions" section still states worktrees are "named after their branch name with `/` replaced by `-` (e.g. `hanf/my-task` → `hanf-my-task`)", which is the rejected design; the shipped implementation uses slug-only directory names.
**Fix:** Update line 46-47 to document the shipped convention: directory = slug only; branch = `<branch_prefix><slug>` (matching the discussion.md decision).

## Verdict

APPROVE
All plan cards realised; shared decisions consistently applied; four NITs, zero blocking issues.
MILL_REVIEW_END
