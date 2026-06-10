# Roadmap: mhgo

`mhgo` will, in time, **replace mill/millhouse (Python) entirely** — the Go
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

1. **board** — the task tracker. ✅ **Done.** See
   [modules/board.md](modules/board.md).

2. **Extract shared infrastructure: `internal/config`, `internal/git`,
   `internal/lock`.** ✅ **Done.** See
   [shared-libs/](shared-libs/README.md).

3. **`internal/state`.** New package: typed read/write of the gitignored,
   machine-local `.mhgo/local-state.json` registry
   ([`internal/state`](shared-libs/state.md)). Built test-first — nothing
   in board needs it, so it has no existing suite to lean on.

4. **worktree module.** Create / track / tear down git worktrees
   ([modules/worktree.md](modules/worktree.md)). First consumer of all three shared
   libs + the new state lib. Owns the **junction-aware teardown** sequence (the
   Windows locked-worktree hazard).

5. **mux v1 — column per worktree.** One psmux window per repo, one column per
   worktree, laid out from the worktree registry
   ([modules/mux.md](modules/mux.md)). No subprocess panes, no daemon, no Slack.

6. **mux v2 — subprocess panes.** Parent/child pane tree (a spawned reviewer
   appears below its parent). Built only once Agent Dispatch stops being enough.

7. **mux daemon.** Standalone watchdog process: detects a psmux crash via
   `cmd.Wait()`, respawns Claude with `--resume <session-id>`, mutual watchdog so
   both must die to go dark. See [modules/mux.md](modules/mux.md#deferred).

8. **Slack relay.** Bidirectional, one channel per worktree, riding on the daemon.

9. **`init` grows: create / clone the board repo.** Today `init` only scaffolds
    `_mhgo/`; the board dir is auto-created on first write and made a git repo by
    hand. This milestone makes `init` create a board repo from scratch or clone one
    from a remote (analogous to mill-setup phases 1–3).

10. **doctor.** A diagnostics command (`mhgo doctor`): checks `_mhgo/` is present,
    `*.yaml` parse and use known keys, the board repo is reachable, no stale lock
    files, the state file is readable — and prints remediation. Pure
    troubleshooting, no domain logic.

11. **session sync.** `mhgo session push/pull` — copy Claude `.jsonl` transcripts
    across machines so `claude --resume` works elsewhere (sessions are not portable
    today). See [modules/mux.md](modules/mux.md#session-files-and-portability).

12. **Claude Code plugin packaging.** Ship `mhgo` as an installable Claude Code
    plugin, exactly as mill/millpy were, once the binary and module architecture are
    proven.

13. **orchestrator — the endgame.** Port mill's spawn → merge → cleanup lifecycle
    into Go, tying board + worktree + mux together. This is what lets `mhgo` finally
    replace mill/millhouse. The toolkit modules (1–8) are deliberately designed so
    this is *possible* — clean state files, no hidden interactive assumptions — but
    it is the last thing built.

## Explicitly out of scope

These stay in the Python/millpy domain and are **not** planned for `mhgo`:

- millpy plumbing that a Go binary does not need: junctions/hardlinks/portals as a
  *config* concern, `PYTHONPATH`, venv setup, `MILL_PYTHON`. (Note: the worktree
  module *does* manage junctions as a teardown concern — see
  [modules/worktree.md](modules/worktree.md) — but mhgo never depends on them for
  its own layout.)
- The millpy wiki daemon and its socket/RPC infrastructure (mhgo's board is
  one-shot and daemonless by design).
- VS Code workspace colour schemes and project-local customisations.
- Heuristic inference of home-file content shape and board-URL derivation.

## Maintenance

This roadmap is the shared reference for sequencing. When landing work:

- Move a milestone to ✅ **Done** with a link to its module doc when it ships.
- Add newly-discovered deferred work as a numbered milestone in the right place.
- Do **not** enumerate fine-grained sub-tasks here — those live in the board.
