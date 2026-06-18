# Batch: docs-and-roadmap

```yaml
task: "Weft repo — companion-repo overlay for lyx"
batch: docs-and-roadmap
number: 3
cards: 6
verify: null
depends-on: [1, 2]
```

## Batch Scope

Update all documentation to: (1) replace old Hub/Prime terminology throughout, (2) add the canonical weft overlay model description as a new section in `docs/overview.md`, (3) update config-path references to `_lyx/config/`, (4) deprecate portals, and (5) add roadmap milestones for tasks 006–008. `CONSTRAINTS.md` gets one prose fix and one identifier swap. No Go code is touched — `verify: null` because docs changes have no runnable test surface.

## Cards

### Card 13: Add weft overlay model section and fix terminology in docs/overview.md

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/overview.md`: (1) replace every use of "hub" or "Hub" that refers to the main/primary worktree with "Prime" or "prime" as appropriate; ensure "Hub" is consistently used for the top-level container directory; explicitly rename the identifier `HubName()` → `PrimeName()` in the Layout methods list (approximately line 47); (2) add a new section `## Weft overlay model` containing: (a) a brief topology diagram or ascii block showing `<hub>/`, `<hub>/<prime>/` (host worktree), `<hub>/<prime>-weft/` (weft prime), and example additional worktrees `<hub>/<slug>/`, `<hub>/<slug>-weft/`; (b) a one-paragraph description of git ownership: the host repo stays pristine; all lyx overlay artifacts live in the weft repo, which is a companion git repo lyx controls; (c) a table or list: which artifact lives in host vs weft (e.g. `_lyx/config/` → weft, `_codeguide/` → weft (task 008), `_board/` → board repo (already separate)); (d) the junction model: host worktree holds junctions `_lyx` and `_codeguide` (task 008) that route writes into the sibling weft worktree; junctions are listed in `.git/info/exclude` per worktree, never in committed `.gitignore`; (e) the weft suffix convention: fixed `-weft`; weft path is always computable as `<hub>/<dir-name>-weft`; (f) a "Status" note: Go implementation (paths geometry, paired spawn, `lyx weft` command) is task 006; `_codeguide` junction activation is task 008; (3) ensure the section is positioned logically (after the general overview, before or near the roadmap reference).
- **Commit:** `docs: update overview.md — Hub/Prime terminology and weft overlay model section`

### Card 14: Update roadmap milestones and terminology in docs/roadmap.md

- **Context:**
  - `_mill/discussion.md`
  - `docs/overview.md`
- **Edits:**
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/roadmap.md`: (1) replace all "hub" or "Hub" that refer to the primary worktree with "Prime"; (2) mark portals as deprecated (one-line note: "deprecated; superseded by weft overlay model (task 006)"); (3) add milestone entries for task 006 (weft engine: `internal/paths` weft geometry, paired host+weft spawn/teardown, `lyx weft` command), task 007 (hub-creator / `lyx-clone` skill), and task 008 (`_codeguide` junction, `lyx config` TUI, `_lyx/config/` schema definitions). Each milestone entry should include: task number, title, and a one-sentence summary. Place the three new milestones after the current last active milestone.
- **Commit:** `docs: update roadmap.md — Hub/Prime terminology and tasks 006–008 milestones`

### Card 15: Update Layout type doc in docs/shared-libs/paths.md

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `docs/shared-libs/paths.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/shared-libs/paths.md`: (1) replace all field name references: `Container` → `Hub`, `MainWorktree` → `Prime`; (2) replace `HubName()` → `PrimeName()` everywhere it appears in method tables, examples, and prose (∼14 references total); (3) update the description of `Hub` to read: "top-level container directory that is NOT a git repo; parent of WorktreeRoot"; (4) update the description of `Prime` to read: "path to the main/first worktree checkout (on main branch)"; (5) add one sentence or footnote to the `PortalsDir()`, `PortalLink()`, and `PortalTarget()` entries: "Deprecated — portals are superseded by the weft overlay model. Removal planned for task 006."; (6) verify no `Container`, `MainWorktree`, or `HubName` identifier strings remain after the update.
- **Commit:** `docs: update paths.md — Hub/Prime rename, PrimeName(), portal deprecation notices`

### Card 16: Fix terminology in docs/modules/worktree.md and docs/modules/board.md

- **Context:**
  - `_mill/discussion.md`
  - `docs/overview.md`
- **Edits:**
  - `docs/modules/worktree.md`
  - `docs/modules/board.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/modules/worktree.md`: (1) replace "hub" used for the container directory or the main worktree with the correct term ("Hub" for the container, "Prime" for the main worktree) throughout layout diagrams and prose; (2) add a one-paragraph pointer to `docs/overview.md#weft-overlay-model` for the canonical weft architecture description — do not duplicate the full description here; (3) mark portals as deprecated with a brief note. In `docs/modules/board.md`: (1) update any "hub" terminology similarly; (2) add a pointer to `docs/overview.md#weft-overlay-model` where the board's relationship to the weft model is referenced; (3) update every config-path reference from `_lyx/board.yaml` to `_lyx/config/board.yaml` — the discussion identifies these at approximately lines 153, 227, 234, 262, and 299; this is a path-string-only update, no prose rewrites.
- **Commit:** `docs: update worktree.md and board.md — Hub/Prime terminology, weft pointer, config path`

### Card 17: Update config path references in docs/shared-libs/config.md, shared-libs/README.md, and benchmarks

- **Context:**
  - `internal/config/config.go`
- **Edits:**
  - `docs/shared-libs/config.md`
  - `docs/shared-libs/README.md`
  - `docs/benchmarks/board-performance.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/shared-libs/config.md`: update every reference to `_lyx/board.yaml` or `_lyx/<module>.yaml` to use the new path `_lyx/config/board.yaml` / `_lyx/config/<module>.yaml` — approximately lines 31 and 51. In `docs/shared-libs/README.md`: update the reference to `_lyx/<module>.yaml` pattern at approximately line 19 to `_lyx/config/<module>.yaml`. In `docs/benchmarks/board-performance.md`: update every `_lyx/board.yaml` reference (approximately lines 65 and 66) to `_lyx/config/board.yaml`. No prose content changes beyond the path strings.
- **Commit:** `docs: update config path references to _lyx/config/ in shared-libs and benchmarks`

### Card 18: Fix CONSTRAINTS.md — PrimeName() and Hub terminology

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `CONSTRAINTS.md`: (1) in the Layout method list (under "For New Code"), replace `HubName()` → `PrimeName()`; (2) in prose at line 5 (approximately: "worktree and container geometry"), replace "container" → "hub"; (3) in prose at line 18 (approximately: "root, container, relative path"), replace "container" → "hub". These are the only changes — do not rename field names `Container` or `MainWorktree` because those identifiers do not appear as text in CONSTRAINTS.md.
- **Commit:** `docs: update CONSTRAINTS.md — PrimeName(), hub terminology`

## Batch Tests

`verify: null` — this batch modifies only Markdown documentation files. There is no runnable test surface for doc changes. Correctness is validated by the plan reviewer reading the Requirements fields above.
