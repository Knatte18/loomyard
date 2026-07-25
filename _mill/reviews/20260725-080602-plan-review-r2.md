MILL_REVIEW_BEGIN
# Review: dev/test lyx.exe separated from production deploy — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-25
```

## Findings

### [BLOCKING] Guard file trips the Hermetic Git Env guard
**Location:** Batch 3, Card 13
**Issue:** `pathresolve_guard_test.go` carries the literal `exec.Command("lyx"` / `exec.CommandContext("lyx"` scan strings; `cmd/lyx/hermeticenv_test.go`'s `gitSpawnTokens` includes `exec.Command` and walks the whole module, so it marks `tools/sandbox` git-spawning with no `HermeticGitEnv` token → `go test ./cmd/lyx/` (batch verify) fails. The card adds the `tierpurity` `allowedSpawners` entry but not the parallel `allowedNonHermetic` entry.
**Fix:** Also add a file-level `allowedNonHermetic` entry for `tools/sandbox/pathresolve_guard_test.go` in `cmd/lyx/hermeticenv_test.go`, mirroring the existing self-exclusion entries.

### [BLOCKING] resolve.go / devbin.go absent from Context though their symbols are used
**Location:** Batch 2 Cards 5–6; Batch 3 Cards 7–11
**Issue:** Cards 6,7,8,9,10,11 reference `resolveLyx`/`prependPath`/`sourceDev`/`sourceProd` (all in `tools/sandbox/resolve.go`) and Card 5 references `devbin.BinPath` (`tools/internal/devbin/devbin.go`), but none list those files in `Context:` or `Edits:` — a same-batch created file is not implicitly readable, forcing cold-start exploration for exact signatures/error strings.
**Fix:** Add `tools/sandbox/resolve.go` to Context of Cards 6–11 and `tools/internal/devbin/devbin.go` to Context of Card 5.

## Verdict

REQUEST_CHANGES
Two blockers: unhandled hermetic-guard collision and missing Context entries for resolve.go/devbin.go.
MILL_REVIEW_END
