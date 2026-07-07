# LoomYard

LoomYard (LY) is a task-orchestration system for [Claude Code](https://claude.ai/code). It manages the lifecycle of coding tasks — from triaging issues to merging finished code — using AI subagents for design, planning, implementation, and review, with each task isolated in its own git worktree.

At its center is **`lyx`** — a single Go binary (LoomYard eXecutable) that owns the task board, the git topology, and (in progress) the orchestrator. Everything else in LY is built around `lyx`. The repo is under active development: several modules ship today, and the orchestration layers are being built out.

> **A re-implementation of Millhouse in Go.** LoomYard is a ground-up rebuild of [Millhouse](https://github.com/Knatte18/millhouse) — same goal (task orchestration for Claude Code with isolated worktrees and AI subagents), rebuilt in Go instead of Python: one compiled binary, deep internal tests, and a cleaner geometry/overlay model.

## Inspiration

Through Millhouse, LoomYard builds on ideas from three projects:

- **[claude-code-plugins](https://github.com/motlin/claude-code-plugins)** by Craig Motlin — task tracking and skill plugins for Claude Code
- **[autoboard](https://github.com/willietran/autoboard)** by Willie Tran — autonomous agent orchestration patterns
- **[skills](https://github.com/mattpocock/skills)** by Matt Pocock — Claude Code skill conventions

## Naming: `lyx` · `loom` · `ly`

Three names for three layers, deliberately non-overlapping:

- **`lyx`** — the binary/CLI (**L**oom**Y**ard e**X**ecutable): one binary with a namespaced subcommand tree (`lyx board`, `lyx weft`, `lyx warp`, …).
- **`loom`** — the orchestrator *module* (`lyx loom run`), a domain like `board` or `weft` that drives a phased run.
- **`ly`** — the skill / orchestration plugin; skills are `/ly-*`.

Convenience alias: **`lyx run` → `lyx loom run`** (the everyday autonomous call).

## Design principles

1. **Toolkit-first.** Build small, composable primitives (board, warp, weft, mux) before the orchestrator that ties them together.
2. **One-shot, daemonless, file-coordinated.** A command does its work, writes JSON to stdout, and exits. Concurrent processes cooperate through files and locks, not a server.
3. **cwd-authoritative.** Config and state resolve from the current working directory, which need not equal the git-repo root.
4. **Correctness by tool design, not by recall.** A `lyx` command makes the correct path the path of least resistance and makes drift *detectable*, rather than relying on an operator or agent to remember a rule.

## Weft overlay model

LoomYard keeps the host repo pristine by routing all its own artifacts into a companion **weft repo** — a separate git repository that `lyx` controls.

```
<hub>/                              (top-level Hub, NOT a git repo)
  ├── <prime>/                      (host worktree, main branch)
  ├── <prime>-weft/                 (weft Prime worktree)
  ├── <slug>/                       (additional host worktree)
  ├── <slug>-weft/                  (weft worktree for <slug>)
  └── _board/                       (board repo; the task store)
```

Each host worktree uses a **junction** (Windows) or symlink to route writes (`_lyx/config/`, `_raddle/`) into its sibling weft worktree — transparently, so code that writes `_lyx/config/board.yaml` never sees the indirection. Two state roots with opposite lifecycles: **`_lyx/`** is durable and weft-synced (config, board, orchestration status — resume works across machines); **`.lyx/`** is ephemeral and machine-bound (live psmux runtime state, never synced).

All worktree and Hub geometry resolves through a single package, `internal/hubgeometry` — the sole owner of cwd and worktree-root math. See [CONSTRAINTS.md](CONSTRAINTS.md).

## Modules

Every user-facing module is a `lyx <module>` namespace, assembled into one cobra root. All commands print JSON: `{"ok":true, ...}` on success, `{"ok":false,"error":"..."}` on failure.

**Shipped:**

- **init** — scaffolds `_lyx/` and reconciles every module's config against its template (idempotent; never clobbers existing values).
- **board** — the task-tracker board.
- **config** — view/edit module configs; `lyx config reconcile` reconciles all configs against their templates; `lyx config <module> --set key=value` writes values non-interactively.
- **weft** — owns all git into the paired weft repo (`status|commit|push|pull|sync`).
- **warp** — the host↔weft git topology owner: clone, dual-worktree add/remove, coordinated checkout, reconcile, status, prune, cleanup.
- **ide** — one-shot IDE launcher for worktrees, with an interactive menu.
- **muxpoc** — a shipped proof-of-concept psmux orchestrator.
- **selfreport** — file bugs/enhancements against the repo via `gh`.

**In progress (design):**

- **mux** — the psmux overlay + strand bookkeeping + render.
- **loom** — the phased orchestrator (Setup → Discussion → Plan → Builder → Finalize), each phase gated by a review.
- **review** — a generic profile-driven gate engine, used by `loom` and standalone.

The internal libraries **proc** (cross-OS process spawn) and **shuttle** (drive one LLM agent via a swappable engine) sit under these; see [docs/modules/](docs/modules/).

## Orchestration stack

The orchestrator is a layered stack, each layer knowing only the one below. It has this shape because agents run as **interactive psmux sessions, never headless `claude -p`** — so spawning an agent is "place a pane, launch a provider, drive it, detect completion," not a plain `exec`.

```
internal/proc     spawn any OS process, cross-OS                    [OS primitive]
internal/mux      psmux overlay + strand bookkeeping + render       [builds on proc]
internal/shuttle  run ONE LLM agent via a swappable engine          [builds on mux]
review            generic gate engine: handler/fixer + judge        [builds on shuttle]
loom              phase machine: drive each phase through a gate     [builds on review]
```

The whole stack runs headless (auto mode): strands exist, agents run, output files are read, nobody need watch.

## Building

```bash
go build ./cmd/lyx        # build the lyx binary
go test ./...             # run the full suite (structural invariants included)
```

`deploy.cmd` builds and installs `lyx` onto PATH. Once deployed, run `lyx init` from a worktree to scaffold its `_lyx/` config.

## Sandbox Hub

The **sandbox Hub** is a dedicated bench for dogfooding `lyx` against itself, exercising the real deployed binary end to end. Build it with `sandbox-build.cmd`, run the agent suite with `sandbox-core-suite.cmd`, and collect its findings with `sandbox-fetch.cmd`. See [docs/sandbox-howto.md](docs/sandbox-howto.md) for the runbook.

## Requirements

- [Claude Code](https://claude.ai/code)
- Go 1.26+
- `gh` CLI authenticated (`gh auth login`)
- Git 2.35+ (for `git worktree`)
- psmux (for the orchestration layers)

## Documentation

- [CONSTRAINTS.md](CONSTRAINTS.md) — the repo's structural invariants (authoritative).
- [docs/overview.md](docs/overview.md) — architecture, naming, module and shared-lib map.
- [docs/roadmap.md](docs/roadmap.md) — numbered milestones and long-term direction.
- [docs/modules/](docs/modules/) — the module map and per-module design docs.
