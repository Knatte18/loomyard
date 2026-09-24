Load skills `mill:prose`, then `mill:conversation`, before reading the rest of this document.

# Handoff

## Where you are

`/home/knatte/Code/loomyard/wts/loomyard` — the **main** worktree, direct push allowed here per `CLAUDE.md`.
Clean tree, in sync with `origin/main` at `2c7c0c251`.
No task worktrees exist, no Monitor waits are armed, no forks are running.

The user drives design in Norwegian; reply in Norwegian.
In design discussions, give your assessment and get explicit agreement BEFORE editing or committing.
Orient from `git log` and the wiki daemon client, not from this file alone — it goes stale between sessions.

## Just landed

`#018`, `#020`, `#021` and the `#019 crucible-batten-followup` campaign (`93559ad52`) are merged.
The campaign ran four rounds without a safety pass; its defects moved outward from batten's rows to fabric's crash windows under batten.
Its record (HANDOFF, round and fixer reports) lives in `_mill/` under the `archive/crucible-batten-followup` tag, never on shared ground — `crucible/campaigns/` was deleted for that reason.

## Unclaimed backlog, in dependency order

The campaign filed these; read each brief through the wiki daemon client.

1. `#025 fabric-pair-state-after-crash` — fabric owns the answer to a killed `Topology.Add`/`Remove`, collapsing batten's hand-combined probes.
   Likely subsumes `#022 fabric-rollback-keeps-warp-branch`; re-check `#022` after it lands.
   `#024 fabric-cleanup-remote-orphans` is related and still needs its own fix.
1. `#026 remedy-texts-followed-verbatim` — structured remedy commands composed from the caller's location, plus a verbatim-from-prime integration test.
1. `#027 crucible-batten-followup-2` — runs only after `#025` and `#026` land, since both change the code it presses on.

`#023 loom-done-after-friction` is independent: loom persists `done` before Tier 2 friction reflection, so batten's teardown deletes the friction report.

## Pending cleanup on this machine

The user has not yet chosen what to delete; ask before acting.
A batch delete of branches was refused by the auto-mode permission classifier, so each deletion needs the user's explicit go-ahead.

- Local branches, mostly `mill-checkpoint-*`, whose tips are already in `main` or an `archive/*` tag.
- Local branches with commits found nowhere else: `backup/fable-high-r2-local-knatte`, `loom-step-supervisor`, `standalone-producers`, `mill-checkpoint-fabric-crucible-hardening`.
- Stray directories: `<container>/live-r2/` (a disposable August fixture of bare repos) and `wts/_board/` (not a git repo; only deployed stencils and specs).
- Remote branches of long-merged or abandoned work, listed by `git ls-remote --heads origin`.

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

Wiki writes go through the daemon client or `/mill-*` skills only. Reading the wiki's git history is fine — use `git -C <container>/wiki`, never `cd` into it.

## Machine note (hanf/WSL2 only, not knatte)

`GOPROXY=direct` is set machine-locally there and should stay.
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
- `mill:mill-spawn` — for the next backlog task; `CLAUDE.md` requires the user's explicit say-so before creating a worktree.
- `mill:mill-resume` — to pick up a task whose branch was pushed from another machine; `mill-spawn` would claim a new task instead.
- `mill:orch-review` — only when the user starts a task with `--orch`; this session owns the Monitor wait and forks only once `discussion.md` exists.
- `mill:mill-quick` — fits a mechanical, compiler-checked task; `mill-config.yaml` has the `done_gate` it requires. It runs no reviewer, so verify the suite yourself and read the doc comments it wrote.
- `mill:git-workflow` — `CLAUDE.md` overrides its `--onmain` gate for this worktree.
