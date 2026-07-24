# Roadmap: Loomyard

Loomyard replaces mill/millhouse (Python) with a Go orchestration layer, built as
self-contained modules landed one at a time. See [docs/overview.md](../docs/overview.md#principles)
for the design principles. This file is a numbered list of what's planned, what's committed-to-
but-unscheduled, and what's shipped — for the detailed design of anything not yet built, see its
doc under [designs/](designs/). See Maintenance below for how the numbering works.

## Planned

Committed to, in this order, next.

1. **gitrepo** — generic, repo-agnostic git primitives (`StageAndCommit`, `Push`, `CurrentSHA`,
   `ChangedFilesSince`, `SHAExists`, `SnapshotSHA`/`SetSnapshotSHA`), built on the existing
   `internal/gitexec` command-execution layer. Lands and gets tested standalone, before `fabric`
   consumes it. See [designs/gitrepo.md](designs/gitrepo.md).

1. **board: use `gitrepo` as its git operator** — rewires board's existing hand-rolled git
   plumbing (`internal/boardengine/git.go`, `sync.go`'s detached sync) onto `gitrepo.Repo`
   instead. Depends only on `gitrepo`, not `fabric` — can be built in parallel with it. Distinct
   from the **board: move storage to `weft:main`** item below (that one changes *where* board
   stores data; this one only changes *how* it talks to git). See
   [designs/board-use-gitrepo.md](designs/board-use-gitrepo.md).

1. **fabric** — replaces `warp` and `weft` in full: all topology (clone, dual-worktree add/remove,
   coordinated checkout, reconcile, prune, cleanup, branch naming — including enforcing
   `<slug>-weft` uniformly, no exceptions) and all git mechanics into the paired weft repo, unified
   into one module built on `gitrepo`. Built alongside the existing `warp`/`weft` code first as a
   reference fixture, then one coordinated cutover deletes the old modules. See
   [designs/fabric.md](designs/fabric.md).

1. **plan-format v3: flat card list** — replaces the pinned, batch-based
   [plan-format.md v2](../docs/reference/plan-format.md). A card carries
   `card`/`name`/`description`/`changes-files`/`depends-on` only; symbol fields wait for
   `codeintel`. Breaking change to an already-shipped contract. See
   [designs/plan-format-v3.md](designs/plan-format-v3.md).

1. **webster: rewrite for flat card list** — fork-per-card unchanged; no DAG/SCC in v0 (a dead
   `HasSymbolFields()` scheduler branch is reserved for later); integration suite runs as one final
   fork with SHA-bisect on failure. `builder` becomes obsolete as a plan-format consumer. See
   [designs/webster-rewrite.md](designs/webster-rewrite.md).

1. **board: move storage to `weft:main`** — replaces board's own separate remote repo with a
   reserved `weft:main` branch (README.md rendering, JSON-backed Proposals/Manifest/Tasks/Done).
   Depends on fabric's branch-naming enforcement (`<slug>-weft` uniformly). See
   [designs/board-weft-storage.md](designs/board-weft-storage.md).

1. **mux → reed** — rename, no behavior change. See [designs/mux-to-reed.md](designs/mux-to-reed.md).

1. **loom: Planner producer** — converts `discussion.md` into a plan-format-v3 card list; no
   inputs beyond `discussion.md`, no review logic of its own. See
   [designs/loom-planner.md](designs/loom-planner.md).

1. **loom: phase-machine skeleton + session bootstrap** — the status-file-driven engine
   (sequencing, resume, crash-recovery, pause), testable against fake phases before real
   producers are wired in, plus the `lyx loom run` entry point. See
   [designs/loom.md](designs/loom.md).

1. **loom: Finalize phase** — merge-back after Builder-review approval; Go-first, LLM only on
   merge conflict; optional PR creation. See [designs/loom-finalize.md](designs/loom-finalize.md).

1. **dev/test `lyx.exe` separated from production deploy** — a second deploy target so
   review/sandbox tooling never overwrites the stable production binary with an in-progress test
   build. See [designs/dev-test-binary.md](designs/dev-test-binary.md).

## Someday

Committed to eventually — will be done — but not scheduled next. No build order is implied
between these items.

1. **doctor** — diagnostics command (`lyx doctor`): checks `_lyx/` layout, config parse, board
   reachability, stale locks.

1. **session sync** — copy Claude `.jsonl` transcripts across machines so `--resume` works
   elsewhere.

1. **Claude Code plugin packaging** — ship `lyx` as an installable plugin.

1. **reed: cross-worktree columns** — all worktrees in one window, a column per worktree.

1. **reed: daemon → Slack relay** — standalone watchdog + bidirectional Slack relay per worktree.

1. **reed: own-window strand anchoring** — a `display` anchor that spawns a strand into its own
   switchable tmux window instead of a pane.

1. **Real-Linux validation** — run the sandbox suite and validate every tmux/`/proc` assumption on
   a real Linux box (built and cross-compiled so far, never executed there).

1. **codeintel** — full four-layer design (toolchain manager, daemon/supervisor, LSP client,
   language registry) exists; deprioritized until loom's first end-to-end run lands. See
   [designs/codeintel-redesign.md](designs/codeintel-redesign.md).

1. **raddle** — codeguide's woven-in successor; parallel-regeneration design exists; deferred phase
   slot between Builder and Finalize. See [designs/raddle.md](designs/raddle.md).

1. **webster: parallel card execution** — worktree-per-card concurrent forking with a DAG;
   explored twice (pre- and during vacation discussion), rejected both times for git-index-race and
   mid-flight-visibility hazards. See
   [designs/webster-parallel-execution.md](designs/webster-parallel-execution.md).

1. **hardener** — behavior-based hardening of a live-substrate module (the archetype: `mux` driving
   real tmux) in a sandbox repo, on-demand and post-loom, off the `shuttle → burler → perch → loom`
   spine. Concept still being figured out. See [designs/hardener.md](designs/hardener.md) (a DRAFT
   doc, do not implement from it yet).

1. **host-visibility: CLAUDE.local.md / CONSTRAINTS.md invisible in host's git history** — a
   `CONSTRAINTS.md`-equivalent directory via junction, and `CLAUDE.local.md` via symlink (with a
   Windows-Developer-Mode note and a copy fallback), so nothing lyx-related shows up in host's own
   git history. See [designs/host-visibility.md](designs/host-visibility.md).

1. **reed daemon: foreign-pane self-heal** — extends the **reed: daemon → Slack relay** item. Today
   reed is one-shot, so an operator-split or stray "faux" pane is only reaped on the *next* reed
   verb; the daemon could reconcile on its own. Prefer event-driven tmux hooks
   (`after-split-window`/`window-layout-changed`) over polling; gate behind a policy that
   distinguishes a bug-induced faux pane from an operator's intentional scratch pane. Prerequisite:
   make the reap probe cheaper first (it currently spawns a fresh pwsh + full `Win32_Process` WMI
   enumeration per poll).

1. **shuttle `Spec`: generic tools-restriction** — meaningless for today's single-session A→B
   agent; cluster reviewers turned out to be fork subagents inside the handler's own session
   (`useExactTools`), not separate sessions needing their own `settings.json`, so this stays
   unmotivated rather than blocked on anything.

1. **shuttle `Spec`: per-round provider selector** — today "provider" means whichever engine is
   wired into the `Runner`; a selector field is only needed once a second engine lands (non-Claude
   engines are not a current priority, per `CLAUDE.md`).

1. **Bulk-mode clusters + provider-side context caching** — a `burler` cluster round can run
   *tool-use* or *bulk* (Go concatenates target + fasit + rubric into one blob). Bulk is what makes
   provider-side context caching (e.g. Gemini's explicit cache) pay off, and only if modelled as
   one shared prefix + N distinct suffixes, never N full prompts.

1. **semantic-index** — semantic search over docstrings/comments (Enzyme-inspired: catalysts +
   embeddings + temporal decay), to find code by concept rather than literal keyword. The
   "deferred idea" `codeintel-redesign.md` already refers to. Genuinely speculative, not yet
   designed in depth. See [designs/semantic-index.md](designs/semantic-index.md).

1. **`PATTERN.md`** — a loomyard-owned equivalent of Millhouse's `CONSTRAINTS.md`, written from
   scratch (not a port) once loomyard starts dogfooding its own development onto `loom`. Format:
   short two-line entries (constraint + pointer), full rule/rationale/enforcement detail in a
   linked per-topic doc. Millhouse's own `CONSTRAINTS.md` stays untouched for as long as Millhouse
   develops loomyard.

## Done

1. **board** — task tracker (storage model superseded by the Planned `board` item once it ships).

1. **shared infra** — `internal/configengine`, `internal/gitexec`, `internal/lock`,
   `internal/state`.

1. **worktree + ide** — worktree/portal management, VS Code launcher (worktree itself superseded by
   `warp`).

1. **weft** — companion weft repo, paired host+weft spawn/teardown (superseded by the Planned
   `fabric` item once it ships).

1. **config TUI** — `lyx config` interactive menu + `reconcile`.

1. **warp** — host↔weft-coordinated git topology (clone, add/remove, checkout, reconcile, cleanup)
   (superseded by the Planned `fabric` item once it ships).

1. **proc** — cross-OS process spawn.

1. **mux** — tmux overlay + strand bookkeeping + render (renamed by the Planned `mux → reed` item
   once it ships).

1. **shuttle** — run one LLM agent as an interactive tmux strand over a swappable engine.

1. **burler** — one review+fix round (A-review → B-fix).

1. **perch** — the gate loop: run `burler` rounds until `APPROVED`/`STUCK`.

1. **builder** — batch-implementation loop over a pinned plan (sequential, one strand per batch) —
   superseded as an active plan-format consumer once the Planned `webster: rewrite for flat card
   list` item ships.

1. **webster** — fork-based sibling of builder (in-session forks, one Master per plan) — rewrite
   tracked under the Planned `webster: rewrite for flat card list` item.

1. **built-in CLI help** — self-documenting `lyx`/`lyx <module>`/`lyx <module> <cmd> --help`.

1. **selfreport** — file Loomyard bugs as GitHub issues (`lyx selfreport create`).

1. **loom: contracts, Preflight, Discussion producer** — the three loom pieces shipped so far (loom
   as a whole is not done — see the Planned `loom` item).

## Maintenance

- **Numbering is automatic, not manual, and restarts at 1 in each section.** Every item is written
  literally as `1.` in the source — GitHub/CommonMark renders ordered-list items sequentially from
  the first item in a contiguous list block regardless of the literal digit on the rest, and a new
  `##` heading starts a new block. So Planned, Someday, and Done each render as their own 1, 2, 3,
  … with **zero number edits ever needed** — inserting, removing, or reordering items anywhere just
  works.
- **Numbers are not stable cross-reference IDs** (the same number exists in all three sections).
  Cross-reference by **bold item name** instead (e.g. "the Planned `fabric` item," "Someday's
  `codeintel` item") — every reference elsewhere in this file and in `designs/*.md` already does
  this.
- Move an item from Planned or Someday to Done, with a link to its module doc if one exists, when
  it ships — no renumbering needed anywhere.
- Delete a module's doc under `designs/` once it ships (see the
  [documentation lifecycle](../docs/overview.md#documentation-lifecycle)) — that's why Done entries
  above don't link anywhere.
- Someday items get a `designs/<name>.md` doc when there's real design behind them (`codeintel`,
  `raddle`, `webster: parallel card execution`, `hardener`, `host-visibility`, `semantic-index`
  above do); trivial
  ones don't need one until they're promoted to Planned.
- This file is the single home for everything not scheduled, whether firmly committed to
  (`codeintel`, `raddle`) or genuinely speculative (`hardener`, the shuttle `Spec` ideas) — no
  separate long-term-ideas file. Add new speculative ideas directly to Someday.
