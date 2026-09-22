Load skills `mill:conversation` and `mill:prose` before reading the rest of this document.

# Handoff

## Where you are

`/home/hanf/Code/loomyard/wts/loomyard` — the **main** worktree, direct push allowed here per `CLAUDE.md`.
Clean tree, in sync with `origin/main` at `36900c6f5`. One worktree, nothing active, no Monitor waits armed, no forks running.

The user drives design in Norwegian; reply in Norwegian.
In design discussions, give your assessment and get explicit agreement BEFORE editing or committing.

## Nothing in flight — two unclaimed candidates

Neither is spawned. Read each brief through the wiki daemon client rather than trusting a summary here.

**`#018 llm-driver-trust-dialog-hang`** is the smaller of the two, and smaller than its brief suggests.
A `driver: llm` run can park forever on Claude Code's workspace-trust dialog.
The brief already records that the gate keys on the worktree's absolute path in `~/.claude.json`'s `projects` map (the field is `hasTrustDialogAccepted`), and that the hang is in the OUTER `ly-drive` session's own `claude` launch, not the inner phase agents.

What the brief does NOT say, and which shrinks the task considerably: **the dismiss machinery already exists and is in production.** `internal/shuttleengine/wait.go:482` classifies the capture as `StartupTrustPrompt` and plays `TrustDismissSequence`, non-fatally. The llm arm's `awaitDriverPane` only probes liveness and never calls it. So the fix is wiring an existing seam into one more code path, not building anything.

That also argues against the brief's other suggested direction (pre-seeding the trust flag): every run mints a new fixture path, so it would grow `~/.claude.json` without bound, and two concurrent runs read-modify-writing the operator's own file would race.

Reproduction needs a path this host has never trusted. This machine already trusts 12, several of them campaign fixtures — a repro on a reused path passes silently and proves nothing, which is exactly how round 3 lost it.

**`#019 crucible-batten-followup`** is the campaign's safety pass, never achieved — five rounds, none of which found nothing.
Its own lesson, per the brief: the last two rounds' yield came from looking OUTSIDE batten's packages.

## Still open from the work that landed

`#015` and `#017` are both merged and cleaned up. Three review notes on `#017` (PR #261) were judged non-blocking at merge and nothing else records them:

- The PR summary claims the prelude also resolves the agent binary (`claude`). It does not — only `lyx`'s own directory is prepended; the `claude` half is a manual pre-condition check in `SANDBOX-SHUTTLE-SUITE.md`.
- The `Pane Binary Resolution` clause pins the dialect to `shell.ForGOOS()`, which is not the pane's real shell when an operator sets `LYX_REED_SHELL`. Pre-existing, and `Chain` joins with `;` so a rejected prelude never kills the launch line — but the assumption is now normative and the clause does not name the override.
- `Shell Mechanics Seam` was softened in passing (its method list is now "illustrative, not exhaustive") to make room for `ExportEnv`/`PrependPathEntry`/`Chain`.

## Rules in force

`manifest/` holds only unbuilt work: a design doc whose work has shipped is deleted, and a doc pinning a format shared between modules moves to `contracts/specs/`.
No tombstones — a dropped idea is removed, not recorded as dropped.
`manifest/designs/` is down to ten docs, all unbuilt, no Go file.

`plugins/scribe/skills/prose/SKILL.md` gained a **Never pin a count** section. It governs every doc and comment you write.

## Machine note (hanf/WSL2)

`GOPROXY=direct` is set machine-locally and should stay.
The route to `proxy.golang.org` stalls partway through any large object from this network — the `.zip` dies at exactly 147 456 B under HTTP/2 and hangs under HTTP/1.1, while github.com serves the same 803 268 bytes fine.
MTU was not the cause: identical truncation at 1400 and 1280.
`GOSUMDB` is untouched; `GOPRIVATE` is deliberately empty, since quarry is public.

A build failing on `github.com/Knatte18/quarry@v0.2.0` is this environment, never a code defect.

## Needs a decision

- The roadmap's two `shuttle Spec` items declare themselves unmotivated (*"stays unmotivated rather than blocked on anything"*, *"meaningless until a second engine lands"*). Under the rule above they belong in GitHub issues, not `manifest/`.
- A spec deployed by an earlier version stays on disk in target repos — `stencilstore` seeds and reconciles registered names and has no removal path, so a stale `loom-plan-card-format.md` may linger in one.
- [millhouse#1127](https://github.com/Knatte18/millhouse/issues/1127) asks `mill-setup` Phase 4.8 to retire the operator's target-blind `Bash(rm -rf:*)` deny rule for target-scoped ones. Until it lands, that rule still blocks scratch and fixture cleanup on every machine. Nothing in this repo's settings was changed; the fix is deliberately owned by millhouse, which already owns `~/.claude/settings.json` via `_claude_settings.py`.

## Suggested skills

- `mill:prose` + `mill:conversation` — load before writing anything.
- `mill:mill-status` / `mill:mill-inspect` — confirm task state before acting.
- `mill:mill-spawn` — both candidates above need one; `CLAUDE.md` requires the user's explicit say-so before creating a worktree.
- `mill:orch-review` only if a task is started with `--orch`; this session owns the Monitor wait, forks only after `discussion.md` exists, and it applies to discussion-review round 1 alone.
- `mill:git-workflow` — `CLAUDE.md` overrides its `--onmain` gate for this worktree.
