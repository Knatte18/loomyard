Load skills `mill:prose`, then `mill:conversation`, before reading the rest of this document.

# Handoff

## Where you are

`/home/hanf/Code/loomyard/wts/loomyard` — the **main** worktree, direct push allowed here per `CLAUDE.md`.
Clean tree, in sync with `origin/main` at `e507d96de`.
No Monitor waits armed, no forks running.

The user drives design in Norwegian; reply in Norwegian.
In design discussions, give your assessment and get explicit agreement BEFORE editing or committing.

## In flight: `#018 llm-driver-trust-dialog-hang`

Worktree `wts/llm-driver-trust-dialog-hang`, phase `discussed`.
The discussion closed on an APPROVE after six rounds; every round's review and fix is under that worktree's `_mill/reviews/`.
Next step is `/mill-plan`, run by a session in that worktree — not from here.

The approach was settled before the discussion started and is recorded in the task's wiki brief under `## Decided approach`.
The substance the plan has to carry is the seam: `awaitDriverPane` (`internal/loomcli/driverlaunch.go`) is handed only `driverPaneProbe.Strands`, and dismissing the dialog also needs a pane capture, `Engine.Startup` classification, and input playback.

## Unclaimed

`#019 crucible-batten-followup` — the campaign's safety pass, never achieved.
Read its brief through the wiki daemon client; its own lesson is that the last two rounds' yield came from looking outside batten's packages.

## Open findings

**The "added forms, never widened signatures" decision has no definition site.**
It is cited six times — `burlerengine/engine.go:26`, `shedadapters/singlellm.go:40` and `:104`, `shuttleengine/attach.go:42`, `shuttleengine/run.go:260` and `:390` — and defined in none of `CONSTRAINTS.md`, `docs/`, or `manifest/`.
Only `singlellm.go:40` states the condition (add a form when the seam is shared by callers that will never use the parameter; widen otherwise); the other five give the conclusion alone.
The user rejected appending a section to `CONSTRAINTS.md`, since accretion makes it long and badly written.
Unresolved; the alternative on the table is to designate one existing site as the definition and have the other five name it, adding no new prose.

**Three `#017` review notes** (PR #261) were judged non-blocking at merge and nothing else records them:

- The PR summary claims the prelude also resolves the agent binary (`claude`). It does not — only `lyx`'s own directory is prepended; the `claude` half is a manual pre-condition check in `SANDBOX-SHUTTLE-SUITE.md`.
- The `Pane Binary Resolution` clause pins the dialect to `shell.ForGOOS()`, which is not the pane's real shell when an operator sets `LYX_REED_SHELL`. `Chain` joins with `;`, so a rejected prelude never kills the launch line, but the clause does not name the override.
- `Shell Mechanics Seam` was softened in passing (its method list is now "illustrative, not exhaustive") to make room for `ExportEnv`/`PrependPathEntry`/`Chain`.

## Rules in force

`manifest/` holds only unbuilt work: a design doc whose work has shipped is deleted, and a doc pinning a format shared between modules moves to `contracts/specs/`.
No tombstones — a dropped idea is removed, not recorded as dropped.

`plugins/scribe/skills/prose/SKILL.md` carries a **Never pin a count** section. It governs every doc and comment you write.

Everything in `.scratch/` is disposable by definition, and the user empties it at will.
Never put anything there whose loss would matter, and never treat a file vanishing from it as an event worth reporting.

Wiki writes go through the daemon client or `/mill-*` skills only. Reading the wiki's git history is fine — use `git -C /home/hanf/Code/loomyard/wiki`, never `cd` into it.

## Machine note (hanf/WSL2)

`GOPROXY=direct` is set machine-locally and should stay.
The route to `proxy.golang.org` stalls partway through any large object from this network, while github.com serves the same bytes fine; MTU was ruled out.
`GOPRIVATE` is deliberately empty, since quarry is public.
A build failing on `github.com/Knatte18/quarry@v0.2.0` is this environment, never a code defect.

## Needs a decision

- The roadmap's two `shuttle Spec` items declare themselves unmotivated (*"stays unmotivated rather than blocked on anything"*, *"meaningless until a second engine lands"*). Under the rule above they belong in GitHub issues, not `manifest/`.
- A spec deployed by an earlier version stays on disk in target repos — `stencilstore` seeds and reconciles registered names and has no removal path, so a stale `loom-plan-card-format.md` may linger in one.
- [millhouse#1127](https://github.com/Knatte18/millhouse/issues/1127) asks `mill-setup` Phase 4.8 to retire the operator's target-blind `Bash(rm -rf:*)` deny rule for target-scoped ones. Until it lands, that rule still blocks scratch and fixture cleanup on every machine.

## Suggested skills

- `mill:prose` + `mill:conversation` — load before writing anything.
- `mill:mill-status` / `mill:mill-inspect` — confirm task state before acting.
- `mill:mill-spawn` — `#019` needs one; `CLAUDE.md` requires the user's explicit say-so before creating a worktree.
- `mill:orch-review` — only when the user starts a task with `--orch`; this session owns the Monitor wait and forks only once `discussion.md` exists.
- `mill:mill-quick` — fits a mechanical, compiler-checked task; `mill-config.yaml` has the `done_gate` it requires. It runs no reviewer, so verify the suite yourself and read the doc comments it wrote.
- `mill:git-workflow` — `CLAUDE.md` overrides its `--onmain` gate for this worktree.
