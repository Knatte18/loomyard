# Batch: rerender-manifest

```yaml
task: "Board fixes from sandbox run — payload keys, help, rerender"
batch: "rerender-manifest"
number: 3
cards: 2
verify: go test ./internal/board/
depends-on: []
```

## Batch Scope

Delivers W13: `rerender` (and every write, since they share the render path) cleans up
stale output files when an output filename changes. Replaces the proposal-prefix-only
`removeOrphanProposals` glob with a general manifest: a best-effort sidecar that records
the filenames the last render produced, so the next render can remove any previously-
rendered file the current render no longer produces — covering `home`/`sidebar` renames
and `proposal_prefix` changes, not just orphaned proposals. Independent of batches 1–2
(touches only `render.go`, `sync.go`, `render_test.go`), so it carries no dependency edge
and may run in parallel. Batch-local decision: the manifest is transient local state,
gitignored exactly like the existing `*.lock`/`*.swaplock` sidecars (via
`ensureLockfilesIgnored`), so it never adds commit churn; correctness does not depend on
it being tracked, because a missing manifest degrades gracefully.

## Cards

### Card 8: Manifest-based render cleanup

- **Context:**
  - `_mill/discussion.md`
  - `internal/fsx/fsx.go`
- **Edits:**
  - `internal/board/render.go`
  - `internal/board/sync.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace `removeOrphanProposals` in `internal/board/render.go` with a
  manifest mechanism. Add a constant for the manifest filename (e.g.
  `renderManifestFile = ".board-rendered.json"`). In `RenderToDisk`, after writing the
  current render's `files` (the `map[string]string` of filename→content): (1) read the
  previous manifest from `filepath.Join(boardPath, renderManifestFile)` best-effort — a
  missing or corrupt/unreadable manifest is treated as an empty set (nothing known to
  clean); (2) for every filename in the previous manifest that is NOT a key in the current
  `files` map, `os.Remove(filepath.Join(boardPath, name))` best-effort; (3) write the new
  manifest as the JSON array of the current `files` keys (sorted for stable output) via
  `fsx.AtomicWrite`/`AtomicWriteBytes`. All manifest read/write/remove steps are
  best-effort and MUST NOT fail the write (mirror the current `removeOrphanProposals`
  contract — "a stale file left behind is harmless and cleaned up on the next render").
  The manifest filename is never one of the rendered `files` keys, so it is never a
  deletion candidate or self-referential. Delete the `removeOrphanProposals` function. In
  `internal/board/sync.go`, extend `ensureLockfilesIgnored` to also ignore the manifest
  (add `.board-rendered.json` — or a matching pattern — to the patterns list) so the
  sidecar is never committed, consistent with the `*.lock`/`*.swaplock` handling.
- **Commit:** `feat(board): manifest-based render cleanup for renamed outputs`

### Card 9: Manifest cleanup tests

- **Context:**
  - `_mill/discussion.md`
  - `internal/board/render.go`
  - `internal/board/config.go`
- **Edits:**
  - `internal/board/render_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update `internal/board/render_test.go`: remove/adapt any test that
  calls `removeOrphanProposals` directly (the function is deleted), and add manifest
  coverage driving `RenderToDisk` (or the `Board.Rerender` path) with different `Outputs`:
  (1) render with `Home: "Home.md"`, then render the same tasks with `Home: "Index.md"`,
  assert `Home.md` is removed and `Index.md` exists; (2) same for a `Sidebar` rename and a
  `ProposalPrefix` change; (3) orphaned-proposal cleanup now works ACROSS renders, not in a
  single pass — the existing `TestRenderToDisk` case that pre-creates a hand-placed
  `proposal-ghost.md` and asserts ONE `RenderToDisk` removes it via the old glob MUST be
  restructured (the manifest only removes files it previously recorded, so a first render
  with no prior manifest seeds only and removes nothing): either pre-seed the manifest with
  `proposal-ghost.md` before the render, or use two renders. Also cover the body-loss
  path: a task that renders a proposal (recorded in the manifest) then loses its `Body` has
  its `proposal-<slug>.md` removed on the next render; (4) a hand-added unrelated file
  (e.g. `README.md`) in the board dir is NEVER removed; (5) graceful degradation — rendering when no manifest exists seeds it
  and removes nothing, and a corrupt/unreadable manifest does not fail the write and is
  overwritten by the current render set. Follow the existing `render_test.go` fixture
  conventions for constructing `Outputs` and a temp board dir.
- **Commit:** `test(board): cover manifest cleanup and graceful degradation`

## Batch Tests

`verify: go test ./internal/board/` runs the board package tests including the rewritten
`render_test.go`. Scope is the single board package — `render.go`/`sync.go` are
board-internal. No `PYTHONPATH=` prefix — Go project. The render tests are the guardrail
for the rename-cleanup behavior and the best-effort graceful-degradation contract.
