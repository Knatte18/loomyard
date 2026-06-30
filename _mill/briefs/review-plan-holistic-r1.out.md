MILL_REVIEW_BEGIN
# Review: Rename internal/paths to internal/hubgeometry — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-30
```

## Findings

### [BLOCKING] Verify never compiles the integration test files it edits
**Location:** Batch 1 verify (`go build ./... && go test ./...`); cards 4, 6, 7
**Issue:** `configcli_integration_test.go`, `clone_integration_test.go`, and `weft_integration_test.go` carry `//go:build integration`; `go build` skips all `_test.go` and untagged `go test ./...` excludes them, so a botched `paths.`→`hubgeometry.` retarget in these three in-scope files ships green and undetected.
**Fix:** Append a tagged compile to the batch-1 verify, e.g. `go test -tags integration ./...` (or at minimum `go vet -tags integration ./...`), so the edited integration files are validated.

### [NIT] Card 10 loom.md instruction misses the line-60 reference
**Location:** Batch 2, card 9/10 (docs/modules/loom.md)
**Issue:** The card only calls out the line-256 anchor + trailing `internal/paths`, but loom.md also names `internal/paths` at line 60 ("cwd/Hub/Prime via `internal/paths`"), which the explicit instruction omits.
**Fix:** Instruct replacing every `internal/paths` in loom.md (line 60 included), not just the line-256 occurrence.

### [NIT] Card 10 overview.md sweep scoped to the invariant section only
**Location:** Batch 2, card 10 (docs/overview.md)
**Issue:** `internal/paths` also appears outside the "Path Invariants" section — the directory tree (line 177) and the shared-modules list (line 242) — but the card frames the edit as "body prose" of the renamed heading section.
**Fix:** Direct the implementer to sweep all `internal/paths` occurrences in overview.md (tree + module list), backstopped by the comprehensive-sweep grep.

## Verdict

REQUEST_CHANGES — plan is mechanically sound, but the batch-1 verify cannot catch errors in the three integration test files it edits.
MILL_REVIEW_END
