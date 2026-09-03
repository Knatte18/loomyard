# LoomYard

LoomYard (LY) is a task-orchestration system for [Claude Code](https://claude.ai/code).
It manages the lifecycle of coding tasks — from a discussion of what to build, through planning, implementation, and review, to the merge back — with each task isolated in its own git worktree.

**The central idea: replace as much of the agent loop as possible with deterministic Go.**

An agentic system built out of prompts asks a model to do work a program does better.
Deciding what runs next, parsing a plan, walking a directory, staging a commit, checking whether an artifact satisfies its format, retrying a failed step, resuming after a crash — every one of those is a program, and every one of them is slow, expensive, and *non-reproducible* when a language model does it instead.
Worse, it fails differently each time.

So LoomYard draws a hard line.
Control flow, state, git, parsing, validation, geometry, routing, retry, and resume are Go — tested, deterministic, and cheap.
A model is called only where judgment is genuinely irreducible: is this plan sound, does this diff match its plan, write this code.
Those calls are made through a narrow file contract — a prompt goes in, named files come out — so even the LLM steps have a machine-checkable shape around them.

The practical payoff is that the same run does the same thing twice, a crashed run resumes exactly where it stopped, and the parts most likely to break are the parts covered by `go test` rather than by hope.

At its center is **`lyx`** — a single Go binary (LoomYard eXecutable) that owns the task board, the git topology, and the orchestrator.
The full spine now ships: `lyx run` in a worktree bootstraps a task and drives it through a seventeen-row phase machine to a merge-back, unattended.

> **Built on Millhouse's ideas, not a port of it.** LoomYard started as a Go rebuild of [Millhouse](https://github.com/Knatte18/millhouse) and still owes it the core premise — task orchestration for Claude Code, isolated worktrees, AI subagents for the judgment steps. It has since grown well past that: the orchestrator is a data-driven phase machine rather than a skill set, review is a Go-owned gate loop, and the git topology is a model Millhouse has no equivalent of.

## Inspiration

Through Millhouse, LoomYard builds on ideas from three projects:

- **[claude-code-plugins](https://github.com/motlin/claude-code-plugins)** by Craig Motlin — task tracking and skill plugins for Claude Code
- **[autoboard](https://github.com/willietran/autoboard)** by Willie Tran — autonomous agent orchestration patterns
- **[skills](https://github.com/mattpocock/skills)** by Matt Pocock — Claude Code skill conventions

## Naming: `lyx` · `loom` · `ly`

Three names for three layers, deliberately non-overlapping:

- **`lyx`** — the binary/CLI (**L**oom**Y**ard e**X**ecutable): one binary with a namespaced subcommand tree (`lyx board`, `lyx fabric`, `lyx webster`, …).
- **`loom`** — the orchestrator *module* (`lyx loom run`), a domain like `board` or `fabric` that drives a phased run.
- **`ly`** — the skill / orchestration plugin;
  skills are `/ly-*`.
  Still a plan rather than a shipped set — see [docs/skills.md](docs/skills.md) for which mill skills become `lyx` verbs and which survive as skills.

Convenience alias: **`lyx run` → `lyx loom run`** (the everyday autonomous call).

## Design principles

1. **Go where it can be;
   LLM only for judgment.**
   The principle above, stated as a build rule: deterministic work — verbs, control flow, parsing, distillation, geometry, git — is Go;
   a model handles only what a program cannot (review verdicts, batch implementation, an orchestrator's recovery decisions).
   When a step could plausibly go either way, it goes to Go.
2. **Toolkit-first.**
   Build small, composable primitives (board, fabric, reed) before the orchestrator that ties them together.
3. **One-shot, daemonless, file-coordinated.**
   A command does its work, writes JSON to stdout, and exits.
   Concurrent processes cooperate through files and locks, not a server.
4. **cwd-authoritative.**
   Config and state resolve from the current working directory, which need not equal the git-repo root.
5. **Told, never derived.**
   Every layer from `reed` up is *handed* its geometry — absolute paths, already resolved — instead of computing it.
   That is what lets the same producer run inside a hub or against a plain checkout with no hub at all;
   see the Told-Geometry Invariant in [CONSTRAINTS.md](CONSTRAINTS.md).
6. **Correctness by tool design, not by recall.**
   A `lyx` command makes the correct path the path of least resistance and makes drift *detectable*, rather than relying on an operator or agent to remember a rule.

## Fabric: the warp and the weft

An orchestrator has to keep state somewhere — config, task board, plans, review verdicts, run status.
Putting that in your repo pollutes it;
putting it outside your repo means it doesn't travel, doesn't branch with the work, and can't be resumed on another machine.

LoomYard's answer is a **piggyback repo woven alongside your own**.
Your repository is the **warp**;
a second git repository, the **weft**, carries everything LoomYard generates.
Every warp worktree gets a weft sibling on a matching branch, and the two are wired together on disk so that state written while working in a worktree lands in the weft — invisibly, without a single LoomYard file ever appearing in your repo's history or its `.gitignore`.

```
<hub>/                                (top-level Hub, NOT a git repo)
  ├── <prime>/                        (your repo, main branch)
  ├── <prime>-weft/                   (its piggyback sibling)
  ├── <slug>/                         (a task worktree)
  ├── <slug>-weft/                    (that task's piggyback sibling)
  ├── _board/                         (the task store, on weft's main branch)
  ├── _portals/                       (per-worktree entry points into the weft side)
  └── _launchers/                     (per-worktree launcher scripts)
```

Because the weft is a real git repository that branches in lockstep with the warp, a task's whole state is versioned, pushed, and recoverable:
pick the task up on another machine and it resumes where it stopped.
And because state is per-branch rather than global, two agents working two tasks never see each other's plans, verdicts, or run status.

`lyx fabric` is the module that owns this — it is the only thing in LoomYard permitted to touch either repository's git, and it moves both sides as a pair: clone, worktree add/remove, branch switch, merge, sync.
`lyx fabric reconcile` converges a drifted or hand-broken pair back onto the recorded layout.

All path resolution goes through a single package, `internal/lyxcwd`, so this geometry has exactly one owner;
see [CONSTRAINTS.md](CONSTRAINTS.md) and [docs/overview.md](docs/overview.md) for the on-disk detail.

## Modules

Every user-facing module is a `lyx <module>` namespace, assembled into one cobra root.
All commands print JSON: `{"ok":true, ...}` on success, `{"ok":false,"error":"..."}` on failure.

- **board** — the task-tracker board, plus a parallel not-yet-claimable `notes` surface and `promote-note` between them.
- **config** — view/edit module configs;
  `lyx config reconcile` reconciles all configs against their templates;
  `lyx config <module> --set key=value` writes values non-interactively.
- **fabric** — the sole warp↔weft git-coordination module, unifying topology (clone, dual-worktree add/remove, coordinated checkout, reconcile, status, prune, cleanup), weft content-sync (`status|commit|push|pull|sync|diff`), and a merge/conflict lifecycle (`merge-in|merge|merge-stage|merge --continue|--abort`) in one command tree.
  `lyx fabric clone` is the hub creator and does the whole job in one call — there is no separate activation step (the former `lyx init` dissolved into it).
- **ide** — one-shot IDE launcher for worktrees, with an interactive menu.
- **reed** — the tmux overlay + strand bookkeeping + render, with a watchdog daemon that reconciles resize geometry and reaps dead panes.
- **shuttle** — runs one LLM agent as an interactive tmux strand over a file contract, via a swappable provider engine (Claude today), classifying every run as `done`/`asking`/`died`/`timeout`.
- **burler** — one review+fix round over an artifact: A-review → B-fix, one agent, no self-grading, driven entirely by a profile YAML so the round itself carries zero domain knowledge.
- **webster** — the implementer: one long-lived Master session reads the flat card-list plan once and forks one implementer per batch **in-session**, bracketed by `begin-batch`/`await-batch`/`record-batch`, escalating a stuck fork to a cold recovery strand.
- **stencil** — the operator surface over the producer prompts every agent reads from disk at call time: `list|validate|diff|sync|promote`.
- **loom** — the phased orchestrator (`run|drive|status|pause|validate-discussion|validate-plan`).
  See [the phase machine](#the-phase-machine) below.
- **selfreport** — file bugs/enhancements against the repo via go-github, authenticated through `internal/githubclient` (`gh` is a fallback token source, not the transport).

Under these sit the internal (non-CLI) layers: **proc** (cross-OS process spawn), **shed** (the generic phase engine), the landing producers (`Publish`/`Finalize`), the precondition/geometry layer (`preflight`, `hubgeom`, `standalonegeom`), and the sole-parser leaves each on-disk format gets exactly one of — `planparser`, `discussionparser`, `summaryparser`.
`treadleengine` is shipped but consumer-less on purpose: it is the generalized round-loop engine kept for the future `Tenter`.
See [docs/overview.md](docs/overview.md) for the full map and [manifest/designs/](manifest/designs/) for what is designed but not yet built.

## Orchestration stack

The orchestrator is a layered stack, each layer knowing only the one below.
It has this shape because agents run as **interactive tmux sessions, never headless `claude -p`** — so spawning an agent is "place a pane, launch a provider, drive it, detect completion," not a plain `exec`.

```
internal/proc     spawn any OS process, cross-OS                     [OS primitive]
internal/reed     tmux overlay + strand bookkeeping + render         [builds on proc]
internal/shuttle  run ONE LLM agent via a swappable engine           [builds on reed]
burler            one review+fix round: review → fix                 [builds on shuttle]
shed              walk a flat producer list to a terminal outcome    [engine; adapters wrap the above]
loom              shed + loom's own producer list                    [builds on shed]
```

`webster` branches off `shuttle` directly (an LLM orchestrator driving fat Go verbs, not a review-gate loop).
The whole stack runs headless (auto mode): strands exist, agents run, output files are read, nobody need watch.

The stack has two entry modes.
In **hub mode** a command resolves its geometry from the surrounding hub;
in **standalone mode** it is told a target directory instead, so `lyx burler run --target-dir …` and `lyx webster run --target-dir …` work against a plain git checkout with no hub, no fabric, and no orchestrator seed.

## The phase machine

`shed` (`internal/shedengine`) is a generic engine with **no predefined slots** — no Preflight slot, no Finalize slot, no review slot.
It walks one flat, ordered list of producers, honoring resume, crash-recovery, and pause uniformly at producer granularity.
What makes a product a product is purely which producers are in its list.

Routing is per-producer and explicit, never positional: a `Done` verdict follows that row's own `on_done`, a `Stuck` verdict follows its `on_stuck` (bouncing back to any row, forward or backward, within a per-producer bounce budget) or escalates to a human when `on_stuck` is empty.
List order is display order only.

`loom` is therefore `shed` plus one list, and that list is data rather than code: [`contracts/recipes/loom-recipe.yaml`](contracts/recipes/loom-recipe.yaml), embedded into the binary and assembled into producers by `internal/loomrecipe` against `internal/shedrecipe`'s engine registry.
Its seventeen rows:

```
Preflight → Loom-Preflight
  → Discussion-Write → Discussion-Validate → [Discussion-Review segment]
  → Plan-Write → Plan-Validate → [Plan-Review segment] → Plan-Revalidate
  → Batchifier → Webster → [Webster-Review segment]
  → Publish → Finalize
```

Each `[…-Review segment]` is two rows: a **`Bouncer`** (the judge — reads the artifact against a rubric, writes a verdict and a cross-round ledger) and a **`BurlerRound`** (one `burler` review+fix round).
The two are bound by a shared `segment:` label, and `shedengine`'s validator refuses an `on_stuck` that crosses a segment boundary — so the pair's mutual bounce edges are structurally enforced rather than conventional.
The `Bouncer` returns `Done` only on an `APPROVED` verdict;
the round producer never returns `Done` at all, handing back to its judge every time.
Re-entering a settled segment re-judges from a fresh round 1 rather than replaying the old approval.

From `loom`'s side every segment is the same black box with two exits.
Only three things differ per phase: the rubric, the round's **fix-scope** (`overlay` for discussion and plan, whose targets are weft content the loop owner must commit;
`source` for Webster, where the agent commits each fix to the warp repo itself), and the segment's **commit seam**.
That split is the Fabric Git Invariant: every weft commit belongs to the loop owner in Go, and the agent's own commit-per-fix to the warp repo is the single named exception.

See [manifest/designs/loom.md](manifest/designs/loom.md) and [manifest/designs/shed.md](manifest/designs/shed.md) for the design record, and the `internal/shedadapters` package documentation for the round-artifact contract the two rows share.

## Contracts

`contracts/` holds what crosses a module boundary, versioned with the code that reads it:

- **`contracts/stencils/`** — every prompt an agent is given, shipped as an embedded default and **read from the hub's stencils directory at call time**, never from a compiled-in copy, so an operator can edit a live prompt without a rebuild.
  `lyx stencil` is the surface over that: `diff` shows upstream changes not yet taken (or, with `--all`, board edits not yet ported back), and `promote` copies an edit back into this source tree.
- **`contracts/specs/`** — the on-disk format contracts (`loom-plan-spec.md`, `loom-status-spec.md`, `webster-spec.md`, `final-summary-spec.md`, `llm-model-spec.md`), each with exactly one parser package in `internal/`.
- **`contracts/recipes/`** — `loom-recipe.yaml`, the producer list above.

## Building

```bash
go build ./cmd/lyx        # build the lyx binary
go test ./...             # run the full suite (structural invariants included)
```

`./deploy` (`deploy.cmd` on Windows) builds and installs `lyx` onto PATH;
`./deploy-dev` targets a derived `.dev-bin` instead, so a dev build never overwrites the production install.

To start a hub, run `lyx fabric clone <weft-url> [<warp-url>]` — it clones both repos, wires the junctions, materializes every module's config, and creates `_board`, in one call.
Then `lyx fabric add <slug>` for a task worktree, and `lyx run` inside it.

## Sandbox Hub

The **sandbox Hub** is a dedicated bench for dogfooding `lyx` against itself, exercising the real deployed binary end to end against a throwaway hub cloned from `lyx-test`/`lyx-test-weft`.
Each suite is an agent script driving the binary and reporting findings.

Build it with `sandbox/posix/build.sh` (`sandbox/win/build.cmd` on Windows), run a suite with `sandbox/posix/core-suite.sh` — plus `fabric-`, `reed-`, `reed-watch-`, `shuttle-`, `burler-`, and `webster-suite.sh` for the per-module benches — and collect findings with `sandbox/posix/fetch.sh`.
See [docs/sandbox-howto.md](docs/sandbox-howto.md) for the runbook.

## Plugins

`plugins/` ships two Claude Code plugins from this marketplace (`.claude-plugin/marketplace.json`), each its own Go module or skill set:

- **prowler** — fetch blocked, restricted, or JS-rendered web pages and output readable markdown, plus cross-repo code search.
- **scribe** — code-writing conventions: quality, comments, testing, and Go mechanics.

`tools/` holds the repo's own dev tools (`deploy`, the sandbox driver, and the `mdreflow`/`godocreflow`/`wordswap` text-mechanics sweepers).

## Requirements

- [Claude Code](https://claude.ai/code)
- Go 1.26+
- Git 2.42+ (for `git worktree add --orphan`)
- tmux (for the orchestration layers;
  on Windows via psmux)
- A resolvable GitHub token for `selfreport` and `Publish`: set `GH_TOKEN` or `GITHUB_TOKEN`, or have the `gh` CLI installed and authenticated (`gh auth login`) as a fallback token source — `gh` is not required when either environment variable is set

## Documentation

- [CONSTRAINTS.md](CONSTRAINTS.md) — the repo's structural invariants (authoritative).
- [docs/overview.md](docs/overview.md) — architecture, naming, module and shared-lib map.
- [docs/shared-libs/](docs/shared-libs/README.md) — the shared infrastructure packages under the modules.
- [manifest/roadmap.md](manifest/roadmap.md) — what's planned and what's shipped.
- [manifest/designs/](manifest/designs/) — per-module design docs for planned, not-yet-built modules.
- [crucible/](crucible/README.md) — `crucible`, the hand-run serial review+fix loop for hardening a live-substrate module before merge (not documentation of shipped code, so it lives at the repo root, not under `docs/`).

Per-package documentation lives in each package's own `doc.go` and is the durable detail for anything shipped;
a design doc under `manifest/designs/` is deleted once its module ships.
