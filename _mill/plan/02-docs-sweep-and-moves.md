# Batch: docs-sweep-and-moves

```yaml
task: "Reconcile stale design docs (stateless + weft model)"
batch: docs-sweep-and-moves
number: 2
cards: 8
verify: null
depends-on: [1]
```

## Batch Scope

This batch applies the cleanup the durable core (batch 1) makes safe: delete the landed mechanical docs, relocate research + reference material, sweep every remaining kept doc of stale vocabulary and dead links, and fix the one dangling benchmarks link. It depends on batch 1 because several edits retarget links *into* overview.md's module map (`#modules`), which batch 1 builds. Every card here is pure-docs (`.md` only); no `.go` and no tests. The batch is the single home for all deletions/moves so the broken-link sweep (Batch Tests) runs once against the final tree.

Batch-local decisions: (1) use `git mv` for relocations to preserve history (Requirements phrase the moves as `git mv`; the plan models them as `Deletes:` old + `Creates:` new). (2) Per the `mux-registry-semantics` decision in discussion.md, in `mux.md` drop only the *worktree*-registry coupling + the dead `state.md` link, keep mux's *own* state-document references and the live `config.md` link.

## Cards

### Card 4: Delete the landed mechanical module + shared-lib docs

- **Context:**
  - `docs/overview.md`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `docs/modules/board.md`
  - `docs/modules/worktree.md`
  - `docs/modules/ide.md`
  - `docs/modules/muxpoc.md`
  - `docs/shared-libs/git.md`
  - `docs/shared-libs/lock.md`
  - `docs/shared-libs/fsx.md`
  - `docs/shared-libs/gitignore.md`
  - `docs/shared-libs/state.md`
- **Requirements:**
  - `git rm` the nine listed docs (landed modules board/worktree/ide/muxpoc; mechanical shared libs git/lock/fsx/gitignore/state). Their content is reconstructable from code+tests per the doc-lifecycle convention. Read `docs/overview.md` (updated in batch 1) to confirm it no longer links to any of them before deleting.
- **Commit:** `docs: delete landed mechanical module and shared-lib docs`

### Card 5: Relocate research logs and the psmux reference; fix their outbound links

- **Context:**
  - `docs/overview.md`
  - `docs/modules/mux.md`
  - `docs/psmux-tui-behavior.md`
- **Edits:** none
- **Creates:**
  - `docs/research/mux-exploration.md`
  - `docs/research/mux-hooks-exploration.md`
  - `docs/research/mux-proposal.md`
  - `docs/reference/psmux_scripting.md`
- **Deletes:**
  - `docs/modules/mux-exploration.md`
  - `docs/modules/mux-hooks-exploration.md`
  - `docs/modules/mux-proposal.md`
  - `docs/vendor/psmux_scripting.md`
- **Requirements:**
  - `git mv docs/modules/mux-exploration.md docs/research/mux-exploration.md` and likewise for `mux-hooks-exploration.md` and `mux-proposal.md`; `git mv docs/vendor/psmux_scripting.md docs/reference/psmux_scripting.md`.
  - In the three moved research docs, fix outbound links broken by the move: links to `mux.md` (which stays in `docs/modules/`) become `../modules/mux.md`; links to the deleted `muxpoc.md` (e.g. `mux-proposal.md` L5/L146) retarget to `../overview.md#modules` (overview's muxpoc entry); verify `../psmux-tui-behavior.md` still resolves from `docs/research/` (it points at `docs/psmux-tui-behavior.md` — correct) and leave it. Fix any link to the relocated psmux reference to `../reference/psmux_scripting.md`, and any link to a deleted shared-lib doc (e.g. `state.md`) by dropping it or pointing at `../modules/mux.md`/overview as appropriate.
- **Commit:** `docs: move mux research logs to docs/research, psmux manual to docs/reference`

### Card 6: Sweep roadmap.md — drop registry/lands-with-mux, fix links

- **Context:**
  - `_mill/discussion.md`
  - `docs/overview.md`
- **Edits:**
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Milestone 3 (`internal/state`): reword from "Deferred to land with mux" to **landed** — `internal/state` (and `internal/fsx`) shipped in `ba81abf` as a generic locked-JSON helper; it simply has no consumer yet. Mark ✅ Done.
  - Milestone 4 (worktree): remove the "state-backed registry … deferred with `internal/state` (milestone 3 → lands with mux)" framing and the implication that `list` is a placeholder; state the module is **stateless by design** — `lyx worktree list` is a thin `git worktree list` wrapper and there is no registry. Keep the portals-deprecated / removed-in-task-006 note.
  - Milestone 8 (mux v1): remove "laid out from the worktree registry" — the worktree module is stateless; mux derives worktree layout from `git worktree list`. Keep mux's own design intent.
  - Fix links: any link to the deleted `modules/{board,worktree,ide,muxpoc}.md` → repoint to `overview.md#modules` (or drop, describing inline); the `modules/mux-hooks-exploration.md` link → `research/mux-hooks-exploration.md`. Keep links to `overview.md`, `modules/mux.md`, `shared-libs/README.md`.
- **Commit:** `docs(roadmap): mark state landed, drop worktree-registry framing, fix links`

### Card 7: Sweep README.md — landed set + Libraries link list

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/shared-libs/README.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Opening line currently lists user-facing modules as "board, worktree, mux" — correct it to the actual landed set (board, worktree, ide, muxpoc shipped; mux is design).
  - The Libraries link list (≈L18-24) links all seven lib docs including the five deleted ones (`fsx`, `git`, `gitignore`, `lock`, `state`) — rewrite it to link only the surviving lib docs (`config.md`, `paths.md`); for the deleted libs, either drop the link and keep a one-line inline description, or describe them as documented in code+tests per the doc-lifecycle convention. Do not leave links to deleted files.
- **Commit:** `docs(shared-libs): fix README module set and Libraries link list`

### Card 8: Sweep config.md — drop unlanded/state references and Container vocab

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/shared-libs/config.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Remove the `.lyx/local-state.json` reference (≈L19-20) from the layout diagram — no code reads it; the worktree module is stateless.
  - The `mux.yaml` reference (≈L14) is for an unlanded module — either drop it or clearly flag it as a future/unlanded config file, not a current artifact.
  - Replace "config container" / lowercase "container" vocabulary (≈L10) with **Hub** terminology to match overview.md and paths.md.
- **Commit:** `docs(shared-libs): drop local-state.json/mux.yaml refs, Container→Hub in config.md`

### Card 9: Sweep paths.md — Prime vocab, Container→Hub, portal deprecation

- **Context:**
  - `_mill/discussion.md`
  - `docs/overview.md`
- **Edits:**
  - `docs/shared-libs/paths.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - L60 struct comment `// path to the main/first worktree checkout` → use **Prime** vocabulary (e.g. `// path to the Prime worktree checkout (main branch)`).
  - L58 `// top-level container directory …` and the prose/algorithm at ≈L3/L14/L41 using lowercase "container" → **Hub** (keep meaning; the Hub is the top-level non-git directory).
  - Keep all three portal methods documented but tag them **deprecated-but-present (removed in task 006)**, consistent with overview.md and CONSTRAINTS.md. Do not remove them — portal code still exists.
- **Commit:** `docs(shared-libs): Prime/Hub vocab and portal deprecation in paths.md`

### Card 10: Sweep mux.md — Hub/Prime, drop worktree-registry coupling, fix links

- **Context:**
  - `_mill/discussion.md`
  - `docs/overview.md`
- **Edits:**
  - `docs/modules/mux.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Terminology: Container→Hub, "main/first worktree"→Prime throughout.
  - Per `mux-registry-semantics`: at L21-23, **drop the worktree-registry coupling** — remove framing that worktree and mux "share the same state document" / mux is "laid out from the worktree registry," and **remove the dead link to the deleted `shared-libs/state.md`** (drop it or point at `internal/state` in code). **Keep** references to mux's *own* planned session/pane state document, and **keep** the live `../shared-libs/config.md` link that sits alongside the removed `state.md` link. State that mux derives worktree layout from `git worktree list` (worktree is stateless).
  - Fix links broken by this task's moves/deletes: links to the moved research docs become `../research/<name>.md`; the psmux reference becomes `../reference/psmux_scripting.md`. mux.md stays in `docs/modules/` and is not deleted (unbuilt module's design draft).
- **Commit:** `docs(mux): Hub/Prime vocab, drop worktree-registry coupling, fix links`

### Card 11: Fix the dangling benchmarks link

- **Context:**
  - `docs/overview.md`
- **Edits:**
  - `docs/benchmarks/board-performance.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - L6 links `[board.md](../modules/board.md#background-sync)`, which the board.md deletion breaks → retarget to overview's board entry, `../overview.md#modules` (link text may keep referring to the board module). This is the only edit to this file; its benchmark data/content stays untouched.
- **Commit:** `docs(benchmarks): retarget dead board.md link to overview`

## Batch Tests

`verify: null` — pure-docs batch. The substantive verification is run by the implementer after the cards, from the worktree root (git-bash):

- **Broken-link sweep (expect zero output):** scan all docs + CONSTRAINTS for links to deleted/moved files —
  `grep -rEn "modules/board\.md|modules/worktree\.md|modules/ide\.md|modules/muxpoc\.md|shared-libs/git\.md|shared-libs/lock\.md|shared-libs/fsx\.md|shared-libs/gitignore\.md|shared-libs/state\.md|vendor/psmux_scripting\.md|modules/mux-exploration\.md|modules/mux-hooks-exploration\.md|modules/mux-proposal\.md" docs/ CONSTRAINTS.md`
  Any hit is a dangling link to fix. Also confirm the moved research docs no longer link to a bare `mux.md` (must be `../modules/mux.md`).
- **Stale-term sweep (expect zero in kept docs):**
  `grep -rEn "local-state\.json|lands.with.mux|main/first worktree" docs/overview.md docs/roadmap.md docs/shared-libs/README.md docs/shared-libs/config.md docs/shared-libs/paths.md CONSTRAINTS.md`
  and a worktree-"registry" check across the same kept set. Per the `mux-registry-semantics` exemption, `docs/modules/mux.md` may keep references to mux's *own* state document but must be free of worktree-registry framing and must not link `state.md`.
- Confirm no kept doc still marks `internal/state` as "(planned)" and that capitalized "Container" (as the old name for Hub) does not appear.
- Confirm `docs/research/` holds the three mux logs and `docs/reference/` holds `psmux_scripting.md`.
