# Batch: docs: CONSTRAINTS, overview, README, manifest, sandbox suites

```yaml
task: 'board: move storage to weft:main'
batch: 'docs: CONSTRAINTS, overview, README, manifest, sandbox suites'
number: 6
cards: 6
verify: null
depends-on: [1, 2, 3, 4, 5]
```

## Rename mechanic

Not applicable — this batch contains no `Moves:` entries.

## Batch Scope

This batch is the documentation-lifecycle close-out `_mill/discussion.md`'s Scope section requires in the same commit as the code: `CONSTRAINTS.md`'s Weft Git Invariant carve-out, `docs/overview.md`'s topology/artifact-table rewrite, `README.md`'s git-version bump, `manifest/roadmap.md`'s Planned→Done move plus a new deferred-automation Someday entry, `manifest/designs/board-weft-storage.md`'s deletion with its durable content folded into `internal/boardengine`'s package doc, and the two sandbox suite files' scenario prose. It depends on every prior batch since its content describes the FINAL shape of the code (exact function names, exact CLI surface, exact test-guard names). `verify: null` — every card in this batch is a prose/comment edit with no runnable surface of its own; `pipeline.done_gate: "go test ./..."` in `mill-config.yaml` is the whole-module compile/test safety net this task relies on at Handoff, not a per-batch concern here. Confirmed during planning (a background research fork, cross-checked against direct reads of the live files) that `manifest/designs/fabric-unified-view.md`'s "After `board: move storage to weft:main`" sequencing passage is written prospectively and already reads correctly once this task ships — it is NOT edited by this batch, despite appearing plausible at first glance as a docs-batch target.

## Cards

### Card 28: CONSTRAINTS.md — Weft Git Invariant board carve-out + guard reference

- **Context:**
  - `cmd/lyx/boardguard_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the Weft Git Invariant section's "**Orchestration, not agent.**" bullet, append a new sentence after the existing "Asymmetry: an agent **does** commit its own code to the **host** repo... — the weft, never." text: "**Board carve-out.** `internal/boardengine`'s writes to `weft:main` are the one deliberate exception to timing control living with the loop owner: any LLM session, in any worktree, may trigger a board write (via `lyx board <verb>`) at any time — that is the whole point of board's shared-visibility model. Module ownership still holds without exception (board's git flows through `fabricengine.CommitWeftAt`/`PushWeftAt`, never raw git); only the *timing*-control half is scoped away from board. The general "who may time a weft commit" question gets a fuller treatment once `fabric-unified-view.md`'s `Fabric.Commit` work lands (sequenced after this task)." In the "**Enforced by**" bullet at the end of the same section, change "The module-ownership half is a candidate for a future import/grep guard; not machine-checked today." to "The module-ownership half is machine-checked for `internal/boardengine` specifically by `cmd/lyx/boardguard_test.go` (no raw `gitrepo`/`gitexec` import or shell-out); the general case (every other `fabricengine` caller) remains a candidate for a future import/grep guard, not machine-checked today."
- **Commit:** `docs(constraints): record board's Weft Git Invariant timing carve-out and its new guard`

### Card 29: docs/overview.md — topology diagram + artifact table

- **Context:** none
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change line 67's topology-diagram entry, currently `  └── _board/                       (board repo; the task store)`, to `  └── _board/                       (weft:main worktree; the task store)`. Change line 81's artifact-table row, currently `| \`_board/\` | Hub | Board | Task board at a **configured** board-repo URL — \`lyx board\` accepts any URL; \`ly-git-clone\` defaults it to the weft repo's GitHub wiki (\`<weft>.wiki.git\`) |`, to `| \`_board/\` | Hub | Board | A second weft worktree, checked out on the host's own unsuffixed default branch (\`weft:main\` in the common case) — never a separate clone, never \`<branch>-weft\` |`. The Modules table's `board` row (module description) needs no change — confirmed during planning it stays accurate ("the task-tracker board (`internal/boardcli` + `internal/boardengine`)").
- **Commit:** `docs(overview): update _board's topology/artifact description for weft:main storage`

### Card 30: README.md — git version requirement bump

- **Context:** none
- **Edits:**
  - `README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `## Requirements` section, change `- Git 2.35+ (for \`git worktree\`)` to `- Git 2.42+ (for \`git worktree add --orphan\`)` — the new minimum needed by `_board`'s fresh-bootstrap orphan-branch path (batch 2, `internal/fabricengine/boardweft.go`).
- **Commit:** `docs(readme): bump minimum git version to 2.42+ for git worktree add --orphan`

### Card 31: manifest/roadmap.md — move Planned → Done, add curation-triage Someday entry

- **Context:** none
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `## Planned` list's first entry (the full `1. **board: move storage to \`weft:main\`** — ...` bullet, currently ending "...See [designs/board-weft-storage.md](designs/board-weft-storage.md)."). Add a new bullet to the `## Done` list (placed adjacent to the two existing `board`-named Done entries, "**board**" and "**board: use \`gitrepo\` as its git operator**", to keep board's shipped history grouped) summarizing what shipped: storage moved to a second weft worktree at `<hub>/_board` on the host's own unsuffixed default branch (never a separate clone, never `<branch>-weft`); board's git routes through `internal/fabricengine`'s `CommitWeftAt`/`PushWeftAt` (a new warp-untethered primitive) instead of a direct `gitrepo.Repo` handle, preserving board's existing detached-sync architecture unchanged; a new `notes.json` store for not-yet-claimable manifest entries, sharing `tasks.json`'s exact schema; a `promote-note` cross-store move command; a single generated `README.md` (replacing `Home.md`+`_Sidebar.md`) with Tasks/Done and Manifest sections; a `short_name` field and a 32-character slug length cap — matching the existing Done entries' prose density and ending with "See the `internal/boardengine` package documentation." (no link — per Maintenance, "Done entries above don't link anywhere" since the design doc is deleted on ship). Update the existing `1. **board** — task tracker (storage model superseded by the Planned \`board\` item once it ships).` Done entry: change "(storage model superseded by the Planned \`board\` item once it ships)" to "(storage model superseded by the \`board: move storage to weft:main\` item below)" — the forward reference becomes a same-section cross-reference once this ships, per Maintenance's bold-name cross-reference convention (not "the Planned `board` item", which is no longer true). In the `## Planned` list's `fabric: unified-repo view` entry (Planned #2 after this deletion), change "Depends on the Planned \`board\` item (which removes \`board-url\` from clone); see \`fabric\` in Done below for its current status." to "Depends on the \`board: move storage to weft:main\` item (which removed \`board-url\` from clone; see Done below); see \`fabric\` in Done below for its current status." Add a new bullet to the `## Someday` list (appended at the end, order is immaterial per Maintenance's automatic-numbering note): `1. **board: curation/triage automation** — the GitHub-issue-intake and periodic-triage workflow originally scoped in \`designs/board-weft-storage.md\`'s Curation flow section, deferred out of \`board: move storage to weft:main\`: an automated skill that ingests GitHub issues and extracts a logical next task from the manifest, promoting it via \`promote-note\` (which already ships as a plain mechanical CLI primitive — this item is the automation layer on top, not the primitive itself). See [designs/curation-triage.md](designs/curation-triage.md).` (this new stub doc is created by Card 32, in the same batch — the roadmap entry and the stub it links to land together).
- **Commit:** `docs(roadmap): move board storage item to Done, record deferred curation/triage automation`

### Card 32: fold board-weft-storage.md into board.go's package doc; stub the deferred curation-triage design; delete the superseded design doc

- **Context:**
  - `manifest/designs/board-weft-storage.md`
  - `internal/boardengine/board.go`
- **Edits:**
  - `internal/boardengine/board.go`
- **Creates:**
  - `manifest/designs/curation-triage.md`
- **Deletes:**
  - `manifest/designs/board-weft-storage.md`
- **Moves:** none
- **Requirements:** Per the Documentation Lifecycle (`docs/overview.md#documentation-lifecycle`): module-design docs are deleted when their module lands, with durable rationale folded into the Go package doc comment next to the code — the exact pattern `internal/fabricengine/doc.go` already follows, and the pattern `board-weft-storage.md`'s own header line already commits to. Extend `board.go`'s package doc comment (already touched by batch 3's Card 14 for the git-routing rewrite) with new content folding in the durable parts of `board-weft-storage.md`: (a) the branch-reservation rationale from its "Branch naming convention" section — no task's weft branch can ever be named exactly the host's default branch (every paired weft branch carries the `-weft` suffix), which is what makes that unsuffixed name permanently unclaimed by the pairing convention and reserved exclusively for board; (b) the why-not-alternatives rationale from "Why the earlier approaches were rejected" — a separate repo is unneeded git-identity overhead, and GitHub wiki rendering requires the host repo to be public, disqualifying for private consulting work; (c) prime's asymmetric relationship to weft from "The 'prime' worktree's asymmetric relationship to weft" — prime is the only worktree with a reason to check out two weft branches simultaneously (its own ordinary `<name>-weft` companion, plus `weft:main` for board access, never paired with any warp branch); (d) the `fabric` consequence from "`fabric` consequence" — `weft:main` has no corresponding warp branch, so the `Warp-SHA` trailer/correspondence-index machinery does not apply to it. Do NOT fold in the now-superseded "Board's data model" (the four-category Proposals/Manifest/Tasks/Done model) or "Two JSON stores, not one" sections' `curated`-flag design — `_mill/discussion.md`'s Decisions section replaced both with the plain two-file `tasks.json`/`notes.json` split with no curated flag; the package doc should describe the CURRENT (shipped) two-store design, not the superseded one. Do NOT fold in "How other worktrees reach board" (portal-based access) — superseded by the finding that no junction was ever needed (`hubgeometry.BoardDir(hub)` is a pure function of the shared `Hub` path). Do NOT fold in "Curation flow" — that content moves to the new stub instead (next). Create `manifest/designs/curation-triage.md`, a short Someday-tier stub (mirroring `hardener.md`'s "DRAFT doc, do not implement from it yet" framing per `manifest/roadmap.md`'s own description of it) with a status header stating it is deferred out of this task and carries forward `board-weft-storage.md`'s now-deleted "Curation flow" section (about to be deleted by this same card) verbatim in substance: raw notes can be added by anyone with no intake gatekeeping (including via a GitHub issue on the `weft` repo); only the orchestrating thread (running in prime) curates them; task extraction from the manifest is a deliberate, explicit, human-triggered command (a skill), not an autonomous background loop; and a section stating explicitly what already shipped and is OUT of this stub's remaining scope — `notes.json`/`tasks.json`'s shared schema and `promote-note`'s pure mechanical cross-store move, both delivered by `board: move storage to weft:main` — this item is the automation layer (GitHub-issue ingestion, periodic/triggered triage) on top of that already-shipped primitive, not the primitive itself. Delete `manifest/designs/board-weft-storage.md` in the same commit as both edits above.
- **Commit:** `docs(boardengine): fold board-weft-storage.md's durable rationale into board.go, stub curation-triage.md, delete superseded design doc`

### Card 33: sandbox suite prose updates

- **Context:** none
- **Edits:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `SANDBOX-CORE-SUITE.md`'s S3 "Board and task interaction" scenario (`**Covers:** board`): add one clarifying sentence near the top of the scenario's description noting `_board` is a second weft worktree with no separate `board-url` (the scenario's existing CRUD-only Goal/Watch content needs no other change — it never asserted anything about storage provenance). In `SANDBOX-FABRIC-SUITE.md`: delete the "Board-URL fallback" pre-condition bullet entirely (it describes a `[board-url]` fallback mechanism this task removes — `lyx fabric clone`'s default board URL no longer exists to fall back on). In the F1 "Clone geometry" scenario's `**Watch:**` text, replace the board-passenger-origin-URL check (currently: "The board passenger's origin URL is the default derived form — `<weft-url>.wiki.git` — provided the operator has initialized that wiki" plus the `git -C _board remote -v` command) with a worktree-provenance check: `_board` shares the weft prime's git common-dir (`git -C <weft-prime> rev-parse --git-common-dir` and `git -C _board rev-parse --git-common-dir` resolve to the same path) and is checked out on the dynamically-derived unsuffixed host branch (never hardcoded to "main") — mirroring batch 2's own `clone_adopt_test.go` assertion shape.
- **Commit:** `docs(sandbox): update board/fabric scenarios for _board's weft-worktree provenance`

## Batch Tests

`verify: null` — every card in this batch is a documentation/comment edit with no independent runnable surface (the code changes they describe were already verified by batches 1-5's own `verify:` commands). `pipeline.done_gate: "go test ./..."` still runs once at Handoff and would catch a card in this batch that accidentally broke a `//go:embed` directive or similar — none of these edits touch embedded/templated files, so no regression is expected.
