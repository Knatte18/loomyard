Load skills `scribe:prose`, then `scribe:conversation`, before reading the rest of this document.

# Handoff — loomyard on lyx

The operator drives in Norwegian; reply in Norwegian.
This session is the hub orchestrator ("lyxhub:orch"), sitting in the hub prime `~/Code/loomyard-LYXHUB/loomyard` on `main`.
This file is updated only when the operator asks.

## How the operator wants this run

- **You are the hub: act.**
  Routine loop steps (board upsert, fabric add, loom start, review, approve, land, deploy, cleanup, recording findings on the board) are done without asking.
  Ask only for real choices: which task next, design decisions, anything the operator has not agreed to.
- **Always give the reed command** whenever a run is started or reported: `cd ~/Code/loomyard-LYXHUB/<slug> && lyx reed attach` (detach `Ctrl+b d`).
- **Watch runs silently.**
  Use one background `until`-loop per run on `<hub>/<slug>/_lyx/shed/self/status.json` that exits when `state` leaves `running`; no per-producer Monitor events.
- **Findings go on the board**, never as GitHub issues (all loomyard issues are closed with pointers to the board).
  Extend an existing task's or note's `body` (`lyx board upsert` / `lyx board notes upsert`, which merge fields); create new notes freely, but new tasks only when the operator agrees.
  Findings that belong to another repo (e.g. quarry) go to that repo's issues; its orchestrator session (`quarry:orch`) can be messaged directly.
- In design discussions, give an assessment and get explicit agreement before editing or committing.

## Run loop, per task

1. `lyx board upsert` the task, `lyx fabric add <slug>`, then `cd <hub>/<slug> && lyx loom start --no-attach` (spawns the ly-drive strand).
2. The run stops at Publish with a PR, reported as `blocked` (the operator dislikes that word for a ready PR; the fix is in `operator-surface`).
   Review the PR with a background general-purpose subagent (build/vet/test, correctness, deploy impact); relay findings.
3. Land: in the task worktree run `lyx loom approve`, then SendMessage the ly-drive session (name from `ListAgents`, e.g. `<slug>-xx`) to re-step.
   Finalize squash-merges locally with one `Co-Authored-By`, closes the PR and sets the board task `done`.
4. Deploy from the prime: `git pull`, `./update-plugins.sh`, then `lyx config reconcile` (preview) / `--apply`; commit reconciled config in `~/Code/loomyard-LYXHUB/loomyard-weft` (`main-weft`, local only).
5. Read the pair's `.lyx/loom/friction/` and `.lyx/shed/self/drive-report-*.md`, record findings, then `lyx fabric remove --remote <slug>` (it now deletes the local warp branch too).

Messages to ly-drive sessions are held until the operator approves them in that pane (bypass mode) — say so when sending.
Never type into agent panes during a run: a reply there gets classified as "asking" and blocks the run (see `operator-surface` below).
Grey text in a pane's prompt line is a Claude Code suggestion, not operator input.
To look at panes: `tmux -S /tmp/tmux-1000/lyx-loomyard-LYXHUB-40e21604 capture-pane -p -t <pane>`; pane ids from `lyx reed status` in the worktree.

## In flight

- **`operator-surface`** is running, last seen in Webster (28 cards, one per batch).
  ly-drive session `operator-surface-bb`.
  Its scope is the board task body; its plan is in `<hub>/operator-surface/_lyx/plan/`.
  Until it lands, a re-step after an "asking" stop spawns a fresh strand instead of attaching (#297); if that happens, remove the stale strand with `lyx reed remove <guid>` and tell the new session what already exists.
  It was planned against `main` before `d619100af`; expect Publish's merge of `main` to touch the review-path change.
- **Deploy is pending and blocked on `operator-surface`.**
  `prod` = `9c9afaa97`; `main` = `d619100af` (burler-review) is not deployed.
  `d619100af` moves review rounds from `.lyx/loom/reviews/` to `_lyx/reviews/`; deploying while `operator-surface` is mid-review makes its Bouncer find no report and stick.
  Deploy once `operator-surface` has landed, then both changes go out together.
- After `operator-surface` lands: the operator was offered `quarry-cli-answers` or `focus-marker-warn` as the next small task; no answer yet.

## Open, not yet recorded anywhere

- Tests leak tmux sockets: `/tmp/tmux-1000/` holds hundreds of `lyx-Test…`, `lyx-contract-…`, `lyx-warp-bare-…` sockets.
- Stencil board copies warn "drifted from worktree source" on every hub command; `operator-surface` (#286) changes the remedy text, but the drift itself should be checked after deploy.
- Warp branches of removed pairs remain on origin (recorded in note `fabric-branch-hygiene`; do not delete them by hand).

## Open design points

- **Replace `manifest/` with the board's Manifest section** — the operator wants it eventually, but the Millhouse fallback track (`~/Code/loomyard/wts/loomyard`) still reads `manifest/`.
  Proposed a board note conditioned on retiring the mill track; not answered, not recorded.
  Until then, Someday lives in `manifest/roadmap.md` and board notes point there (as `kick-start-pack` does).
- **`main-weft` is never pushed** and never advances on landing; lazy by design or a weft-side landing gap. Uninvestigated.
- **Opening a task in VS Code, one operation:** extend `lyx ide spawn <slug>` to create the pair when missing (ide depends on fabric, never the reverse) and generate a VS Code task running `lyx reed attach` on folder open. Not recorded.
- **Shared conventions:** `Knatte18/scribe` (local `~/Code/scribe`) is the one source; version frozen at `1.1.0`, deploy with its own `./update-plugins.sh`, never bump.
  The operator's `~/.claude/CLAUDE.md` still carries sed wording that reads as allowing awk for edits.
- **Agent context baseline** is ~47k tokens per spawned agent, mostly Claude Code's own tool definitions; motivates the Someday item "shuttle `Spec`: generic tools-restriction".

## Suggested skills

- `scribe:prose`, `scribe:conversation` — before writing anything.
- `mill:git-workflow` — for commits in the hub prime.
- `ly:ly-drive` — only to understand what the driver does; the orchestrator never runs it.
- `scribe:handoff` — for the next handoff.
