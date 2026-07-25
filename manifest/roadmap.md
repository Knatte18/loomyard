# Roadmap: Loomyard

Loomyard replaces mill/millhouse (Python) with a Go orchestration layer, built as
self-contained modules landed one at a time. See [docs/overview.md](../docs/overview.md#principles)
for the design principles. This file is a numbered list of what's planned, what's committed-to-
but-unscheduled, and what's shipped — for the detailed design of anything not yet built, see its
doc under [designs/](designs/). See Maintenance below for how the numbering works.

## Planned

Committed to, in this order, next.

1. **fabric: cutover** — rewires every consumer currently calling into `warp`/`weft`
   (`initengine`, `loomengine`, `buildercli`, `webstercli`, `perchcli`, `configcli`) onto the
   already-built `fabric` (`internal/fabricengine` + `internal/fabriccli`, validated by
   differential tests against `warp`/`weft` as the reference fixture), then deletes the old
   `warp`/`weft` modules in one coordinated pass — not incremental, since the two old modules are
   tightly coupled to how git state is read across the codebase today. Connecting `fabric` into
   the actual system is what this item is — the parallel build itself already landed. See
   [designs/fabric.md](designs/fabric.md).

1. **webster: rewrite for flat card list** — fork-per-card unchanged; no DAG/SCC in v0 (a dead
   `HasSymbolFields()` scheduler branch is reserved for later); integration suite runs as one final
   fork with SHA-bisect on failure. `builder` becomes obsolete as a plan-format consumer. See
   [designs/webster-rewrite.md](designs/webster-rewrite.md).

1. **board: move storage to `weft:main`** — replaces board's own separate remote repo with a
   reserved `weft:main` branch (README.md rendering, JSON-backed Proposals/Manifest/Tasks/Done).
   Depends on the Planned `fabric: cutover` item's branch-naming enforcement (`<slug>-weft`
   uniformly) actually taking effect, not just `fabric`'s code existing alongside the old
   modules. See [designs/board-weft-storage.md](designs/board-weft-storage.md).

1. **loom: phase-machine skeleton + session bootstrap** — the status-file-driven engine
   (sequencing, resume, crash-recovery, pause), testable against fake phases before real
   producers are wired in, plus the `lyx loom run` entry point. See
   [designs/loom.md](designs/loom.md).

1. **loom: Finalize phase** — merge-back after Builder-review approval; Go-first, LLM only on
   merge conflict; optional PR creation. See [designs/loom-finalize.md](designs/loom-finalize.md).

1. **native clients: migrate `gitrepo` to `go-git` (ADOPT-PARTIAL) + `selfreportengine`'s internal
   `gh`-CLI transport to `go-github`** — executes the `git-native-library` spike's finding (read
   surface, both commit methods, and `SetSnapshotSHA` migrate cleanly; `Push`'s rebase-retry stays
   CLI-bound permanently — go-git has no rebase) into `internal/gitrepo`, and separately swaps only
   what's underneath `selfreportengine`'s public `CreateIssue` entry point — its `gh`-CLI shell-out
   — for `google/go-github`, for the same "stop parsing CLI output as an API" reason, on a much
   smaller, already-stable surface (no spike needed). `CreateIssue`'s signature/behavior and all its
   callers are unaffected. One task, since both are the same underlying cleanup. See
   [designs/native-clients-migration.md](designs/native-clients-migration.md).

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

1. **gorch: shared orchestrator engine for `perch` + `hardener`** — generalizes perch's existing
   judge/gate/round-spawn/cap/pause/lock loop into a shared engine with a pluggable round-runner
   (`burlerengine` for perch, a live-substrate agent for hardener) and a judge-maintained handoff
   (bounds perch's own O(N) review-history growth too, not just a hardener need). A dedicated
   discussion round must pin the design before either perch gets rewritten onto it or hardener gets
   built on it — not folded into hardener's own task. See [designs/gorch.md](designs/gorch.md).

1. **hardener** — behavior-based hardening of a live-substrate module (the archetype: `reed` driving
   real tmux) in a sandbox repo, on-demand and post-loom, off the `shuttle → burler → perch → loom`
   spine. Concept still being figured out; its orchestrator design is now shared with `perch` via
   the Someday `gorch` item above. See [designs/hardener.md](designs/hardener.md) (a DRAFT doc, do
   not implement from it yet).

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

1. **self-report: two-tier friction capture** — loom's per-phase file-contract design means no
   single LLM session has full-run context the way Millhouse's self-report assumes. Splits into
   Go-detected structural anomalies (crash-resumes, stuck escalations, repeated review rounds — off
   loom's own status/history, no LLM needed) plus a narrow per-phase friction note any spawned agent
   may write about its own scoped task, aggregated by Go and fed to one dedicated reflection agent
   at natural end points (Finalize/stuck) — mirroring the `Raddle` pattern. See
   [designs/self-report.md](designs/self-report.md).

1. **`PATTERN.md`** — a loomyard-owned equivalent of Millhouse's `CONSTRAINTS.md`, written from
   scratch (not a port) once loomyard starts dogfooding its own development onto `loom`. Format:
   short two-line entries (constraint + pointer), full rule/rationale/enforcement detail in a
   linked per-topic doc. Millhouse's own `CONSTRAINTS.md` stays untouched for as long as Millhouse
   develops loomyard.

## Done

1. **git-native-library: feasibility spike** — empirical spike evaluating a native Go git library
   (`go-git`) as a replacement for `internal/gitexec`'s shell-out plumbing, across the full surface
   `gitrepo` uses (reads and writes, including the `Push` rebase-retry path). Recommendation:
   ADOPT-PARTIAL — the read surface, both commit methods, and `SetSnapshotSHA` migrate cleanly;
   the rebase-retry recovery on a rejected push stays CLI-BOUND because go-git ships no rebase
   implementation. The kept prototype and its findings write-up live in
   `internal/gitnativepoc/doc.go`; no consumer was migrated.

1. **board** — task tracker (storage model superseded by the Planned `board` item once it ships).

1. **board: use `gitrepo` as its git operator** — board's detached sync (`internal/boardengine`)
   talks to git exclusively through a single `gitrepo.Repo` (`StageAllAndCommit` +
   `PushCoalesced`) under board's own write and push locks, replacing its former hand-rolled
   `gitexec` calls.

1. **shared infra** — `internal/configengine`, `internal/gitexec`, `internal/lock`,
   `internal/state`.

1. **gitrepo** — generic, repo-agnostic git primitives (`StageAndCommit`, `Push`,
   `PushCoalesced`, `CurrentSHA`, `ChangedFilesSince`, `SHAExists`, `SnapshotSHA`/
   `SetSnapshotSHA`) built on `internal/gitexec` (`internal/gitrepo`; consumed by the Planned
   `fabric` item once it ships).

1. **worktree + ide** — worktree/portal management, VS Code launcher (worktree itself superseded by
   `warp`).

1. **weft** — companion weft repo, paired host+weft spawn/teardown (superseded by the Planned
   `fabric` item once it ships).

1. **config TUI** — `lyx config` interactive menu + `reconcile`.

1. **warp** — host↔weft-coordinated git topology (clone, add/remove, checkout, reconcile, cleanup)
   (superseded by the Planned `fabric` item once it ships).

1. **proc** — cross-OS process spawn.

1. **reed** — tmux overlay + strand bookkeeping + render (renamed from `mux`, no behavior change).

1. **shuttle** — run one LLM agent as an interactive tmux strand over a swappable engine.

1. **burler** — one review+fix round (A-review → B-fix).

1. **perch** — the gate loop: run `burler` rounds until `APPROVED`/`STUCK`.

1. **builder** — batch-implementation loop over a pinned plan (sequential, one strand per batch) —
   superseded as an active plan-format consumer once the Planned `webster: rewrite for flat card
   list` item ships.

1. **webster** — fork-based sibling of builder (in-session forks, one Master per plan) — rewrite
   tracked under the Planned `webster: rewrite for flat card list` item.

1. **plan-format v3: flat card list** — a card carries
   `card`/`name`/`description`/`changes-files`/`depends-on` only; symbol fields wait for
   `codeintel`. Coexists with the still-live
   [plan-format v2](../docs/reference/plan-format.md) until `webster`'s rewrite lands and
   `builder` is deleted. See [docs/reference/plan-format-v3.md](../docs/reference/plan-format-v3.md).

1. **built-in CLI help** — self-documenting `lyx`/`lyx <module>`/`lyx <module> <cmd> --help`.

1. **selfreport** — file Loomyard bugs as GitHub issues (`lyx selfreport create`).

1. **loom: contracts, Preflight, Discussion producer** — the three loom pieces shipped so far (loom
   as a whole is not done — see the Planned `loom` item).

1. **loom: Planner producer** — reads the discussion decision-record and writes a
   plan-format-v3 flat-card plan; a prompt/profile fed to `shuttle.Run` (not a module), the
   `PlanSpec(...)` factory + `plan-template.md` in `internal/loomengine`. No review logic of
   its own.

1. **dev/test `lyx.exe` separated from production deploy** — a second deploy target
   (`deploy-dev`/`deploy-dev.cmd`) so review/sandbox tooling never overwrites the stable
   production binary with an in-progress test build. See CONSTRAINTS.md's Dev/Prod Binary
   Separation invariant.

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
