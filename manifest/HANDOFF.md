Load skills `scribe:prose`, then `scribe:conversation`, before reading the rest of this document.

# Handoff — loomyard on lyx

The operator drives in Norwegian; reply in Norwegian.
This session is the hub orchestrator ("lyxhub:orch"), sitting in the hub prime `~/Code/loomyard-LYXHUB/loomyard` on `main`.
This file is updated only when the operator asks.

## How the operator wants this run

- **You are the hub: act, and fix.**
  Talk to the operator about what to build, priorities and design choices; everything else is yours: start runs, review PRs, un-wedge stuck runs, approve, land, deploy, clean up, record findings, start the next task.
  Un-wedging includes small code fixes committed and pushed directly to `main` from the prime, then `./update-plugins.sh`; the operator explicitly authorised deploys ("Du får fikse den stucken", "det er jo bare å fikse alt").
  New board tasks were agreed wholesale ("Omgjør alt dette til tasks"); still ask before inventing a new task on your own initiative (notes are free).
- **Run everything that can run in parallel** ("Kjør alt som KAN kjøres i paralell"); hold back only tasks whose packages overlap a running task.
  The operator finds nine at once a lot but accepted it; follow runs closely ("følg GODT med").
- **Always give the reed command**: `cd ~/Code/loomyard-LYXHUB/<slug> && lyx reed attach`; `Ctrl+b s` in any attached client lists every run's session with previews.
- **Findings go on the board** (`lyx board notes upsert`, fields merge), never GitHub issues.
- Say "awaiting" for a ready PR, never "blocked".

## Run loop, per task

1. `lyx fabric add <slug>`, then `cd <hub>/<slug> && lyx loom start --no-attach`.
   Status file: `<hub>/<slug>/_lyx/shed/<slug>/status.json` (`state`: running / awaiting / blocked / failed / done).
2. **Watch** with one background loop over every `<hub>/*/_lyx/shed/<slug>/status.json` (skip the `-weft` twins) that exits when any run leaves `running`, plus a 20-minute health check (state, minutes since status changed, live `claude` panes per tmux session on socket `/tmp/tmux-1000/lyx-loomyard-LYXHUB-40e21604`).
3. **awaiting at Publish**: review the PR (small diff: yourself — build/vet/test the touched packages; large: a background general-purpose subagent, read-only, with build/vet all tags/test and deploy impact).
   Then in the worktree `lyx loom approve`, **wait until the session has no `claude` pane left** (the driver removes itself after a hand-back; an early `loom start` races that removal and leaves no driver — board note `driver-restart-race`), then `lyx loom start --no-attach`.
   Finalize squash-merges, closes the PR, sets the board task `done`.
4. **blocked / failed**: read `error` in the status file; rerun failing tests yourself to tell flaky from real.
   Real regressions in the task: a background subagent fixes them in the task worktree (warp commits only, no `lyx fabric`, no `_lyx`/`.lyx`, no push), then `loom start` again.
   Lyx bugs: fix on `main`, deploy, `loom start` again.
   **Never `lyx fabric merge-in main` while Webster is mid-batch**: record-batch then refuses the batch (`webster-parent-merge-midrun`); merge-in is safe only when all batches are recorded (e.g. blocked at the integration verify).
5. After landing, from the prime: `git pull`, `./update-plugins.sh`, `lyx config reconcile` (preview, then `--apply`; commit changed config in `~/Code/loomyard-LYXHUB/loomyard-weft` with `git -C`), read friction (`.lyx/loom/friction/` or its new `_lyx` home once `batten-hub-ready` lands) and the drive report, record findings, `lyx fabric remove --remote <slug>`.

## In flight (2026-09-30, ~16:40)

`prod` = `199aa315e` (main).
Runs, all started with the llm driver:

- `batten-hub-ready` — Webster-Review; hub-side fix commit `a25c57c0d` (integration regressions) sits on its branch.
  It changes where friction and drive reports live (`_lyx`) and teaches batten to wait through `awaiting`.
- `verify-triage` — Webster; rerun/parent-compare so flaky or pre-existing test failures stop halting runs.
- `hub-orch-strand` — Plan; lyx hosts this orchestrator in tmux and cycles its context via `/scribe:handoff` + `/clear` (the operator's answer to compaction).
- `plan-write-respawn`, `publish-verify-after-merge`, `reed-launch-script` — Webster.
- `trace-log-retention` — Plan-Review.

Queue (board `depends_on`, held back for package overlap): `webster-parent-merge-midrun` ← `verify-triage`; `refusal-audit` ← `webster-parent-merge-midrun`; `status-strand-by-driver`, `ly-drive-safe-repairs`, `fabric-readd-weft-push` ← `batten-hub-ready`; `batten-open-ide` ← `fabric-readd-weft-push`, `verify-triage`, `status-strand-by-driver` (first task to run through `lyx batten run` instead of the manual loop).
Start each as soon as its dependencies land.

## Priorities (agreed with the operator)

1. Fewer and recoverable stops: a false stop counts as badly as a wrong landing; every refusal needs a way forward (`verify-triage`, `webster-parent-merge-midrun`, `refusal-audit`).
2. Automate the loop: batten, driver self-repairs (`ly-drive-safe-repairs`), orchestrator context cycling (`hub-orch-strand`).
3. Records survive teardown (`batten-hub-ready`, `trace-log-retention`).
4. Operator surface (panes, VS Code, launch line).

## Open with the operator

- Offered, unanswered: a one-screen overview of all runs (one line per run: state, producer, age), as a hub-workspace file or a self-refreshing `lyx` command.
- The hub VS Code workspace is live: `cd ~/Code/loomyard-LYXHUB/loomyard && lyx ide spawn loomyard` (prime + `_board` + `_portals`).
- `collapsed_rows: 16` is set in `main-weft` (`18a4950`) so the ly-drive pane stays readable; pairs created before that keep 3.
- **Replace `manifest/` with the board's Manifest** — wanted eventually; the Millhouse track (`~/Code/loomyard/wts/loomyard`) still reads `manifest/`.
- **`main-weft` is never pushed** and never advances on landing; uninvestigated.

## Suggested skills

- `scribe:prose`, `scribe:conversation` — before writing anything.
- `mill:git-workflow` — for commits in the hub prime.
- `ly:ly-drive` — only to understand what the driver does; the orchestrator never runs it.
- `scribe:handoff` — for the next handoff.
