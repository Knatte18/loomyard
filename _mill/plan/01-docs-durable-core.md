# Batch: docs-durable-core

```yaml
task: "Reconcile stale design docs (stateless + weft model)"
batch: docs-durable-core
number: 1
cards: 3
verify: null
depends-on: []
```

## Batch Scope

This batch makes `docs/overview.md` the single durable map of what each module and shared lib is for, and writes the documentation-lifecycle convention that stops the rot from recurring. It is the root batch because the rest of the work links *into* overview.md (the benchmarks retarget and the moved research docs point at overview's module map) — those anchors must exist first. Deliverables: (1) overview's Modules + shared-libs + "Other docs" sections rebuilt into a status-marked module/lib map with no links to soon-deleted/moved docs and no stale `(planned)` marker on `internal/state`; (2) a new "Documentation lifecycle" section, the `PortalLink` addition + portal-deprecation tag on the Path-Invariants method list, and a clear weft-not-built note; (3) the same convention pointer + portal-deprecation tags landed in `CONSTRAINTS.md`. External interface the next batch consumes: the `## Modules` heading in overview.md (anchor `#modules`) is the retarget destination for board/muxpoc links elsewhere.

Batch-local decision: when rebuilding overview's module map, use an explicit status token per entry — `✅ implemented` for landed modules (board, worktree, ide, muxpoc) and shared libs (config, git, lock, output, paths, state, fsx, gitignore), `🚧 design — not built` for `mux` and the **weft** model. Do not invent module docs links that this task deletes; describe purpose inline in overview instead.

## Cards

### Card 1: Rebuild overview.md into the status-marked module/lib map

- **Context:**
  - `_mill/discussion.md`
  - `docs/roadmap.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In the `## Modules` section: keep one bullet per user-facing module (`board`, `worktree`, `ide`, `muxpoc`, `mux`) but **remove every link to `modules/board.md`, `modules/worktree.md`, `modules/ide.md`, `modules/muxpoc.md`** (those docs are deleted in batch 2). For each landed module give a one-line purpose statement inline + a `✅ implemented` marker. Keep the `mux` bullet linking to `modules/mux.md` (it survives) and mark it `🚧 design — not built`. Keep the `init` paragraph.
  - In the shared-infrastructure sentence (currently `internal/config`, `internal/git`, `internal/lock`, `internal/state` **(planned)**) and the `## Other docs` `shared-libs/` line: **remove the `(planned)` marker on `internal/state`** — it landed in `ba81abf` (generic helper, no consumer yet). State the shared libs as the landed set (`config`, `git`, `lock`, `output`, `paths`, `state`, `fsx`, `gitignore`).
  - In the `## Other docs` list: **remove the bullets that link to the four deleted module docs** (`modules/board.md`, `modules/worktree.md`, `modules/ide.md`, `modules/muxpoc.md`) and **repoint the three research-doc references** — the existing `modules/mux-hooks-exploration.md` reference (and any `mux-exploration.md` / `mux-proposal.md` references) move to `research/` (e.g. `research/mux-hooks-exploration.md`). Repoint the psmux upstream reference from `vendor/psmux_scripting.md` to `reference/psmux_scripting.md` and label it "Upstream psmux command reference (vendored)". Keep links to `roadmap.md`, `modules/mux.md`, `shared-libs/README.md`, and `benchmarks/`.
  - **(review r1) Directory-tree block:** in the fenced directory-tree block (≈L122-135, inside the `## Weft overlay model` section) the per-package lines carry trailing `(see modules/board.md)`, `…/worktree.md`, `…/ide.md`, `…/muxpoc.md` comments — **strip just those `(see modules/<x>.md)` comments** for the four deleted module docs (leave the tree structure and any `internal/mux`/`modules/mux.md` reference intact). This is a link fix; do not otherwise alter the Weft section's architecture content (the not-built Status note is Card 2's job).
  - **(review r1) init paragraph:** the `init` paragraph (≈L185) links `modules/board.md#init`, which is deleted — **retarget it to `overview.md#modules`** (or drop the link and describe `init` inline). Keep the paragraph itself.
- **Commit:** `docs(overview): rebuild module/lib map with status markers, drop stale links`

### Card 2: Add Documentation-lifecycle section, fix portal method list and weft-not-built note

- **Context:**
  - `_mill/discussion.md`
  - `docs/shared-libs/paths.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Add a new `## Documentation lifecycle` section stating the convention verbatim in intent: *mechanical per-module docs (`docs/modules/<module>.md`) are deleted when their module lands; the implementation + tests are the source of truth; the durable docs are this `overview.md` (principles, naming, the module/lib map, the weft contract) and the not-yet-landed portion of `roadmap.md`; a module's purpose and key hazards live in its Go package doc-comment.* Place it after `## Principles` or near the module map — planner's choice for flow.
  - In the `## Path Invariants` section's `Layout` method enumeration (the line listing `LyxDir()`, `WorktreePath(slug)`, `PortalsDir()`, `PortalTarget(slug)`, `LaunchersDir()`, …): **add `PortalLink(slug)`** (currently omitted; `PortalTarget(slug)` is already present) and tag the three portal methods (`PortalsDir`, `PortalLink`, `PortalTarget`) as **deprecated-but-present (removed in task 006)**, matching `paths.md`'s framing. Do not remove them.
  - In the `## Weft overlay model` `### Status` subsection: make explicit that **weft has no Go implementation yet** — portals are still the live mechanism until task 006 — strengthening the existing "Go code lands in downstream tasks" note. The weft model stays described as the decided target architecture.
- **Commit:** `docs(overview): add doc-lifecycle section, fix portal method list, flag weft not-built`

### Card 3: CONSTRAINTS.md — convention pointer and portal-deprecation tags

- **Context:**
  - `_mill/discussion.md`
  - `docs/overview.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In the `Layout`-methods enumeration (the bullet listing `LyxDir()`, `WorktreePath(slug)`, `PortalsDir()`, `PortalLink(slug)`, …): tag `PortalsDir`, `PortalLink`, `PortalTarget` as **deprecated-but-present (removed in task 006)**, matching `docs/overview.md` and `docs/shared-libs/paths.md`. Do not remove them.
  - Add a one-line pointer to the documentation-lifecycle convention: a sentence directing readers to `docs/overview.md#documentation-lifecycle` for the delete-mechanical-module-docs-on-landing rule. Do not duplicate the convention text here — CONSTRAINTS.md owns the build-enforced path invariant; the lifecycle convention lives in overview.md.
- **Commit:** `docs(constraints): tag portal methods deprecated, point to doc-lifecycle convention`

## Batch Tests

`verify: null` — pure-docs batch with no runnable test surface. Verification is by inspection and is finalized in batch 2's broken-link / stale-term sweep (which scans all kept docs including this batch's output). For this batch specifically, confirm by reading `docs/overview.md`: (1) no link to any `modules/{board,worktree,ide,muxpoc}.md`; (2) `internal/state` carries no `(planned)` marker; (3) a `## Documentation lifecycle` section exists; (4) the Path-Invariants method list includes `PortalLink` with the three portal methods tagged deprecated; (5) the weft Status note says weft is not built yet. Confirm `CONSTRAINTS.md` tags the three portal methods deprecated and points to the convention.
