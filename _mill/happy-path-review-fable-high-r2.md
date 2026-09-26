# happy-path — independent review, round 2 (tag `fable-high-r2`)

Worktree: `/home/knatte/Code/loomyard/wts/crucible-happy-path`, branch `crucible-happy-path`.
Dev binary: `.dev-bin/lyx` deployed from `4c7fea5db18987ad228d93852bd9172cadd569fd` (`happy-path: crucible re-seed r2`).
Production baseline: `/home/knatte/go/bin/lyx`.
Scratch root: `$HOME/crucible-happy-path/fable-high-r2/`.

## Executive summary

(filled in when Job 1 completes)

## 1a — Regression table (prior-round fix commits)

| sha | fix | verdict | evidence |
|---|---|---|---|
| `242d46983` | clone names weft primary/_board after the warp prime's branch | (pending) | diff reading: `checkedOutBranch(warpWorktreePath)` reads the warp clone's branch after step 5's `refuseUncheckedOutWarpClone`, so a detached/unborn warp is refused earlier; `suffixWeftPrimaryBranch` keeps the adopt path keyed on `origin/<branch>-weft`. Live: fixture weft bare is created with `git init --bare` on this host (default branch checked below). |
| `c4f0b1a39`, `3d59b14c6` | ly-drive writes envelopes to a private `mktemp -d` dir | (pending) | diff reading: rule is recipe-blind and names the loom-launched cwd case. Live: llm run transcript. |
| `5550dd00d` | Finalize pushes the parent branch | (pending) | diff reading: `PushBranch` → `PushWarpRebaseFreeAt` → `gitrepo.PushRebaseFree` runs `git -c push.autoSetupRemote=true push`, so a parent branch with no upstream gets one instead of failing; `SkipPush` honoured. Beyond the happy path: a parent with no remote at all fails the push and the row goes Stuck with "push it by hand" — correct, never a false Done. Live: bare warp `main` after landing. |
| `efc1f7053`, `d4f6a83d8` | help texts / launch prompt name the `ly` plugin; missing skill stops | (pending) | diff reading: prompt still under the launch-prompt byte cap (test). Live: the driver transcript must show the project-local stand-in skill loaded, no filesystem search. |
| `f51cb430f` | yamlengine sets/reconciles lists whole | (pending) | diff reading: the only reconciled template list is `landing.require_pr_to_base` (`burler` fans and `models` are `SeedOnly` in `configreg`), so whole-list carry drops nothing a normal upgrade relies on. `collectSequencePaths` does not descend into sequences, matching `sequenceBasePath`'s element grammar. Live: `lyx config landing --set 'require_pr_to_base=[]'` then an unrelated `--set`. |
| `761dc64a5` | `shed seed --help` loom example | (pending) | to run verbatim on a fixture task worktree then `lyx loom start`. |
| `ff6e654ba` | ly-drive resumes blocked/paused/failed baseline | (pending) | skill text read; live only if the llm run is ever resumed. |
| `655bcb6be` | ly-drive names how to wait for a backgrounded step | (pending) | skill text read; live: the driver transcript's wait mechanism. |

## 1b — Fixture and operator sequence

(filled in as the sequence is executed)

## Evidence for the task's three conditions

(per run)

## Findings

(severity-ranked, appended provisionally as spotted)

## Out-of-scope observations

(none yet)

## What was tested

- `./deploy-dev` → `Deployed lyx @ 4c7fea5db (26223 KB) .../.dev-bin/lyx`.
- Pre-existing processes: no live tmux server (`/tmp/tmux-1000/*` sockets are all stale), the operator's own `claude` sessions, and the millhouse wiki daemon; nothing of mine yet.
- `lyx fabric clone --help`, `lyx board upsert --help`, `lyx fabric add --help`, `lyx shed seed --help`, `lyx loom start --help`, `lyx config --help` read; each names its flags and a usable example.
