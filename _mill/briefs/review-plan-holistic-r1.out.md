MILL_REVIEW_BEGIN
# Review: Spike: structured Go reference/call-graph lookup (go/packages / gopls) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-17
```

## Findings

### [BLOCKING] Card 1 `go mod tidy` strips x/tools before any import
**Location:** Batch 1, Card 1 (and Card 2)
**Issue:** Card 1 runs `go get golang.org/x/tools@latest && go mod tidy`, but no file imports x/tools yet (Card 1 only creates flag/dispatch `main.go`; the import lands in Card 2). `go mod tidy` prunes unused requires, so it removes x/tools from `go.mod`/`go.sum`; Card 2 then adds the import but its `Edits:` lists only `main.go` and it never re-runs `go get`, so Card 2's `go build` fails with a missing-module error.
**Fix:** Drop `&& go mod tidy` from Card 1 (leave the require in place until Card 2 imports it), or move the dep addition into Card 2 with `go.mod`/`go.sum` added to its `Edits:`.

### [BLOCKING] Cards 7 and 8 declare a Commit but stage nothing
**Location:** Batch 3, Cards 7 and 8
**Issue:** Both cards have `Creates/Edits/Deletes: none` and write all output to `.scratch/codeintel/`, which is gitignored (`**/.scratch/` in `.gitignore`). The `Commit:` line is therefore unachievable — `git commit` has nothing to stage, breaking commit-per-card.
**Fix:** Mark these as explicit non-committing measurement steps (no `Commit:` line), or fold their scratch-data capture into Card 9's commit.

## Verdict

REQUEST_CHANGES
One build-breaking dep-sequencing bug and two empty-commit cards; otherwise well-scoped and constraint-clean.
MILL_REVIEW_END
