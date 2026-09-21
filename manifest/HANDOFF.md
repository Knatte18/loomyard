Load skills `mill:conversation` and `mill:prose` before reading the rest of this document.

# Handoff

## Where you are

`/home/hanf/Code/loomyard/wts/loomyard` — the **main** worktree, direct push allowed here per `CLAUDE.md`.
Clean tree, in sync with `origin/main` at `74ffd5f9f`. No Monitor waits armed, no forks running.

The user drives design in Norwegian; reply in Norwegian.
In design discussions, give your assessment and get explicit agreement BEFORE editing or committing.

## In flight

### `#017 lyx-bin-pane-path` — has a verdict waiting

Worktree `wts/lyx-bin-pane-path`. Started with `mill-start --orch`; `_mill/orch-review.md` is written.

**REQUEST_CHANGES, 2 BLOCKING (1 design, 1 scope), 0 NIT.**
Lead finding: the discussion specifies both shell dialects but never says which `shell.Shell` composes the prelude.
The pane's real shell is `e.cfg.Shell`, operator-overridable via `LYX_REED_SHELL`, so a `ForGOOS()`-keyed choice emits pwsh syntax into bash — and `Chain` joins with `;`, so the launch line gets a syntax-error prefix while the PATH guarantee fails silently.

The worker picks the file up and resumes on its own.
`--orch` waits for `orch-review.md` on discussion-review **round 1 only** — re-running `/orch-review` against this task later does nothing.

### `#015 crucible-batten-end-to-end` — spawned, not started

Worktree `wts/crucible-batten-end-to-end`.
Runs as the crucible orchestrator role (`crucible/orchestrator-prompt.md`), not through the mill-start chain — so the `phase: discussing` that `mill-spawn` seeded into `_mill/status.md` drives nothing.

Its wiki brief carries the disposable fixture-hub recipe, the GOPROXY note below, and the queued sabotage ideas.
Read it through the wiki daemon client.

The two tasks are file-wise disjoint; neither blocks the other.

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
- `mill:orch-review` only if a NEW task is started with `--orch`; this session owns the Monitor wait, and forks only after `discussion.md` exists.
- `mill:git-workflow` — `CLAUDE.md` overrides its `--onmain` gate for this worktree.
