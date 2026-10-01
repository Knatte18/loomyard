# LoomYard

LoomYard is an autonomous software-development pipeline built on [Claude Code](https://claude.ai/code).
You hand it a task, and it can discuss the task with you before it writes a plan.
It then implements the plan batch by batch, has independent agents review every artifact, opens a pull request, and lands it once you approve.
Each task runs in its own isolated git worktree pair, and many tasks can run at once.

At its center is **`lyx`**, a single Go binary (LoomYard eXecutable) that owns the task board, the git topology, every phase transition, and every agent launch.
LoomYard is developed with LoomYard: its own tasks run through the same pipeline, with their state in [`loomyard-weft`](https://github.com/Knatte18/loomyard-weft).

## The central idea: deterministic Go around narrow LLM calls

An agentic system built out of prompts asks a model to do work a program does better.
Deciding what runs next, parsing a plan, staging a commit, checking an artifact against its format, retrying a failed step, resuming after a crash: each of these is a program, and each is slow, expensive, and non-reproducible when a model does it instead.

LoomYard draws a hard line.
Control flow, state, git, parsing, validation, routing, retry, and resume are Go — tested, deterministic, and cheap.
Models do two kinds of work.
Inside a run, they do the steps that need judgment: is this plan sound, does this diff match its plan, write this code.
Above a run, an LLM driver steps it forward and supervises it: it reads what each step reported, repairs what went wrong, and escalates what it cannot fix.
The driver never decides what runs next, though;
the phase engine does.
Every call inside a run goes through a narrow file contract — a prompt goes in, named files come out — and a mechanical gate checks the output before the run moves on.
An agent whose output fails its gate is re-prompted in the same session with the findings, rather than trusted.

The payoff: the same run does the same thing twice, a crashed run resumes exactly where it stopped, and the parts most likely to break are covered by `go test` rather than by hope.

## What a task run looks like

A task's run is a flat list of producers, walked by a generic phase engine (`shed`).
The list is data, not code — [`contracts/recipes/loom-recipe.yaml`](contracts/recipes/loom-recipe.yaml), embedded in the binary:

```
Preflight → Loom-Preflight
  → Discussion-Write → [Discussion review]
  → Plan-Write       → [Plan review]
  → Batchifier → Webster → [Webster review]
  → Describe → Publish → PR-Gate ⇄ PR-Rework
  → Finalize → Friction-Reflect
```

- **Discussion** — an agent turns the task into a decision record: scope, decisions taken, what is out of scope.
  It can stop and ask you when a decision is yours to make.
- **Plan** — an agent writes a plan as a list of cards, each naming the code it touches by *glyph* — a stable symbol spelling resolved against the real tree through [quarry](https://github.com/Knatte18/quarry), so a plan that names a function that does not exist fails validation before any code is written.
- **Webster** — the implementer.
  One long-lived Master session reads the plan once and forks one implementer per batch inside its own session, bracketed by Go verbs (`begin-batch`, `await-batch`, `record-batch`) that verify and record each batch.
  A stuck fork escalates to a cold recovery session.
- **Review segments** — each `[… review]` is a judge (`Bouncer`) paired with a review-and-fix round (`burler`).
  The judge approves or sends the artifact back for another round, within a bounce budget;
  an exhausted budget halts the run for a human rather than looping.
  The judge and the reviewer are fresh agents, independent of the one that wrote the artifact, so no agent ever reviews its own work.
- **Landing** — `Describe` writes the change description, `Publish` opens the pull request, and `PR-Gate` waits for the operator.
  `lyx loom approve` lands it;
  `lyx loom reject <review-file>` sends the operator's findings to `PR-Rework`, which plans and implements a rework round and comes back to the gate.
- **Friction-Reflect** — the agents' own notes on what was hard or broken in the tooling are reflected on and can be filed as issues against LoomYard itself (`selfreport`).

Routing is explicit per row (`on_done`, `on_stuck`), never positional, and the engine's validator refuses a recipe whose edges cross a review segment's boundary.
Resume, pause, and crash recovery work uniformly at producer granularity.

## Three layers of driving

The phase engine only moves one producer at a time;
what calls it is a choice of driver.

- **`lyx shed step`** — drive one producer forward and report a JSON envelope naming its outcome and the trace it wrote.
  This is the primitive every driver is built on.
- **`ly-drive`** — a Claude Code skill that drives a run by repeated `lyx shed step`, reads each envelope and trace, repairs what it can, re-steps after a transient failure, and escalates what it cannot.
  It carries no phase knowledge: which recipe runs is a property of the run's seed alone.
  `lyx loom start` (alias `lyx start`) bootstraps a task worktree and launches its driver session.
- **`lyx batten`** — drives a task worktree's whole lifecycle from the hub's main worktree as one run of its own: create the worktree pair, seed the task's run, watch it to a terminal state, tear the pair down.
- **`lyx orch`** — hosts the hub orchestrator: one long-lived Claude session that dispatches and supervises runs.
  A detached watcher reads its context usage after each turn;
  past a threshold, while idle, it has the session write a handoff, clears it, and resumes it from that handoff, so the orchestrator outlives any single context window.

The split is the same principle again: the engine decides *what* runs next, and the LLM layers above it only decide how to recover when a step fails.

## Agents run as interactive tmux sessions, never `claude -p`

Every agent LoomYard launches is an interactive Claude Code session in a tmux pane, never headless `claude -p`.
Interactive sessions keep subscription coverage;
headless usage is moving to API billing.

So launching an agent is not an `exec`: it is "place a pane, launch the provider, dismiss its startup dialogs, drive it, detect completion."
That is a layered stack, each layer knowing only the one below:

```
proc     spawn any OS process, cross-OS
reed     tmux overlay: strand bookkeeping, layout, a watchdog that reaps dead panes
shuttle  run ONE agent over the file contract via a swappable provider engine
burler   one review+fix round              webster   the batch implementer
shed     walk a flat producer list to a terminal outcome
loom     shed + the task recipe            batten    shed + the worktree-lifecycle recipe
```

`shuttle` classifies every run as `done`, `asking`, `died`, or `timeout`, and owns everything provider-specific — how Claude is launched and which startup gates it shows — behind an engine interface, so a second provider plugs in as another engine without any layer above changing.

## Fabric: state that travels without touching your repo

An orchestrator has to keep state somewhere: config, the task board, plans, review verdicts, run status.
In your repo it pollutes your history;
outside it, the state does not branch with the work and cannot resume on another machine.

LoomYard keeps it in a second git repository woven into yours.
Your repository is the **warp**;
the **weft** carries everything LoomYard generates.
Every warp worktree gets a weft sibling on a matching branch, wired together on disk, so state written while working in a worktree lands in the weft without a single LoomYard file in your repo's history or `.gitignore`.

Together the two are the **Fabric**, and `lyx fabric` is the seam that moves both sides as one: `add`/`remove` a worktree pair, `checkout` both branches together, `pull`, `status`, `diff`, `merge`, and `reconcile` a pair that drifted back onto the recorded layout.
Every git operation LoomYard's own code performs goes through the `fabric` engine — never raw git, and never an agent.
An agent commits its own code to the warp and nothing else;
the weft is committed by Go, at boundaries the orchestrator controls.

Because the weft branches in lockstep with the warp, a task's whole state is versioned and pushed:
pick the task up on another machine and it resumes where it stopped, and two tasks never see each other's plans or status.

```
<hub>/                    (not a git repo)
  ├── <prime>/            your repo, main branch        ┐ one Fabric
  ├── <prime>-weft/       its weft side                 ┘
  ├── <slug>/             a task worktree               ┐ likewise, on
  ├── <slug>-weft/        its weft side                 ┘ the task branch
  ├── _board/             the task store
  ├── _portals/           entry points into each worktree's state
  └── _launchers/         per-worktree launcher scripts and workspaces
```

## Engineering discipline

- **Structural invariants as tests.**
  [`CONSTRAINTS.md`](CONSTRAINTS.md) records the repo's cross-cutting invariants, and most of them are enforced by `go test` scans rather than by review: one package owns path resolution, one parser exists per on-disk format, nothing outside `shuttle` touches provider readiness, and so on.
- **Told, never derived.**
  Every layer from `reed` up is handed its geometry — absolute, already-resolved paths — instead of computing it, which is what lets the same producer run inside a hub or against a plain checkout (`--target-dir`) with no hub at all.
- **Prompts are versioned contracts.**
  Every prompt an agent reads is a stencil under [`contracts/stencils/`](contracts/stencils/), embedded as a default and read from the hub at call time, so an operator can edit a live prompt without a rebuild and `lyx stencil` shows the drift.
- **Correctness by tool design, not by recall.**
  A `lyx` command makes the correct path the path of least resistance and makes drift detectable, instead of relying on an agent to remember a rule.- **Crucible.**
  Modules that drive live tmux and real agents are hardened by [crucible](crucible/README.md): serial, model-rotating review-and-fix rounds against the live substrate, each finding proved by sabotage — revert the fix, watch the regression test fail, restore.

## Modules

Every user-facing module is a `lyx <module>` namespace;
`lyx --help` lists them, and every command takes `--help` (or `--json` for structured help).
Commands print a JSON envelope: `{"ok":true, ...}` or `{"ok":false,"error":"..."}`.

| Module | What it does |
|---|---|
| `board` | the task board, plus a not-yet-claimable notes surface |
| `fabric` | warp↔weft topology, sync, and the merge lifecycle; `fabric clone` creates a hub in one call |
| `config` | view, edit, and reconcile module configs against their templates |
| `reed` | the tmux strand overlay and its watchdog |
| `shuttle` | run one agent over the file contract |
| `burler` | one review+fix round over an artifact |
| `webster` | the batch implementer |
| `shed` | the generic `seed`/`run`/`step`/`status`/`pause` verbs over any recipe |
| `loom` | the task pipeline, plus `approve`/`reject` at the PR gate and the standalone validators its gates use |
| `batten` | a task worktree's whole lifecycle as one run |
| `orch` | the self-cycling hub orchestrator session |
| `quarry` | glyph lookups against the worktree's own code, the planner's source of symbol spellings |
| `stencil` | inspect, diff, and promote the prompts agents read |
| `selfreport` | file a bug or enhancement against LoomYard's own repo |
| `ide` | open a worktree in VS Code |

[`docs/overview.md`](docs/overview.md) maps every package, including the internal layers under these.

## Getting started

```bash
go build ./cmd/lyx                                  # build the binary
lyx fabric clone <weft-url> <warp-url>              # create a hub: both repos, wiring, config, board
lyx fabric add <slug>                               # a task worktree pair
cd <hub>/<slug> && lyx start                        # bootstrap the task and hand the terminal to its driver
```

`./update-plugins.sh` (`update-plugins.cmd` on Windows) is the only route to production: from a clean tree pushed to `origin/main`, it installs the plugins, builds `lyx` into the Go bin dir, and moves the `prod` branch to that commit.
`./deploy-dev` builds the working tree into `.dev-bin` for testing without touching production.

The [sandbox Hub](docs/sandbox-howto.md) is a bench for running the real binary end to end against a throwaway hub, with per-module suites an agent drives and reports findings from.

### Requirements

- [Claude Code](https://claude.ai/code), logged in
- Go (the version in `go.mod`) and a C toolchain — `lyx` links quarry's tree-sitter grammars through cgo
- Git 2.42+
- tmux (on Windows, psmux)
- A GitHub token for `Publish` and `selfreport`: `GH_TOKEN`, `GITHUB_TOKEN`, or an authenticated `gh` CLI

## Plugins

[`plugins/`](plugins/) holds this repo's Claude Code marketplace:

- **ly** — `ly-drive`, the recipe-blind driver described above.
- **prowler** — fetch blocked or JS-rendered web pages as readable markdown, plus cross-repo code search.

The writing and code conventions every agent loads come from the shared [scribe](https://github.com/Knatte18/scribe) plugin.

## Lineage: Millhouse

LoomYard grew out of [Millhouse](https://github.com/Knatte18/millhouse), a set of Claude Code skills and Python tools for running parallel Claude Code sessions with minimal input:
isolated worktrees per task, a wiki-backed task board, and subagents for discussion, planning, implementation, and review.
Millhouse proved the workflow and is still how much of LoomYard was built.
LoomYard is the rebuild that moved the workflow out of prompts and into a program: the phase machine is data, review is a Go-owned gate loop, and the git topology is a model Millhouse has no equivalent of.

Through Millhouse, LoomYard builds on ideas from [claude-code-plugins](https://github.com/motlin/claude-code-plugins) (Craig Motlin), [autoboard](https://github.com/willietran/autoboard) (Willie Tran), and [skills](https://github.com/mattpocock/skills) (Matt Pocock).

## Documentation

- [CONSTRAINTS.md](CONSTRAINTS.md) — the structural invariants (authoritative).
- [docs/overview.md](docs/overview.md) — architecture, naming, and the module and package map.
- [contracts/specs/](contracts/specs/) — the on-disk format contracts, each with exactly one parser.
- [manifest/](manifest/roadmap.md) — what is planned and not yet built.
- [crucible/](crucible/README.md) — the hardening method for live-substrate modules.

Per-package documentation lives in each package's `doc.go` and is the durable detail for anything shipped.
