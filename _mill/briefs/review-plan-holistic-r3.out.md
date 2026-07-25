MILL_REVIEW_BEGIN
# Review: dev/test lyx.exe separated from production deploy — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-25
```

## Findings

### [BLOCKING] Card 12 misses launchAgent stub update in main_test.go
**Location:** Batch 3, Card 12 (edits `tools/sandbox/main_test.go`)
**Issue:** Card 7 changes the `launchAgent` seam to 4 args `(hostRepoDir, claudePath, instruction, binDir string)`, but `main_test.go` assigns seven 3-arg `launchAgent = func(dir, claude, instruction string) int` stubs (lines 379, 434, 487, 556, 609, 677, 736) and Card 12's Requirements only mention `cloneRun`/`decideClone` — so batch-3 verify `go test ./tools/sandbox/` fails to compile (type mismatch).
**Fix:** Extend Card 12 to update every `launchAgent` stub in `main_test.go` to the 4-arg signature (and stub `devBinPath` to a non-existent path in the suite-routing tests so they keep resolving `sourceProd`); add `tools/sandbox/suite.go` and `tools/sandbox/resolve.go` to Card 12's `Context` so the implementer can see the new signature and the `devBinPath` seam.

### [NIT] prependPath PATH-name casing on Windows
**Location:** Batch 2, Card 5 (`prependPath`)
**Issue:** `prependPath` matches the `PATH=` entry, but `os.Environ()` on Windows (the primary dev OS) commonly returns `Path=...`; a case-sensitive match misses it and appends a duplicate `PATH=` entry.
**Fix:** Specify case-insensitive matching of the variable name (compare the pre-`=` key uppercased) so the existing `Path`/`PATH` entry is edited in place rather than duplicated.

## Verdict

REQUEST_CHANGES
Card 12 must update main_test.go's launchAgent stubs or batch 3 fails to compile.
MILL_REVIEW_END
