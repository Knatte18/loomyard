# Roadmap: Loomyard

Loomyard will, in time, **replace mill/millhouse (Python) entirely** — the Go
infrastructure becomes the orchestration layer, and the mill skills get reworked
and trimmed in the same move.

We get there by building **toolkits first**: small, self-contained modules with
deep internal tests, landed one at a time so the operator keeps full control at
every step. The toolkit layer (board, worktree, weft, ide, config) is largely
done. What remains splits into two tracks:

- **Setup track** — finish bootstrapping a hub: config TUI, board-repo creation,
  `doctor`. (`warp clone` handles the clone step — no standalone `ly-git-clone`.)
- **Orchestration stack** — the part that ties worktrees, the board, and psmux
  into a spawn→review→merge lifecycle. This used to be a single distant "endgame";
  it is now a **designed, layered path**: `proc → mux → shuttle → review → loom`
  (see the [execution stack](overview.md#execution-stack-orchestration-layers)
  and [modules/loom.md](modules/loom.md)). Each layer is its own shippable
  milestone; mill's existing Agent Dispatch handles orchestration until `loom`
  lands.

See [overview.md](overview.md#principles) for the design principles these
milestones follow.

## Build order

The dependency-ordered sequence — what is actually buildable next, respecting what each layer
needs below it. The numbered [Milestones](#milestones) below carry the detail; this is the
at-a-glance order.

**Done foundation:** board → shared infra (`configengine`/`git`/`lock`) → `state` → worktree + ide →
weft engine + producers → **`proc`** (cross-OS spawn). ✅

**Orchestration spine** — a strict chain, each layer needs the one before it:

```
proc ✅ ──▶ mux ✅ ──▶ shuttle ──▶ review ──▶ loom
```

- **`mux`** is done — psmux overlay + **strand** bookkeeping + render sub-package
  (`internal/muxcli` + `internal/muxengine` + `internal/muxengine/render`; it absorbs what earlier
  drafts split into `shed`/`glance`). See [milestone 9](#orchestration-stack) /
  [modules/mux.md](modules/mux.md).
- **`shuttle`** needs `mux` and is next — its only dependency is done. **`review`** needs
  `shuttle`; **`loom`** needs `review`. That is the critical path to the orchestrator. `lyx loom
  status` (the 1-line view) ships as a loom subcommand, not a module.

**Setup track** — independent of the spine, interleave at any time: config TUI (in progress) ·
`init`/board-repo creation · `doctor`.

**Deferred** — after `loom` works and only if wanted: mux daemon → Slack relay; session sync;
plugin packaging.

So the immediate front: **`shuttle`** (unblocks the rest of the spine) in parallel with finishing
the **config TUI** — none of which block each other.

## Milestones

Each milestone is independently shippable. Refactor milestones (2–4) are
**behaviour-preserving**: board's existing test suite is the guardrail, so nothing
observable changes until the new module that needs the extracted lib arrives.

1. **board** — the task tracker. ✅ **Done.** See the board module in
   [overview.md#modules](overview.md#modules).

2. **Extract shared infrastructure: `internal/configengine`, `internal/git`,
   `internal/lock`.** ✅ **Done.** See
   [shared-libs/](shared-libs/README.md).

3. **`internal/state`.** Generic locked typed JSON I/O primitive: `WriteJSON[T]` / `ReadJSON[T]` with
   exclusive/shared locking on `path + ".lock"` via `internal/lock` and atomic writes
   via atomic filesystem operations. No fixed schema — callers own the fields
   and file paths. Built test-first. ✅ **Done.** A generic locked-JSON helper, shipped as part
   of the shared infrastructure with no consumer yet.

4. **worktree module + portals, launchers, and ide module.** ✅ **Done.** Create / track / tear down
   git worktrees; manage container junctions and spawnable
   launchers; VS Code launcher with interactive menu; centralized path geometry
   in `internal/hubgeometry`. Consumes `internal/configengine` + `internal/git`; owns the **junction-aware teardown**
   sequence (the Windows locked-worktree hazard). The module is **stateless by design** — `lyx worktree list` is a thin
   `git worktree list` wrapper; there is no worktree registry. Introduces `internal/hubgeometry` as the sole geometry owner, banning
   raw `os.Getwd` and `git rev-parse --show-toplevel` outside `internal/hubgeometry` and `cmd/lyx/main.go`
   via `internal/hubgeometry/enforcement_test.go`. (Portals are present and working — a subdir-mirrored
   Hub view of each worktree's `_lyx/`; kept available, not slated for removal.)

5. **Task 006 — Weft engine.** ✅ **Done.** Path geometry for weft worktrees, paired host+weft spawn and teardown, `lyx weft` command (`status|commit|push|pull|sync`).
   Implements the canonical weft overlay model (host stays pristine, all lyx artifacts in companion weft repo).
   Weft directories are reached by direct sibling access; portals remain available as the cross-worktree status view.
   The weft **producers** (the `lyx worktree add` paired host+weft spawn) also landed.

6. **Hub-creator / clone.** ✅ **Done — absorbed by `warp` (milestone 20).** `lyx warp clone <host> <weft>` creates the Hub and clones host, weft, and board. The standalone `lyx git-clone` subcommand was never built; warp made it redundant.

7. **Task 008 — configuration TUI.** 🚧 Mostly shipped / in progress. `lyx config` and
   `lyx config <module>` — an interactive menu over the `_lyx/config/` YAML schema.
   **`_codeguide` junction activation is split out and deferred** to a separate later task
   (tracked on the board) — it is no longer part of this milestone.

### Orchestration stack

The concrete path to the orchestrator, replacing the old single "endgame" milestone.
Each layer knows only the one below it; built bottom-up. See the
[execution stack](overview.md#execution-stack-orchestration-layers).

8. **`internal/proc`.** ✅ **Done.** Cross-OS windowless/detached process spawn — the OS
   base every higher layer launches through (build-tagged `proc_windows.go` / `proc_other.go`;
   third member of the portability family after `fsx` and `fslink`).

9. **`internal/mux` — the window to the world.** ✅ **Done.** Three things in one, split across
   `internal/muxcli` + `internal/muxengine` + `internal/muxengine/render`: **overlay** over psmux
   (panes, send-keys/capture, env hygiene, native `--resume`, one named server per hub — orphan
   firewall — with one session per worktree); **strand bookkeeping** (each managed process is a
   strand — a `guid`-keyed record with name, worktree slug, parent, opaque `cmd`/`resumeCmd`,
   generic display spec — persisted to `.lyx/mux.json`); and a **render** sub-package
   (`internal/muxengine/render`, `Rules(strands, box) -> (layout, focus)` over a closed generic
   display vocabulary — a pure-function, golden-file test surface, heights fully derived). Callers
   hand it `{cmd, name, display}`; mux never learns a domain `type`. Scope: one terminal per
   worktree (cross-worktree columns deferred). It absorbs what earlier drafts split into
   `shed`/`glance`. CLI verbs: `up`, `add`, `remove`, `status`, `attach`, `resume`, `down`.
   ([modules/mux.md](modules/mux.md)) **Built on what muxpoc proved** — clean-env boot,
   interactive claude, child-pane spawn, bottom-dominant layout, and resume after `kill-server`;
   muxpoc itself is now parked ([overview.md#modules](overview.md#modules)).

10. **`internal/shuttle` — one LLM agent via a swappable engine.** Run a single agent in a strand
    over the file contract; `Stop`-hook completion; `PreToolUse` guardrails (deny in-process `Agent`
    + `AskUserQuestion`). The **engine** seam isolates the provider (Claude now; Gemini etc. later,
    not a priority). Named `shuttle`, not `agent`, to avoid colliding with Claude's own agent
    vocabulary. Asks `mux.AddStrand` for its pane. ([modules/shuttle.md](modules/shuttle.md))

11. **`review` (`lyx review`) — the gate engine.** Generic profile-driven reviewer: handler+fixer
    in one agent, optional cluster reviewers (own-window strands), a progress/circularity judge, and
    N-round stuck detection. One engine serves discussion / plan / builder / ad-hoc review — the
    per-type difference is the profile (rubric + fasit), not the code. **Independent of `loom`**
    (builds on `shuttle`, runs standalone); loom just uses it between every phase.
    ([modules/review.md](modules/review.md))

12. **`loom` (`lyx loom run`, alias `lyx run`) — the phase machine.** The autonomous driver:
    Setup → Discussion → Plan → Builder → Finalize, each gated by a review, resume-from-disk via
    the `_lyx/` status file, yielding only at human boundaries (or never, in `--auto`). **This is
    the orchestrator that finally replaces mill/millhouse** — the top of the stack above, sitting on
    board + worktree + weft + the `mux → shuttle` layers. `lyx run` is the **session bootstrap**:
    ensure the worktree's psmux session, add the `lyx loom status` strand (1-line top pane), spawn
    the driver detached (`proc`), attach the terminal; a `.lyx/lyxrun.cmd` launcher makes it one
    click. The 1-line view ships as the `lyx loom status` subcommand (a strand), not a module.
    ([modules/loom.md](modules/loom.md))

### Deferred mux enhancements

Layer in once the core stack works; not required for `loom` v1.

13. **Cross-worktree columns.** All worktrees in one window, a column per worktree — just a
    `worktree` strand field + a grouping rule on top of mux's strand model
    ([modules/mux.md](modules/mux.md#deferred)). Deferred only because one-terminal-per-worktree is
    the right starting scope.

14. **mux daemon.** Standalone watchdog process: detects a psmux crash via `cmd.Wait()`, recovers
    each strand by replaying its stored opaque `resumeCmd` (native `--resume` **works** for
    programmatically-driven Claude panes once the inherited Claude-Code parent-session env is
    stripped — see
    [modules/mux.md](modules/mux.md#resume-native---resume-via-the-stored-opaque-resumecmd); the
    capture journal is optional belt-and-suspenders, not the primary mechanism), mutual watchdog so
    both must die to go dark. See [modules/mux.md](modules/mux.md#deferred). **Proven in muxpoc,
    now built into mux's on-demand reconcile** ([overview.md#modules](overview.md#modules)).

15. **Slack relay.** Bidirectional, one channel per worktree, riding on the daemon.

### Setup & supporting milestones

Independent of the orchestration stack; interleave as needed.

16. **`init` grows: create the board repo from scratch.** `warp clone` already handles the
    "clone existing board repo" case (it clones all three repos). What remains: when starting
    fresh with no existing board remote, `init` should create and initialise a board git repo
    locally (and optionally push it to a new remote). Today the board dir is auto-created on
    first write and made a git repo by hand.

17. **doctor.** A diagnostics command (`lyx doctor`): checks `_lyx/` is present, `*.yaml` parse and
    use known keys, the board repo is reachable, no stale lock files, the state file is readable —
    and prints remediation. Pure troubleshooting, no domain logic.

18. **session sync.** `lyx session push/pull` — copy Claude `.jsonl` transcripts across machines so
    `claude --resume` works elsewhere (sessions are not portable today). See
    [modules/mux.md](modules/mux.md#session-files-and-portability-the-session-sync-milestone).

19. **Claude Code plugin packaging.** Ship `lyx` as an installable Claude Code plugin, exactly as
    mill/millpy were, once the binary and module architecture are proven.

20. **`warp` — host↔weft-coordinated git topology.** ✅ **Done.** Consolidated the
    host↔weft mirror invariant into one module: coordinated checkout (switches host+weft
    together + re-points junctions — the correctness gap raw `git checkout` left),
    dual-worktree add/remove, clone, reconcile, and cleanup. **Replaced** `worktree`
    (milestone 4), **folded in** `git-clone` (milestone 6), and **renamed** `internal/git`
    → `internal/gitexec` (the thin leaf both `weft` and `warp` sit on). The config module
    `worktree` → `warp` (`_lyx/config/warp.yaml`). The design doc was deleted on landing
    per the documentation lifecycle; durable parts live in the `internal/warpengine` package
    header and the [warp module entry](overview.md#modules) in overview.md.

21. **Built-in CLI help — self-documenting modules & commands.** ✅ **Done.** Cobra refactor of
    `cmd/lyx` + every module's `RunCLI`: `lyx` lists all modules; `lyx <module>` lists subcommands;
    `lyx <module> <cmd> --help` gives per-command usage. Help text lives co-located with each
    command (no central stale table). Introduces `internal/clihelp` (exec + JSON help). A persistent
    `--json` flag on the root command offers machine-readable help output.

22. **`selfreport` — file LoomYard bugs as GitHub issues.** ✅ **Done.** `lyx selfreport create <title>`
    files a new issue on the `Knatte18/loomyard` GitHub repository via the `gh` CLI. The target repo
    is hardcoded (no config required), making the command trivially callable from any sandbox agent
    context. Supports `--body` (or `-` to read from stdin) and `--label`; defaults to the `bug`
    label when no label is supplied. Durable design lives in the `internal/selfreportengine` package header.

## Explicitly out of scope

These stay in the Python/millpy domain and are **not** planned for `lyx`:

- millpy plumbing that a Go binary does not need: junctions/hardlinks/portals as a
  *config* concern, `PYTHONPATH`, venv setup, `MILL_PYTHON`. (Note: the worktree
  module *does* manage junctions as a teardown concern, but Loomyard never depends on them for
  its own layout.)
- The millpy wiki daemon and its socket/RPC infrastructure (Loomyard's board is
  one-shot and daemonless by design).
- Heuristic inference of home-file content shape and board-URL derivation. (Note: the deterministic weft→wiki URL rewrite (`.git`→`.wiki.git`) performed by `lyx warp clone` is **in** scope; only *heuristic* inference of board URLs stays out.)

## Maintenance

This roadmap is the shared reference for sequencing. When landing work:

- Move a milestone to ✅ **Done** with a link to its module doc when it ships.
- Add newly-discovered deferred work as a numbered milestone in the right place.
- Do **not** enumerate fine-grained sub-tasks here — those live in the board.
