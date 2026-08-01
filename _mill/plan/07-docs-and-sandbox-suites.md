# Batch: docs-and-sandbox-suites

```yaml
task: 'fabric: clone-does-everything + subpath-in-weft + init dissolution'
batch: docs-and-sandbox-suites
number: 7
cards: 5
verify: go test ./cmd/lyx/...
depends-on: [6]
```

## Batch Scope

This batch lands the documentation lifecycle work for slice 5, after the CLI tree reaches its final shape (batch 6: `init` gone, `unwire` registered). It: documents slice 5's shipped behavior in `internal/fabricengine/doc.go`; removes `init` from `docs/overview.md` and updates the junction-geometry prose to clone-does-everything + repo-wide `pathspec`; marks slice 5 DONE in the design doc and resolves its open question (without deleting the file — slice 6 remains); rewrites the five "`lyx init` first" sandbox preconditions; and updates `SANDBOX-FABRIC-SUITE.md` (its `lyx init`-seeding precondition, the F0 verb list, and a new `unwire` scenario).

The SANDBOX-CORE-SUITE S6 retarget and its `**Covers:** init` removal are NOT here — they live in batch 6 (card 31) because they are machine-coupled to `sandbox_coverage_test.go`'s Assert-2. This batch's markdown edits are not machine-checked except that `go test ./cmd/lyx/...` confirms `sandbox_coverage_test.go` still passes after the FABRIC-suite edits (no new stale/unknown `**Covers:**` tag introduced). Depends on batch 6 (final CLI tree + CONSTRAINTS/doc state).

Batch-local decision: `manifest/designs/fabric-unified-view.md` is NOT deleted (the Documentation Lifecycle's "delete on landing" rule is overridden here per the discussion) because slice 6 (warp-rebase / remote-reconcile) still lives in that file; slice 5 is only marked DONE.

## Cards

### Card 32: Document slice 5 in `fabricengine/doc.go`

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/unwire.go`
  - `internal/hubgeometry/anchor.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append a paragraph block to the `internal/fabricengine/doc.go` package comment (before the closing `package fabricengine` at doc.go:81), grouped near the existing junction/`pathspec` paragraphs (doc.go:30-42) which already own `WireJunctions`/`UnwireJunctions` and the `pathspec` name-set. Document slice 5's shipped behavior: (1) subpath-in-weft — the lyx-anchor subpath recorded as `.fabric-anchor` on `weft:main`, read by `hubgeometry.Resolve`/`SiblingLayout`; (2) clone-does-everything — `CloneHub` records the anchor + repo-wide `fabric.yaml`, wires junctions, creates `_lyx`, maintains `.gitignore`, and runs `configsync.ReconcileAll` in one shot; (3) repo-wide `pathspec` — the junction name-set is a per-repo fact at `<BoardDir>/_lyx/config/fabric.yaml`, so reconcile converges every worktree; (4) declarative reconcile with stale-removal (fail-closed on a broken `pathspec`); (5) `Unwire` — the per-worktree full deactivation that replaced `lyx init --undo`, leaving the repo-wide records intact. Match the file's existing comment-paragraph style (no headings).
- **Commit:** `docs(fabricengine): document slice 5 shipped behavior in package doc`

### Card 33: Remove `init` from `docs/overview.md` and update junction geometry

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/reconcile.go`
  - `cmd/lyx/main.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/overview.md`:
  - Remove the `init` module-list entry (overview.md:168, the `- **init** — scaffolds the _lyx/ directory structure…` bullet) and the summary line (overview.md:188, `**init** is not a module but a cross-cutting setup command…`).
  - Sweep the two cross-references that tie other modules' config reconcile to `lyx init`: overview.md:170 (`config`: "`lyx config reconcile` reconciles…") and overview.md:181 (`loom`: "…reconciles via `lyx init` / `lyx config reconcile`.") — drop the `lyx init` mention, keeping `lyx config reconcile` where still accurate.
  - Update the `### Junction model` section (overview.md:93-103) to describe the clone-does-everything setup: host worktrees are wired at `lyx fabric clone`/`worktree add` (no separate `lyx init` step), the wired junction set is the **repo-wide** `pathspec` on `weft:main` (read from `<BoardDir>/_lyx/config/fabric.yaml`, filtered against `HubReservedNames()`), and reconcile declaratively converges every worktree to it (add missing / remove stale / no-op correct).
  Keep one-line-per-paragraph markdown. Do not touch the `## Documentation lifecycle` section (overview.md:46-53) — it stays accurate.
- **Commit:** `docs(overview): remove init module, update junction geometry for clone-does-everything`

### Card 34: Mark slice 5 DONE in the design doc and resolve its open question

- **Context:**
  - `docs/overview.md`
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/designs/fabric-unified-view.md`:
  - Mark slice 5 DONE in the Build order (line 112): change the `5. **Clone-does-everything + subpath-in-weft + `init` dissolution** — …` lead-in to the established DONE marker format used by slices 1-4 (e.g. `5. **DONE — Clone-does-everything + subpath-in-weft + `init` dissolution** (landed as the `fabric-clone-subpath` task) — …`), and briefly record the resolved shape (subpath recorded on `weft:main`; `Resolve`/`SiblingLayout` read the anchor with a cwd gate; clone/add wire eagerly; `init` deleted; `unwire` is the teardown).
  - Resolve the "RelPath record-vs-cwd reconciliation mechanism" open question (line 120) using the same `**DONE — …**` convention the resolved open questions already use (lines 119,121-123,126): state the answer — a plain `.fabric-anchor` marker at `<BoardDir>` (weft:main root) holds the subpath; `Resolve` reads it (record wins), validates cwd is at or below the anchored subtree (hard error otherwise), and falls back to cwd when absent; `SiblingLayout` reads the same marker without the gate.
  - **Do NOT delete the file** — slice 6 (warp-rebase / remote-reconcile) remains, so the Documentation Lifecycle's delete-on-landing rule does not apply yet.
  Keep one-line-per-paragraph markdown.
- **Commit:** `docs(design): mark fabric-unified-view slice 5 DONE, resolve RelPath open question`

### Card 35: Rewrite the five "`lyx init` first" sandbox preconditions

- **Context:**
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/add.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
  - `tools/sandbox/SANDBOX-PERCH-SUITE.md`
  - `tools/sandbox/SANDBOX-BURLER-SUITE.md`
  - `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the item-4 "**`lyx init` first.**" precondition in each suite to describe the clone-does-everything setup — worktrees are wired at `lyx fabric clone`/`lyx fabric add` (junctions + `_lyx`/config materialized eagerly), NOT via a separate `lyx init` step: `SANDBOX-SHUTTLE-SUITE.md:14`, `SANDBOX-WEBSTER-SUITE.md:14`, `SANDBOX-PERCH-SUITE.md:16`, `SANDBOX-BURLER-SUITE.md:16`, `SANDBOX-BUILDER-SUITE.md:16`. Preserve each precondition's module-specific config detail (e.g. shuttle needs `_lyx/config/shuttle.yaml` + `reed.yaml`; perch needs `perch.yaml`; etc.) — only the "run `lyx init` first" mechanism changes to "the worktree must be wired by `lyx fabric clone`/`add`" (which now materializes those configs). Retitle the bold lead-in from "**`lyx init` first.**" to something accurate like "**Wired worktree required.**". Keep one-line-per-paragraph markdown.
- **Commit:** `docs(sandbox): rewrite lyx-init-first preconditions for clone-does-everything`

### Card 36: Update SANDBOX-FABRIC-SUITE (precondition, verb list, unwire scenario)

- **Context:**
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/unwire.go`
  - `internal/fabricengine/unwire.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `tools/sandbox/SANDBOX-FABRIC-SUITE.md`:
  - Rewrite precondition 4 (SANDBOX-FABRIC-SUITE.md:18, "**Weft `_lyx/` must be seeded before `lyx init`.**") — `lyx init` is gone; `lyx fabric clone` now wires the host `_lyx` junction. Keep the substantive warning (the dedicated weft test repo must have an `_lyx/` seeded on its primary branch, else a fresh clone creates a dangling junction) but reframe it around `lyx fabric clone` instead of `lyx init`.
  - Update F0 (SANDBOX-FABRIC-SUITE.md:81) — the verb-list assertion at line 85 lists 14 verbs (`clone, add, list, remove, checkout, pairs, reconcile, prune, cleanup, status, commit, push, pull, sync`); add `unwire` so it lists 15.
  - Add a new scenario **F5 — Junction deactivation (`lyx fabric unwire`)** after F4 (insert before the `---`/`## Session log format` header at line 133), mirroring F4's format (`### F5 -- …`, optional `**Covers:** fabric`, `**Goal:**`, `**Watch:**`, `**Verdict:**`): drive `lyx fabric unwire` on a wired worktree, watch that it removes all fabric junctions, clears the weft `_lyx` (not `_pattern`), reverts `.gitignore`, is idempotent/no-ops on an unwired worktree, and leaves the repo-wide `.fabric-anchor`/`fabric.yaml` intact so a follow-up `lyx fabric reconcile` re-wires.
  - Update the two spots that hardcode the `F0`..`F4` range for the new F5: the `ref` field doc (SANDBOX-FABRIC-SUITE.md:73) and the Session log format template (SANDBOX-FABRIC-SUITE.md:139-150).
  Keep one-line-per-paragraph markdown.
- **Commit:** `docs(sandbox): update FABRIC suite for unwire verb and clone-does-everything`

## Batch Tests

`verify: go test ./cmd/lyx/...` runs `sandbox_coverage_test.go` (which reads every `tools/sandbox/*SUITE.md`) plus the other cmd/lyx guards, confirming this batch's suite-file edits introduce no stale/unknown `**Covers:**` tag and the doc edits leave the coverage guard green. The remaining edits (`doc.go`, `docs/overview.md`, `manifest/designs/fabric-unified-view.md`, the five precondition suites) are pure documentation with no runnable surface — `doc.go` is a package comment (compiled by any fabricengine build, already covered by batch 6's verify and the `done_gate`). No new Go behavior is introduced here, so a scoped cmd/lyx run plus the repo-wide `done_gate` (`go test ./...`) is sufficient.
