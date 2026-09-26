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
- 17:37:47 Plan-Write → 17:40:17 Plan-Bouncer seed → 17:40:47 Plan-Burler round 1 (review APPROVED, 2 LOW findings fixed) → 17:43:17 Plan-Bouncer judge APPROVED (round 1, `_lyx/plan/` has 6 cards: 01-priority-type, 02-task-priority-field, 03-sort-by-priority, 04-store-priority, 05-list-priority, 06-add-priority-flag) → Batchifier (identity, `batcher.yaml` `active: ""`) → 17:44:45 Webster. No bounce in the plan segment either.
- 17:44:45 Webster (Master `sonnet`, 6 identity batches) → 17:47:57 done: commits `6ba5b50`..`a666100`, one per card, 8 files / +578 −23 across `task`, `store`, `cmd/tasktool`; `go vet ./... && go test ./...` pass at HEAD. `_lyx/webster/outcome.yaml`: `outcome: done, batches_done: 6`; `reports/integration.yaml` `status: OK`. Webster's own `summary.md` notes: (a) the integration fork ran only `go vet ./...` because its prompt listed only that, while the plan's `## verify:` lists `go vet ./...` AND `go test ./...`; (b) `record-batch` warned for batches 03–06 that "the fork never returned a final report" although each report landed.
- 17:48:17 Webster-Bouncer seed → 17:48:47 Webster-Burler round 1.

Provisional findings (cont.):
- P2 (MEDIUM): a multi-line plan-level `## verify:` section is silently truncated to its first line. `internal/planparser/sections.go:68` `plan.Verify = firstNonEmptyLine(extractSection(body, planVerifyHeading))`, while `contracts/stencils/loom/loom-template-plan.md:140` tells the planner the section is "one or more runnable shell commands". The integration suite (`.lyx/webster/prompts/integration.md` line 19) and the bisect path (`websterengine/integration.go` `runVerifyCommand`) therefore ran `go vet ./...` alone and never `go test ./...`; the plan-level gate reported OK without running the plan's tests. CONFIRMED (hub1). Fix: carry every non-empty line, joined with ` && `, so a failing later command fails the verify.
- 17:48:47 Webster-Burler round 1: `round-1-review.md` verdict APPROVED with LOW findings F1/F2, fixer committed `251dc69`, `cdc3317` on the task branch → 17:50:47 Webster-Bouncer judge APPROVED (`round-1-bouncer-verdict.md`, ledger, `round-2-focus.md`) → Publish done → Finalize done → Friction-Reflect done (Tier 2 off) — all three at 17:51:00; `lyx loom status` `state: done`, `history_length: 18`.

**go run landed: YES.** `git -C hub1/warp.git log main` = `2410e42 Persisted task priority for tasktool` (squash of `6ba5b50..cdc3317`, message = webster's `summary.md`) over `405ccf6`; `task/priority.go` readable from `main` in the bare. Prime worktree `<hub>/warp` is at `2410e42`, clean. `lyx fabric pairs`: both pairs `in_sync`, `junction_healthy`; `lyx fabric status` clean in both worktrees. Survivors after done: the tmux server `lyx-warp-LYXHUB-59652fb8` (session `task-priority` with the `lyx loom status --watch` strand) and `lyx reed watchdog`; the detached `lyx loom run` exited. `.lyx/shed/self/` holds only `run.lock`/`status.json.lock` (lock files, not held). Driver log: three WARN lines, none an error (plan gate informational findings twice; "focus directive names cluster excludes but the template profile has no cluster fan; dropping" once).

Condition evidence, go run: (1) packages touched: `task`, `store`, `cmd/tasktool`, 8 files, tests in every package; (2) batches: `_lyx/webster/outcome.yaml` `batches_done: 6`, `reports/01..06-*.yaml` each `status: OK`, six card commits; (3) bounce: **none** — the first judge verdict was APPROVED in all three segments (Discussion, Plan, Webster); each segment's shape was seed → Burler round 1 → judge APPROVED. Coverage gap recorded; the llm run uses a harder brief (below).

### Log — hub2, the llm-driver run (appended live)

Brief strengthened before start (board upsert on hub2/warp): the same priority task plus crash-safe `Store.Save` (temp file + atomic replace, tests for no leftover temp and for old content surviving a failed replace), a distinct corrupt-file error mapped to exit 1, `list --priority` unknown-level exit 2, and `done` on an already-done task exiting 1 — a brief with real design traps, no reviewer/verdict tampering.

Sequence: `cd $HUB/task-priority && $L shed seed self --recipe loom --driver llm --param parent=main` — the `shed seed --help` example verbatim (F9) → `{"driver":"llm","ok":true,...}`; `$L loom start --no-attach` → exit 0, again no output; `lyx loom status` = running at Preflight, history 0; processes: tmux `lyx-warp-LYXHUB-cba984e8`, `lyx reed watchdog`, and a `claude` strand whose argv starts `Run the ly-drive skill (from loomyard's ly plugin) for run-id "self"...`. `loom start` returned only once the driver TUI was up (~40 s).

Driver transcript (`~/.claude/projects/-home-knatte-crucible-happy-path-fable-high-r2-hub2-warp-LYXHUB-task-priority/3811ac43-....jsonl`), first turns:
1. `Skill {"skill":"ly-drive","args":"self"}` → "Launching skill: ly-drive" — the project-local stand-in loaded through the Skill tool; no Glob/Grep/find over the filesystem for a SKILL.md anywhere in the transcript (efc1f7053/d4f6a83d8 sound).
2. `D=$(mktemp -d)` (`/tmp/tmp.iSzqIsYDiy`) + `lyx shed status self` baseline → `("", 0)`-shaped (`current_producer: Preflight`, `history_length: 0`).
3. Step 1 launched in the background with a fresh `LYX_TRACE_ID`, stdout to `$D/step-1.json` (c4f0b1a39/3d59b14c6 sound: nothing under the drive directory; Preflight passed clean).
4. Then a deviation from the skill's loop: it wrote `$D/loop.sh` — `for n in 2..200: LYX_TRACE_ID=fresh lyx shed step self > $D/step-$n.json; break unless "continue":true` — and ran the whole loop as one background job. Every step still gets its own trace id and envelope file, and an error envelope (no `continue:true`) stops the loop for the session to inspect, but the skill's "after every step, read its trace_file right away" is not followed while the loop runs.
5. Waiting: first `sleep 100; cat ...` (blocked by the harness's own sleep rule), then a `Monitor` with `until ! pgrep -f "tmp.iSzqIsYDiy/loop.sh" ...; do sleep 5; done` — a self-matching `pgrep -f` pattern of a different spelling than the one 655bcb6be forbids by example. The session also relies on the harness's own background-job completion notice, which is what the skill names first, so the run is not stalled by it; judged half-effective (the rule's example is too narrow) rather than broken.

hub2 timeline: 17:52:28 Preflight → Loom-Preflight → Discussion-Write → 17:55:20 Discussion-Bouncer seed → 17:56:18 Discussion-Burler round 1 (review APPROVED, LOW/NIT F1–F7 fixed) → 17:59:40 judge APPROVED → 18:00:07 Plan-Write. No bounce in the discussion segment.
- 18:00:07 Plan-Write → 18:03:35 Plan-Bouncer seed → 18:04:20 Plan-Burler round 1 (review APPROVED, F1–F3 fixed) → 18:07:27 judge APPROVED → Batchifier → 18:08:12 Webster (9 identity batches: 01-priority-type … 09-list-priority; `## verify:` again two lines `go vet ./...` / `go test ./...`) → 18:12:18 Webster done: `outcome.yaml` `batches_done: 9`, commits `ecd731f..d0c49ce`, 8 files / +589 −25, `go vet && go test` pass at HEAD; `.lyx/webster/prompts/integration.md:19` again carries `go vet ./...` only — P2 CONFIRMED on both runs. → 18:13:06 Webster-Burler round 1.
- 18:13:06 Webster-Burler round 1 (review APPROVED, LOW/NIT F1–F5, five fix commits `3baaed3..f89db9a`) → 18:15:39 judge APPROVED → Publish → Finalize → Friction-Reflect, all done by 18:16:24; `state: done`, `history_length: 18`.

**llm run landed: YES.** `git -C hub2/warp.git log main` = `6fb0ac7 Persisted task priority for tasktool` over `56a263f`; `store/store.go` on `main` carries the `os.CreateTemp` + rename save. Prime `<hub>/warp` at `6fb0ac7`, clean; `fabric pairs` both in sync/healthy; `fabric status` clean in both worktrees. Survivors: tmux `lyx-warp-LYXHUB-cba984e8`, `lyx reed watchdog`, and the ly-drive `claude` strand idle in its pane after its final turn. Trace WARNs: the same plan-gate informational lines and the "focus directive names cluster excludes … dropping" line (twice).

Driver end-of-run (transcript tail): the loop ended at step 18 on `continue:false`/`state:done`; the session read the loop output, confirmed no error envelope and an empty `repairs/`, wrote `<scratch_dir>/drive-report-20260926-175217-3d1c.md` (stop condition, 18 steps, no repairs, no friction notes), and filed no `lyx selfreport` (0 selfreport tool calls) — autonomous-mode rules held.

Condition evidence, llm run: (1) packages `task`, `store`, `cmd/tasktool`, 8 files, tests in every package; (2) `batches_done: 9`, nine card commits; (3) bounce: **none** — first judge verdict APPROVED in all three segments again.

Coverage gap: neither run bounced. With loom's segment shape (Bouncer seed → one Burler round that reviews AND fixes → Bouncer judge), a bounce needs the judge to find the fixer's own output still blocking; both runs' fixers cleared every LOW/NIT finding in-round and the judges approved. Re-driving once (hub3, go driver) with a brief carrying harder traps: cross-process file locking around the read-modify-write with a real concurrent test, plus a journal-backed `undo`.
