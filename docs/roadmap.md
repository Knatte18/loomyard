# Roadmap: Loomyard

Loomyard will, in time, **replace mill/millhouse (Python) entirely** — the Go
infrastructure becomes the orchestration layer, and the mill skills get reworked
and trimmed in the same move. That is the long-term endgame, not the next step.

We get there by building **toolkits first**: small, self-contained modules with
deep internal tests, landed one at a time so the operator keeps full control at
every step. Orchestration (the part that ties worktrees, the board, and the mux
together into a spawn→merge→cleanup lifecycle) comes last — until then, mill's
existing Agent Dispatch handles it.

See [overview.md](overview.md#principles) for the design principles these
milestones follow.

## Milestones

Each milestone is independently shippable. Refactor milestones (2–4) are
**behaviour-preserving**: board's existing test suite is the guardrail, so nothing
observable changes until the new module that needs the extracted lib arrives.

1. **board** — the task tracker. ✅ **Done.** See the board module in
   [overview.md#modules](overview.md#modules).

2. **Extract shared infrastructure: `internal/config`, `internal/git`,
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
   in `internal/paths`. Consumes `internal/config` + `internal/git`; owns the **junction-aware teardown**
   sequence (the Windows locked-worktree hazard). The module is **stateless by design** — `lyx worktree list` is a thin
   `git worktree list` wrapper; there is no worktree registry. Introduces `internal/paths` as the sole geometry owner, banning
   raw `os.Getwd` and `git rev-parse --show-toplevel` outside `internal/paths` and `cmd/lyx/main.go`
   via `internal/paths/enforcement_test.go`. (Portals are present and working — a subdir-mirrored
   Hub view of each worktree's `_lyx/`; kept available, not slated for removal.)

5. **Task 006 — Weft engine.** Path geometry for weft worktrees, paired host+weft spawn and teardown, `lyx weft` command.
   Implements the canonical weft overlay model (host stays pristine, all lyx artifacts in companion weft repo).
   Weft directories are reached by direct sibling access; portals remain available as the cross-worktree status view.

6. **Task 007 — Hub-creator / `lyx-clone` skill.** Bootstrap and clone new host repos as neighbors in an existing hub.

7. **Task 008 — `_codeguide` junction and configuration TUI.** ✅ `lyx config` menu interface shipped.
   Remaining: activate `_codeguide` junctions and define `_lyx/config/` YAML schema for codeguide (deferred to a later task).

8. **mux v1 — session layout.** One psmux instance per worktree with stacked agent panes
   ([modules/mux.md](modules/mux.md)). Event-driven pane switching via Claude Code hooks. No daemon, no Slack.
   **Note:** A working proof-of-concept of the daemon and pane-recovery model
   already ships as `internal/muxpoc`.

9. **mux v2 — subprocess panes.** Parent/child pane tree (a spawned reviewer
   appears below its parent). Built only once Agent Dispatch stops being enough.
   **Proven in muxpoc:** see [overview.md#modules](overview.md#modules).

10. **mux daemon.** Standalone watchdog process: detects a psmux crash via
    `cmd.Wait()`, recovers each pane by relaunching interactive Claude and re-injecting
    context from mux's own capture journal (native `--resume` does **not** work for
    programmatically-driven panes — see [modules/mux.md](modules/mux.md#resume-after-crash-the-corrected-model)),
    mutual watchdog so both must die to go dark. See [modules/mux.md](modules/mux.md#deferred).
    **Proven in muxpoc:** see [overview.md#modules](overview.md#modules). **Event-driven pane
    switching / idle detection via Claude Code's own hooks + `claude agents --json` is
    explored in [research/mux-hooks-exploration.md](research/mux-hooks-exploration.md)** — a
    lower-cost alternative to the capture-pane poller for the focus decision.

11. **Slack relay.** Bidirectional, one channel per worktree, riding on the daemon.

12. **`init` grows: create / clone the board repo.** Today `init` only scaffolds
    `_lyx/`; the board dir is auto-created on first write and made a git repo by
    hand. This milestone makes `init` create a board repo from scratch or clone one
    from a remote (analogous to mill-setup phases 1–3).

13. **doctor.** A diagnostics command (`Loomyard doctor`): checks `_lyx/` is present,
    `*.yaml` parse and use known keys, the board repo is reachable, no stale lock
    files, the state file is readable — and prints remediation. Pure
    troubleshooting, no domain logic.

14. **session sync.** `Loomyard session push/pull` — copy Claude `.jsonl` transcripts
    across machines so `claude --resume` works elsewhere (sessions are not portable
    today). See [modules/mux.md](modules/mux.md#session-files-and-portability).

15. **Claude Code plugin packaging.** Ship `lyx` as an installable Claude Code
    plugin, exactly as mill/millpy were, once the binary and module architecture are
    proven.

16. **orchestrator — the endgame.** Port mill's spawn → merge → cleanup lifecycle
    into Go, tying board + worktree + mux together. This is what lets `lyx` finally
    replace mill/millhouse. The toolkit modules (1–7) are deliberately designed so
    this is *possible* — clean state files, no hidden interactive assumptions — but
    it is the last thing built.

## Explicitly out of scope

These stay in the Python/millpy domain and are **not** planned for `lyx`:

- millpy plumbing that a Go binary does not need: junctions/hardlinks/portals as a
  *config* concern, `PYTHONPATH`, venv setup, `MILL_PYTHON`. (Note: the worktree
  module *does* manage junctions as a teardown concern, but Loomyard never depends on them for
  its own layout.)
- The millpy wiki daemon and its socket/RPC infrastructure (Loomyard's board is
  one-shot and daemonless by design).
- VS Code workspace colour schemes and project-local customisations.
- Heuristic inference of home-file content shape and board-URL derivation.

## Maintenance

This roadmap is the shared reference for sequencing. When landing work:

- Move a milestone to ✅ **Done** with a link to its module doc when it ships.
- Add newly-discovered deferred work as a numbered milestone in the right place.
- Do **not** enumerate fine-grained sub-tasks here — those live in the board.
