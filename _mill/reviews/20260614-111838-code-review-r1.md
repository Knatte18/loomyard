MILL_REVIEW_BEGIN
# Review: Extend worktree module: portals and launchers — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-14
```

## Findings

### [BLOCKING] Out-of-plan file: internal/paths/helpers_test.go

**Location:** `internal/paths/helpers_test.go`
**Issue:** This file is present on disk but appears in neither the `00-overview.md` "All Files Touched" manifest nor any batch's Creates/Context/Edits list; the plan discipline was skipped for it.
**Fix:** Add `internal/paths/helpers_test.go` to the overview's "All Files Touched" list and to the batch-1 Creates section.

---

### [BLOCKING] Broken `contains` helper silences all launcher content assertions

**Location:** `internal/worktree/launchers_test.go:201`
**Issue:** The `contains(s, substr string) bool` implementation returns `true` whenever both inputs are non-empty (`len(s) > 0 && len(substr) > 0` is always true), making every assertion in `TestWriteLaunchers` that calls `contains(content, "mhgo ide menu")` or `contains(content, "subdir\\nested")` a no-op that never actually validates the file content.
**Fix:** Replace the body with `return strings.Contains(s, substr)` (stdlib `strings` package).

---

### [BLOCKING] vscode.go writes nested titleBar map; VS Code requires flat dot-notation keys

**Location:** `internal/ide/vscode.go:37-44`, `internal/ide/color.go:100-105`
**Issue:** The plan's Shared Decision specifies `workbench.colorCustomizations = {"titleBar.activeBackground": color, "titleBar.activeForeground": "#ffffff", ...}` (flat dot-notation keys inside the object), which is how VS Code actually resolves title-bar colors. Both `vscode.go` and `color.go` instead use a nested `{"titleBar": {"activeBackground": color}}` map, so the written `settings.json` will not apply any color in VS Code, and the `color_test.go` tests use the same broken format so they pass without catching the deviation.
**Fix:** In `vscode.go`, rewrite the `workbench.colorCustomizations` value as `map[string]any{"titleBar.activeBackground": color, "titleBar.activeForeground": "#ffffff", "titleBar.inactiveBackground": color, "titleBar.inactiveForeground": "#ffffffaa"}`. In `color.go`, read back the flat key `colorCustomizations["titleBar.activeBackground"]`. Update `color_test.go` to write settings using the flat key format.

---

### [NIT] portals.go and launchers.go export functions the plan designates unexported

**Location:** `internal/worktree/portals.go:14,24`, `internal/worktree/launchers.go:27,90`
**Issue:** The plan (batch 3, cards 7 and 8) specifies `createPortal`, `removePortal`, `writeLaunchers`, `removeLaunchers` as unexported helpers called by `Add`/`Remove`; the implementation exports them as `CreatePortal`, `RemovePortal`, `WriteLaunchers`, `RemoveLaunchers`, expanding the package's public API beyond the design.
**Fix:** Lowercase the function names; move the test assertions for these helpers into `package worktree` (internal test file) so they can access unexported symbols without export.

---

## Verdict

REQUEST_CHANGES
Two blocking correctness bugs (broken test helper, wrong VS Code JSON structure) and one out-of-plan file must be resolved before approval.
MILL_REVIEW_END
