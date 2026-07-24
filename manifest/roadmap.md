# Roadmap: Loomyard

Loomyard replaces mill/millhouse (Python) with a Go orchestration layer, built as
self-contained modules landed one at a time. See [docs/overview.md](../docs/overview.md#principles)
for the design principles. This file is a flat list of what's planned and what's shipped —
for the detailed design of anything not yet built, see its doc under [modules/](modules/).

## Planned

- **loom** — phase machine orchestrator (`lyx loom run`). Preflight, Discussion producer built;
  Plan producer, phase-machine skeleton, Finalize, and session bootstrap remain. Raddle (a phase
  slot reserved between Builder and Finalize) stays deferred. See [modules/loom.md](modules/loom.md).
- **webster v2 plan-card schema** — decide the plan's card schema (declared file
  reads/writes, DAG derived in Go, not by the LLM) before loom's Plan producer builds against it.
  The further, riskier parallel-batch executor stays undecided, gated on real evidence. See
  [modules/websterv2.md](modules/websterv2.md).
- **doctor** — diagnostics command (`lyx doctor`): checks `_lyx/` layout, config parse, board
  reachability, stale locks.
- **init: board-repo creation from scratch** — when starting with no existing board remote.
- **session sync** — copy Claude `.jsonl` transcripts across machines so `--resume` works
  elsewhere.
- **Claude Code plugin packaging** — ship `lyx` as an installable plugin.
- **mux: cross-worktree columns** — all worktrees in one window, a column per worktree.
- **mux: daemon → Slack relay** — standalone watchdog + bidirectional Slack relay per worktree.
- **mux: own-window strand anchoring** — a `display` anchor that spawns a strand into its own
  switchable tmux window instead of a pane.
- **Real-Linux validation** — run the sandbox suite and validate every tmux/`/proc` assumption on
  a real Linux box (built and cross-compiled so far, never executed there).

## Done

- **board** — task tracker.
- **shared infra** — `internal/configengine`, `internal/gitexec`, `internal/lock`, `internal/state`.
- **worktree + ide** — worktree/portal management, VS Code launcher (worktree itself superseded by `warp`).
- **weft** — companion weft repo, paired host+weft spawn/teardown.
- **config TUI** — `lyx config` interactive menu + `reconcile`.
- **warp** — host↔weft-coordinated git topology (clone, add/remove, checkout, reconcile, cleanup).
- **proc** — cross-OS process spawn.
- **mux** — tmux overlay + strand bookkeeping + render.
- **shuttle** — run one LLM agent as an interactive tmux strand over a swappable engine.
- **burler** — one review+fix round (A-review → B-fix).
- **perch** — the gate loop: run `burler` rounds until `APPROVED`/`STUCK`.
- **builder** — batch-implementation loop over a pinned plan (sequential, one strand per batch).
- **webster** — fork-based sibling of builder (in-session forks, one Master per plan).
- **built-in CLI help** — self-documenting `lyx`/`lyx <module>`/`lyx <module> <cmd> --help`.
- **selfreport** — file Loomyard bugs as GitHub issues (`lyx selfreport create`).
- **loom: contracts, Preflight, Discussion producer** — the three loom pieces shipped so far
  (loom as a whole is not done — see Planned).

## Maintenance

- Move an item from Planned to Done, with a link to its module doc if one exists, when it ships.
- Delete a module's doc under `modules/` once it ships (see the
  [documentation lifecycle](../docs/overview.md#documentation-lifecycle)) — that's why Done
  entries above don't link anywhere.
- Unscheduled, speculative ideas that aren't ready to commit to go in
  [long-term-ideas.md](long-term-ideas.md), not here.
