MILL_REVIEW_BEGIN
# Review: board: move storage to weft:main — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [BLOCKING] CloneHub test call sites break on the new 2-value return
**Location:** Batch 2, Card 9 (`internal/fabricengine/clone_adopt_test.go`)
**Issue:** Card 5 shrinks `CloneHub` from 3 return values (`hubPath, resolvedBoardURL, err`) to 2 (`hubPath, err`). Card 9's instructions for all three existing call sites only say to "drop the trailing argument" — the current source has `hubPath, _, err := fabricengine.CloneHub(...)` / `_, _, err := fabricengine.CloneHub(...)`, and nothing in Card 9 tells the implementer to also drop one destructured variable. Literal compliance leaves `go test ./internal/fabricengine/...` failing to compile ("assignment mismatch: 3 variables but 2 values").
**Fix:** Explicitly instruct changing each destructuring to 2 values (`hubPath, err :=` / `_, err :=`), matching the correction Card 7 already makes correctly for the production call site.

### [BLOCKING] No verify: actually runs the integration tests this plan adds
**Location:** Plan-wide — `00-overview.md` Batch Index; every batch's `verify:` field
**Issue:** Every `verify:` is bare `go test ./...`-shaped with no `-tags integration`. Per `docs/benchmarks/running-tests.md`, that is Tier 1 only — every `//go:build integration` file is excluded from the build entirely, including Cards 3, 9, 13 (`boardtest`), 23/24, and 27's new/updated tests. Yet each batch's own "Batch Tests" section claims these are exercised ("both tagged and untagged"), which is false as literally specified.
**Fix:** Add a second `-tags integration` run (or append the flag) to every batch's `verify:` that touches an integration-tagged file.

### [BLOCKING] Batch 5's verify: never targets internal/fabriccli
**Location:** Batch 5 (`00-overview.md` Batch Index and `05-cmd-lyx-board-guards.md` frontmatter)
**Issue:** `verify: go test ./cmd/lyx/...` never targets `internal/fabriccli`, yet Card 27 adds `TestRunCLI_CloneRequiresExactlyTwoArgs` to `internal/fabriccli/cli_test.go`. The batch's own "Batch Tests" prose claims `go test ./cmd/lyx/... ./internal/fabriccli/...`, directly contradicting the authoritative frontmatter `verify:` both files declare.
**Fix:** Change the overview's Batch Index entry and the batch file's frontmatter `verify:` to `go test ./cmd/lyx/... ./internal/fabriccli/...`.

### [BLOCKING] Card 15 Context omits store.go
**Location:** Batch 3, Card 15
**Issue:** Requirements reference `*Store` and several of its methods (`slugIndex()`, `GetTask`, `RemoveTask`, `UpsertTask`, `SetStatus`, `SetDeps`, `UpsertTasksBatch`, `MergeTasks`, `ListTasksBrief`, `ListTasksFull`) throughout, but Context lists only `internal/boardengine/task.go`. `store.go`, where `Store` and every one of those methods is actually defined, is absent from both Context and Edits.
**Fix:** Add `internal/boardengine/store.go` to Card 15's Context.

### [NIT] Card 32 leaves dead links to the deleted design doc
**Location:** Batch 6, Card 32
**Issue:** Deleting `manifest/designs/board-weft-storage.md` leaves dangling `[board-weft-storage.md](board-weft-storage.md)` links in `fabric-unified-view.md`, `host-visibility.md`, `pattern.md` (×2), and `raddle.md`, plus a stale name-mention in `internal/fabricengine/cleanup.go`'s comment — none addressed by this or any other card. The doc's forward-looking "Consequence" section (roadmap.md/mill-wiki eventual fold-in) is also dropped with no preservation anywhere, unlike "Curation flow," which explicitly gets a stub.
**Fix:** Add a sub-step sweeping `manifest/designs/*.md` (and the cleanup.go comment) for `board-weft-storage` references; fold the "Consequence" note into board.go's doc or a roadmap Someday bullet.

### [NIT] README.md keeps the stale `_board` description Card 29 fixes elsewhere
**Location:** Batch 6, Card 29 / `README.md`
**Issue:** README.md's own topology diagram carries the identical stale line `_board/  (board repo; the task store)` that Card 29 corrects in `docs/overview.md`; no card touches README's copy, so it ships inconsistent with the corrected description right after this task lands.
**Fix:** Add a matching README.md diagram-line update to Card 29 (or a new sub-step in the same batch).

## Verdict

REQUEST_CHANGES
Four BLOCKING gaps: a compile-breaking test-signature miss, two verify: reachability gaps, one missing Context entry.
MILL_REVIEW_END
