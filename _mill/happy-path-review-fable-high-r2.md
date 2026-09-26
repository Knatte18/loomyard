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

### Log — fixture hub1 and the go-driver run (appended live)

Fixture project: `example.com/tasktool`, packages `task` (domain), `store` (JSON file persistence, imports `task`), `cmd/tasktool` (CLI, imports `store`), each with tests; committed `405ccf6` on `main`, pushed into `hub1/warp.git` (HEAD re-pointed to `refs/heads/main`); `hub1/weft.git` is `git init --bare` with the host default, HEAD `refs/heads/master` (no `init.defaultBranch` on this host) — the F1 shape.

Operator sequence (dev binary `L=.../.dev-bin/lyx`):
1. `cd $ROOT && $L fabric clone --into $ROOT $ROOT/weft.git $ROOT/warp.git` → ok, hub `$ROOT/warp-LYXHUB`; weft prime on `main-weft`, `_board` on `main` (F1 CONFIRMED sound live, weft bare HEAD was `master`).
2. `cd $HUB/warp && $L config loom --set selfreport=false --set 'friction='` → ok. `$L config landing --set 'require_pr_to_base=[]'` → ok, file shows `require_pr_to_base: []`. Unrelated `$L config landing --set squash=true` keeps `[]` (F6 CONFIRMED). `$L config reconcile` dry-run reports no added/removed for any module. Weft prime log shows two `weft sync` commits on `main-weft` after the sets, `fabric status` clean.
3. `cd $HUB/warp && $L board upsert '{"slug":"task-priority",...}'` (brief in the board; see "The task" below) → ok. `$L fabric add task-priority` → ok, pair `task-priority`/`task-priority-weft`, both pushed to the local bares. Task worktree's `_lyx/config/loom.yaml` shows `selfreport: false`, `friction: ""`; `landing.yaml` shows `require_pr_to_base: []` — the override was inherited from `main-weft`.
4. `cd $HUB/task-priority && $L shed seed self --recipe loom --driver go --param parent=main` → `{"driver":"go","ok":true,"recipe":"loom","run_id":"self"}`; `seed.json` = `{recipe: loom, driver: go, params: {parent: main}}`. `fabric status` shows the seed as an uncommitted weft change (expected: `loom start`'s bootstrap commits it).
5. `cd $HUB/task-priority && $L loom start --no-attach` → exit 0, **no output at all** (no JSON envelope). Immediately after: `lyx loom status` = `running`, `current_producer: Discussion-Write`, `history_length: 2`; tmux server `lyx-warp-LYXHUB-59652fb8` session `task-priority` up; `lyx reed watchdog`, `lyx loom run` (detached driver), `lyx loom status --watch` (status strand) and the Discussion-Write `claude --model opus --effort high` session are all running. `.lyx/loom/driver.log` exists, 0 bytes so far.

Network check: `selfreport: false` + `friction: ""` in the task's effective `loom.yaml`; `require_pr_to_base: []` so Publish takes the no-pull-request branch (the only `githubclient` caller in landing); the warp remote is `hub1/warp.git`, the weft remote `hub1/weft.git`.

hub2 (for the llm run, not started until hub1 is terminal) prepared with the same sequence: fresh bares, clone, the two config overrides, the same board task, `fabric add task-priority`; effective config verified in the task worktree; `.claude/skills/ly-drive/SKILL.md` copied from this worktree's `plugins/ly/skills/ly-drive/SKILL.md`; `.claude/` appended to `<hub>/warp/.git/info/exclude` (the pair's common git dir); `lyx fabric status` clean, `git status --ignored` shows `.claude/` as ignored.

Provisional findings so far:
- P1 (LOW): `lyx loom start --no-attach` returns exit 0 with no output at all on success (`internal/loomcli/start.go:404`, `if !mustAttach(noAttachFlag) { return nil }`), while the CLI/Cobra Invariant admits only the terminal-handover tail as JSON-exempt. An operator or script cannot tell "driver up" from a silent failure without a follow-up `status`. CONFIRMED (hub1).

hub1 go run timeline (from `lyx loom status` polls):
- 17:31 Preflight → Loom-Preflight → Discussion-Write (opus high) — `_lyx/discussion/{decision-record,support-log}.md` written by 17:33.
- 17:34:17 Discussion-Bouncer (seed pass, `round-1-focus.md`) → stuck → 17:34:47 Discussion-Burler round 1 (`round-1-review.md` verdict APPROVED with 7 findings F1–F7, `round-1-fixer-report.md` fixed all) → stuck → 17:37:17 Discussion-Bouncer judge: `round-1-bouncer-verdict.md` = APPROVED, ledger + `round-2-focus.md` written → done → 17:37:47 Plan-Write. No bounce in the discussion segment (first judge verdict APPROVED).
