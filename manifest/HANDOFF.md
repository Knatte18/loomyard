Load skills `scribe:prose`, then `scribe:conversation`, before reading the rest of this document.

# Handoff — loomyard on lyx

The operator drives in Norwegian; reply in Norwegian.
In design discussions, give an assessment and get explicit agreement before editing or committing.
This file is updated only when the operator asks.

## Where work happens now

loomyard is developed **through lyx**, from the lyx hub `~/Code/loomyard-LYXHUB/` (warp `Knatte18/loomyard`, weft `Knatte18/loomyard-weft`, private).
The orchestrator session sits in the hub prime `~/Code/loomyard-LYXHUB/loomyard` (on `main`), not in the mill worktree `~/Code/loomyard/wts/loomyard`, which is now only a fallback for when lyx itself is stuck.
Both are the same GitHub repo; the mill wiki is empty and no longer used for this work — tasks live on the lyx board (`lyx board list`).

Run loop, per task:

1. `lyx board upsert` the task (brief = the triaged issues it covers), then `lyx fabric add <slug>`.
2. Start it headless yourself: `cd <hub>/<slug> && lyx loom start --no-attach` — `llm` is the default driver, so this spawns an ly-drive strand that drives the whole run.
   The operator watches with `cd <hub>/<slug> && lyx reed attach` (detach: `Ctrl+b d`).
3. The run ends blocked at Publish with a PR (landing is PR mode). Review it, squash-merge with a clean message (never the Webster narrative), then tell the ly-drive session to re-step; it goes through Finalize to `done`.
4. Deploy from the hub prime: `git pull`, then `./update-plugins.sh` (see CLAUDE.md "Production lyx and plugins") — it moves `prod`.
5. Clean up: `lyx board set-status '{"slug":"<slug>","status":"done"}'` (#275), `lyx fabric remove --remote <slug>`, then `git branch -d <slug>` in the prime (#293 leaves it).

Messages to ly-drive sessions (SendMessage, name from ListAgents) are held for the operator's approval because those sessions run in bypass mode; tell the operator to approve in that pane.
Worker review-round-cap asks get `approve` immediately.

## In flight

- **`stuck-reason-in-status`** (#283): `done`, PR #296 squash-merged as `e29b9de5a`. Not yet deployed; deploy (step 4), then clean up (step 5).
  Its driver reported a hiccup worth an issue, not yet filed: Webster's Master ended its turn while waiting on its integration fork via Monitor; shuttle classified that as "asking" (`cleanedUp=false`) and the row blocked while the session kept working, and the next step killed and respawned the live strand instead of attaching. Details in `<hub>/stuck-reason-in-status/.lyx/shed/self/drive-report-20260930-095507-5c34.md` (read it before `fabric remove` deletes the pair).
- **`integration-verify-fence`** (#285): `done`, PR #294 merged and deployed. Only cleanup remains (step 5 above).
- Leftover local warp branches in the hub prime from removed pairs (#293): `board-done-on-landing`, `burler-exclude-warn`, `loom-start-layout`, `stencil-drift-remedy` — `git branch -d` each.
- Production: `prod` = `db68c5b8a`; `main` is ahead.

## Next: triage

Agreed with the operator: findings are filed as issues; triage groups issues into tasks (never one issue per task); the reader of the issues does the grouping.
Three board tasks to create, replacing the stale board entries `board-done-on-landing` and `stencil-drift-remedy`:

- **Landing:** #275, #276, #289, #292, #293.
- **Operator surface:** #295, #277, #286, plus three findings not yet filed as issues:
  - a deploy does not upgrade running sessions (`lyx loom start` finds the existing status strand via `IfAbsent` and keeps its old display);
  - layout rule: every pane but the bottom-most collapses to a fixed configurable minimum (3 rows), the bottom-most takes the rest, adding a pane moves nothing above it (`3+20` → `3+3+17`), Selvage excluded;
  - ly-drive reporting: name the run by slug, never `self`; give the run's own history position, not ly-drive's step counter; say when a reported state is from before it acted. Also `_lyx/shed/self/` should be named by slug, with `self` only an alias.
- **Review (Burler):** #287, #282, #263, plus one finding not yet filed: the 100-column comment width, and how a leading tab counts toward it, is written down nowhere, so a width finding costs a whole review round (friction note `burler-webster-r2.md` from `stuck-reason-in-status`).

Leave parked: #269, #270, #271, #274. #281 is an auto-filed anomaly already resolved — close it. #227/#228 are older ideas.

## Open design points

- **Opening a task in VS Code, one operation:** extend `lyx ide spawn <slug>` to create the pair when missing (ide depends on fabric, never the reverse; fabric must not call ide), and generate a VS Code task that runs `lyx reed attach` on folder open (`runOptions.runOn: folderOpen`; the operator's settings already allow automatic tasks). Not `lyx loom start` — that starts a run. Not yet filed.
- **`main-weft` is never pushed** and never advances on landing; either lazy by design or a weft-side landing gap. Uninvestigated.
- **Shared conventions:** `Knatte18/scribe` (local `~/Code/scribe`) is the one source; version frozen at `1.1.0`, deploy edits with its own `./update-plugins.sh`, never bump. The operator's `~/.claude/CLAUDE.md` still carries the old sed wording that lets agents read awk as fine for edits.
- **Agent context baseline** is ~47k tokens per spawned agent, mostly Claude Code's own tool definitions; this motivates the Someday item "shuttle `Spec`: generic tools-restriction" (currently marked unmotivated).

## Suggested skills

- `scribe:prose`, `scribe:conversation` — before writing anything.
- `scribe:handoff` — for the next handoff.
- `ly:ly-drive` — only to read what the driver does; the orchestrator never runs it itself.
- `mill:git-workflow` — for commits in the hub prime.
