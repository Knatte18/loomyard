# CLAUDE.md — Loomyard (lyx)

## CONSTRAINTS.md is authoritative — read it, follow it

This repo has a `CONSTRAINTS.md` at its root. It exists, it is non-negotiable, and it
**MUST be read before writing or reviewing any code** and followed exactly. It encodes
structural invariants that are partly enforced at `go test` / CI time and partly by
review discipline — violating one breaks the build or silently rots the design.

Do **not** ever claim "no constraints in repo" or proceed as if there are none. The file
is there. If you have not read it this session, read it now (`CONSTRAINTS.md`). Current
invariants include: the **Hub Geometry Invariant** (`internal/hubgeometry` owns all cwd/geometry and
`_lyx`/config paths), the **lyxtest Leaf Invariant**, the **CLI / Cobra Invariant**
(module `Command()`/`RunCLI` seam, `Short` on every command, help-tree tests), and the
**Documentation Lifecycle**. When you add a new cross-cutting invariant, record it in
`CONSTRAINTS.md` in the same commit.

## Persistent notes, not file-memory

This project is worked in short-lived mill **worktrees** that get torn down once a task
merges. The file-based `memory/` store is per-worktree, so anything written there
vanishes with the worktree — don't bother saving project facts as memory. Put durable
notes where they get versioned and merged into `main` instead: this `CLAUDE.md`,
`_codeguide/`, or code comments.

## Pushing to main is OK

Pushing directly to `main` is fine in this repo — no PR or branch-first gate. Small,
self-contained changes (doc moves, fixups) may be committed and pushed straight to
`main`. This overrides the global "branch first / commit only when asked" default.

## Mill wiki

Never write to the mill wiki directly. Absolutely all interaction with the mill
wiki goes through mill's wiki module — never raw `git` on the wiki, never `Edit`/`Write`
on wiki files (`Home.md`, `_Sidebar.md`, `proposal-*.md`, `tasks.json`), and never
`cp`-into-wiki. Use the daemon client (`wiki._client`: `upsert_task`, `set_phase`,
`merge_tasks`, `list_tasks_*`) or the `/mill-*` skills (`mill-add`, `mill-groom`,
`mill-wiki-push`, …). The daemon owns the wiki repo and serializes every write.

## Agent execution: interactive psmux sessions, NOT `claude -p`

Every LLM agent lyx spawns (loom producers, the review handler, cluster reviewers,
the progress-judge) runs as an **interactive session inside psmux** — never headless
`claude -p`.

**Why (economic, not technical):** Anthropic announced that headless `claude -p` will
no longer draw on a Pro/Max subscription — it will be billed as API and reserved
interactive sessions for the subscription. (Slated for 2026-06-15, postponed, but
expected to land.) Headless is technically possible but would force API cost, so we do
not use it. Interactive sessions keep subscription coverage; psmux is what makes a
programmatically-driven session *interactive*.

**Consequences for design:**
- The orchestrator drives agents by launching an interactive session in a psmux
  pane/window, injecting the prompt, and detecting completion via Claude Code hooks.
  I/O still rides the **file contract** (the agent writes its output files; Go reads
  them) — that part is unchanged from a headless model.
- Therefore `internal/agent` depends on the **mux** module; it cannot be built purely
  on a headless `exec`. mux is on loom's critical path for this reason.
- Agents are provider-agnostic via **engines** — per-LLM adapters (a Claude engine now;
  Gemini etc. later) that know how to launch/drive their provider as a psmux session.
  The verdict/output contract is provider-invariant, which is what makes engines
  swappable. **Non-Claude support is not a current priority.**
- Cluster-reviews (N parallel reviewers) scale via psmux **windows** (spawned clusters
  land in their own windows, not a pane explosion) — long-term mux work, not now.

## Task completion

Every task that adds a module, changes observable CLI behaviour, or introduces
cross-cutting infrastructure **must update docs as part of the same commit** —
not as a follow-up. Specifically:

- Update or create the module doc in `docs/modules/` if the change touches a named
  module's design.
- Update `docs/overview.md` if the module table or execution stack changes.
- Record new cross-cutting invariants in `CONSTRAINTS.md` (same commit).

A commit that ships behaviour without updating the docs is incomplete. The docs
are the shared reference — they rot the moment the code moves without them.

**`docs/roadmap.md` is for planned milestones only.** Update it only when a task
**completes a planned milestone** (mark it ✅ Done, with a link to the module doc
if one exists) or **adds a new planned milestone**. Do *not* append notes to the
roadmap for bugfixes, hardening, or ergonomics/polish passes — that is delivered
work, not a planned goal, and a collection of fixes is not its own milestone.
Such changes are recorded by git history, the relevant module doc, and
`CONSTRAINTS.md` invariants — not by the roadmap.

## Filesystem links (fslink)

All cross-OS links go through `internal/fslink`. On Windows it uses **directory
junctions** (mount-point reparse points), which need no special privileges; on other
platforms it uses symlinks. The cross-platform contract is **directory-only**:
`fslink.CreateDirLink` is the entry point, and a `CreateFileLink` is reserved for the
future. Do not rely on Windows **file** symlinks — they require admin / Developer Mode
and are not available on every dev machine, so junctions (directory links) are the only
link type guaranteed to work everywhere.
