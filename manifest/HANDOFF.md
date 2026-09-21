Load skills `mill:conversation` and `mill:prose` before reading the rest of this document.

# Handoff

## Where you are

`/home/hanf/Code/loomyard/wts/loomyard` — the **main** worktree, direct push allowed here per `CLAUDE.md`.
Clean tree, in sync with `origin/main` at `bf0b20038`. No Monitor waits armed, no forks running.

The user drives design in Norwegian; reply in Norwegian.
In design discussions, give your assessment and get explicit agreement BEFORE editing or committing.

## In flight

### `#015 crucible-batten-end-to-end` — the only active task

Worktree `wts/crucible-batten-end-to-end`.
Runs as the crucible orchestrator role (`crucible/orchestrator-prompt.md`), not through the mill-start chain — so the `phase: discussing` that `mill-spawn` seeded into `_mill/status.md` drives nothing.

R1 and R2 are done and R3 was deliberately held for `lyx-bin-pane-path`, which has now landed, so R3 is unblocked.
The campaign keeps its own rolling handoff on its branch — read that for round state rather than anything here.

Its wiki brief carries the disposable fixture-hub recipe, the GOPROXY note below, and the queued sabotage ideas.

## Just landed

`#017 lyx-bin-pane-path` merged as `bf0b20038` (PR #261): every strand pane reed creates now prepends the spawning binary's directory to its `PATH` and exports `LYX_BIN`.
Marker is `[done]` but `wts/lyx-bin-pane-path` is still on disk — run `/mill-cleanup --apply`.

Three review notes were judged non-blocking at merge and are still open:

- The PR summary claims the prelude also resolves the agent binary (`claude`). It does not — only `lyx`'s own directory is prepended, and the `claude` half is a manual pre-condition check in `SANDBOX-SHUTTLE-SUITE.md`.
- The new `Pane Binary Resolution` clause pins the dialect to `shell.ForGOOS()`, which is not the pane's real shell when an operator sets `LYX_REED_SHELL`. Pre-existing, and `Chain` joins with `;` so a rejected prelude never kills the launch line — but the assumption is now normative and the clause does not name the override.
- `Shell Mechanics Seam` was softened in passing (its method list is now "illustrative, not exhaustive") to make room for `ExportEnv`/`PrependPathEntry`/`Chain`.

## Rules in force

`manifest/` holds only unbuilt work: a design doc whose work has shipped is deleted, and a doc pinning a format shared between modules moves to `contracts/specs/`.
No tombstones — a dropped idea is removed, not recorded as dropped.
`manifest/designs/` is down to ten docs, all unbuilt, no Go file.

`plugins/scribe/skills/prose/SKILL.md` gained a **Never pin a count** section this session. It governs every doc and comment you write.

## Machine note (hanf/WSL2)

`GOPROXY=direct` is set machine-locally and should stay.
The route to `proxy.golang.org` stalls partway through any large object from this network — the `.zip` dies at exactly 147 456 B under HTTP/2 and hangs under HTTP/1.1, while github.com serves the same 803 268 bytes fine.
MTU was not the cause: identical truncation at 1400 and 1280.
`GOSUMDB` is untouched; `GOPRIVATE` is deliberately empty, since quarry is public.

A build failing on `github.com/Knatte18/quarry@v0.2.0` is this environment, never a code defect.

## Needs a decision

- The roadmap's two `shuttle Spec` items declare themselves unmotivated (*"stays unmotivated rather than blocked on anything"*, *"meaningless until a second engine lands"*). Under the rule above they belong in GitHub issues, not `manifest/`.
- A spec deployed by an earlier version stays on disk in target repos — `stencilstore` seeds and reconciles registered names and has no removal path, so a stale `loom-plan-card-format.md` may linger in one.

## Suggested skills

- `mill:prose` + `mill:conversation` — load before writing anything.
- `mill:mill-status` / `mill:mill-inspect` — confirm task state before acting.
- `mill:orch-review` only if a NEW task is started with `--orch`; this session owns the Monitor wait, forks only after `discussion.md` exists, and it applies to discussion-review round 1 alone.
- `mill:mill-cleanup` — `wts/lyx-bin-pane-path` is merged and awaiting teardown.
- `mill:git-workflow` — `CLAUDE.md` overrides its `--onmain` gate for this worktree.
