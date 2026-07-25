MILL_REVIEW_BEGIN
# Review: dev/test lyx.exe separated from production deploy — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-25
```

## Findings

### [BLOCKING] Guard test trips cmd/lyx tier-purity guard
**Location:** Batch 3 / Card 13 (and Batch 4 / Card 15)
**Issue:** `tools/sandbox/pathresolve_guard_test.go` must contain the literal `exec.Command("lyx"` / `exec.CommandContext("lyx"` as its scan tokens; `cmd/lyx/tierpurity_test.go` walks the whole module and bans the raw substring `exec.Command` in any untagged `*_test.go`, so `go test ./cmd/lyx/` will fail — and no batch verify (`go test ./tools/sandbox/`) nor the overview `go build ./tools/...` catches it. Existing sandbox tests contain no such literal, so this file is a new violation.
**Fix:** Add `tools/sandbox/pathresolve_guard_test.go` to `allowedSpawners` in `cmd/lyx/tierpurity_test.go` (with a reason) as an explicit card Edit; list that file in All Files Touched.

### [NIT] devbin API referenced without Context entry
**Location:** Batch 1 / Cards 3 and 4
**Issue:** Both cards' Requirements call `devbin.RepoRoot()` / `devbin.Dir()` but neither lists `tools/internal/devbin/devbin.go` in `Context:` (it is created in Card 1 of the same batch); Card 4's test even compares against `devbin.Dir()` directly.
**Fix:** Add `tools/internal/devbin/devbin.go` to the `Context:` of Cards 3 and 4.

## Verdict

REQUEST_CHANGES
One blocking cross-guard failure the plan's verification would miss; otherwise sound.
MILL_REVIEW_END
