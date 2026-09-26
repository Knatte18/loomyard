# happy-path — independent review, round 2 (tag `fable-high-r2`)

Worktree: `/home/knatte/Code/loomyard/wts/crucible-happy-path`, branch `crucible-happy-path`.
Dev binary: `.dev-bin/lyx` deployed from `4c7fea5db18987ad228d93852bd9172cadd569fd` (`happy-path: crucible re-seed r2`); `git log -1` at first deploy = that commit.
Production baseline: `/home/knatte/go/bin/lyx`.
Scratch root: `$HOME/crucible-happy-path/fable-high-r2/` (`hub1` go run, `hub2` llm run, `hub3` go re-drive with a harder brief).

## Executive summary

**Landed: go driver YES (hub1 and hub3), llm driver YES (hub2).**
Every run went Preflight → Discussion → Plan → Batchifier → Webster → Webster-Review → Publish → Finalize → Friction-Reflect to `state: done` with the change squash-merged onto the fixture warp's `main` in the bare repo, the prime worktree clean and at that commit, and `lyx fabric pairs`/`status` clean afterwards; the llm driver needed no repair and no escalation.
No blocker was found.
Top finding (MEDIUM): a multi-line plan-level `## verify:` section is silently truncated to its first line, so the integration gate ran `go vet` only and never the plan's `go test` on both hub1 and hub2 — the run lands, but its last mechanical gate reports OK without having run the tests the plan asked for.
Three LOW/NIT frictions: `lyx loom start --no-attach` prints no success envelope; the ly-drive wait rule is stated too narrowly (the live driver used another self-matching `pgrep -f` spelling); the ly-drive loop text lets a driver batch many steps into one background job and skip the per-step trace read.
Coverage gap: **no review segment bounced in any of the three runs** (every first judge verdict was APPROVED, in nine segments); see "Condition 3".
All nine prior-round fixes are sound; two are only half-exercised (details in the table).

## 1a — Regression table (prior-round fix commits)

| sha | fix | verdict | evidence |
|---|---|---|---|
| `242d46983` | clone names the weft primary and `_board` after the warp prime's branch | sound | Diff: `checkedOutBranch(warpWorktreePath, "warp prime")` runs after step 5's `refuseUncheckedOutWarpClone`, so a warp with no branch is refused earlier with its own message; the adopt path still keys on `origin/<branch>-weft`. Live (all three hubs): weft bare created by `git init --bare` on this host has HEAD `refs/heads/master`; clone produced `main-weft` (born, two commits) and `_board` on `main`; `fabric add` forked pairs from it. |
| `c4f0b1a39`, `3d59b14c6` | ly-drive writes envelopes to a private `mktemp -d` dir | sound | Diff: recipe-blind wording (`TestLyDriveSkill_IsRecipeBlind` passes). Live (hub2 transcript): `D=$(mktemp -d)` → `/tmp/tmp.iSzqIsYDiy`, every `step-N.json` under it; Preflight's clean-tree gate passed on the first step and `fabric status` stayed clean. |
| `5550dd00d` | Finalize pushes the parent branch after the parent-side merge | sound | Diff: `PushBranch` → `PushWarpRebaseFreeAt` → `gitrepo.PushRebaseFree` = `git -c push.autoSetupRemote=true push`, so a parent branch without an upstream gets one instead of failing; `SkipPush` honoured; a failed push is Stuck with "push it by hand", never a false Done. Live: all three bare warps' `main` carry the squash commit right after Finalize (`2410e42`, `6fb0ac7`, `7a775c8`). Beyond the happy path: a parent with no remote at all fails `git push` → Stuck with the local merge intact, the correct shape. |
| `efc1f7053`, `d4f6a83d8` | help/launch prompt name the `ly` plugin; a missing skill stops the session | sound | Diff: prompt under the launch-prompt byte cap (its test). Live (hub2): argv of the driver strand carries the new sentence; the transcript's first tool call is `Skill {"skill":"ly-drive"}` against the project-local stand-in and there is no Glob/Grep/find for a SKILL.md anywhere. The "stop if missing" branch was not exercised (the stand-in was present). |
| `f51cb430f` | yamlengine sets and reconciles config lists whole | sound | Diff: the only reconciled template list is `landing.require_pr_to_base` (`burler` fans and `models` are `SeedOnly` in `internal/configreg`), so whole-list carry drops nothing a normal upgrade relies on; `collectSequencePaths` does not descend into sequences, matching `sequenceBasePath`'s element grammar. Live: `--set 'require_pr_to_base=[]'` wrote `[]`; an unrelated `--set squash=true` kept `[]`; `config reconcile` dry-run reported no added/removed keys for any module; Publish took the no-pull-request branch on all three runs. |
| `761dc64a5` | `lyx shed seed --help`'s loom example matches the bootstrap's writes | sound | Live (hub2): `lyx shed seed self --recipe loom --driver llm --param parent=main` verbatim, then `lyx loom start --no-attach` accepted the seed and spawned the llm driver; `seed.json` = `{recipe: loom, driver: llm, params: {parent: main}}`. The go form did the same on hub1/hub3. |
| `ff6e654ba` | ly-drive resumes a `blocked`/`paused`/`failed` baseline | sound, not exercised live | The skill text distinguishes the *baseline* status read from a step envelope's `continue:false`, and `internal/shedengine/run.go:110` confirms a Step on a blocked/failed run re-calls the current producer, so "proceed to the first step, which resumes the run" is true. No run in this round halted, so no driver ever met that baseline. |
| `655bcb6be` | ly-drive names how to wait for a backgrounded step | half-effective | The rule forbids by example only `pgrep -f 'lyx shed step'`; the hub2 driver waited with `until ! pgrep -f "tmp.iSzqIsYDiy/loop.sh"` (another self-matching pattern) inside a 1-second placeholder Monitor and otherwise leaned on the harness's own completion notice, which the sentence names first — the run was never stalled by it, but the rule as written does not stop the pattern it targets. Finding F3. |

## 1b — Fixture and operator sequence (the deliverable the operator repeats)

Fixture project `example.com/tasktool` (three packages that depend on each other, each with tests): `task` (domain), `store` (JSON-file persistence, imports `task`), `cmd/tasktool` (CLI, imports `store`); committed on `main`, pushed into a `git init --bare` warp whose HEAD is re-pointed to `refs/heads/main`; the weft is an empty `git init --bare` (HEAD `master` on this host).

With `L=<worktree>/.dev-bin/lyx`, `ROOT=<scratch>`, `HUB=$ROOT/warp-LYXHUB`:
1. `cd $ROOT && $L fabric clone --into $ROOT $ROOT/weft.git $ROOT/warp.git`
2. `cd $HUB/warp && $L config loom --set selfreport=false --set 'friction='` and `$L config landing --set 'require_pr_to_base=[]'` (both commit and sync onto `main-weft`; verify with `cat $HUB/<slug>/_lyx/config/loom.yaml` after step 3).
3. `cd $HUB/warp && $L board upsert '{"slug":"<slug>","title":...,"brief":...,"body":"<the brief>"}'` then `$L fabric add <slug>`.
4. `cd $HUB/<slug> && $L shed seed self --recipe loom --driver go|llm --param parent=main` (the go seed is optional; `loom start` writes it), then `$L loom start --no-attach` (returns exit 0 with no output — F2; confirm with `$L loom status`).
5. Follow with `$L loom status` (or `--watch`), `$L shed status`, and `<slug>/.lyx/loom/driver.log` (go driver) / the ly-drive pane (llm driver).
6. After `state: done`: `git -C $ROOT/warp.git log main`, `git -C $HUB/warp status`, `$L fabric pairs`, `$L fabric status` in both worktrees.

llm stand-in for the uninstalled `ly` plugin: copy `plugins/ly/skills/ly-drive/SKILL.md` to `$HUB/<slug>/.claude/skills/ly-drive/SKILL.md` and append `.claude/` to `$HUB/warp/.git/info/exclude` before `loom start`.

Every step above worked as documented; no step of round 1's sequence regressed.

## Evidence for the task's three conditions

| | hub1 (go) | hub2 (llm) | hub3 (go, harder brief) |
|---|---|---|---|
| Task | `task-priority`: persisted `Priority` (domain enum, legacy-JSON default, `--priority` on `add`/`list`, ordering) | same brief plus crash-safe atomic `Save`, corrupt-file error → exit 1, `done` on done → exit 1 | `safe-store`: cross-process `flock` around read-modify-write with a 20-goroutine concurrent test, journal-backed `undo` |
| (1) Files/packages | 8 files, `task`/`store`/`cmd/tasktool`, +578/−23, tests in each | 8 files, same three packages, +589/−25, tests in each | 18 files, same three packages (11 new), tests in each |
| (2) Batches | `_lyx/webster/outcome.yaml` `batches_done: 6`; six card commits `6ba5b50..a666100`; `reports/01..06-*.yaml` OK | `batches_done: 9`; nine card commits `ecd731f..d0c49ce` | `batches_done: 8`; eight card commits `60b7a74..6517414` |
| (3) Bounce | none: Discussion, Plan and Webster segments each seed → Burler round 1 (review APPROVED, LOW/NIT only, fixed in-round) → judge `round-1-bouncer-verdict.md` APPROVED | none, same shape in all three segments | none, same shape in all three segments |
| Landed | `hub1/warp.git` `main` = `2410e42` | `hub2/warp.git` `main` = `6fb0ac7` | `hub3/warp.git` `main` = `7a775c8` |

What a bounce concretely is (from `contracts/recipes/loom-recipe.yaml` and `internal/shedadapters/bouncer.go`): the Bouncer's first call is a *seed* pass (writes `round-1-focus.md`, returns Stuck → Burler); the Burler row reviews and fixes in one round (`round-N-review.md` with its own `verdict:`, `round-N-fixer-report.md`) and always returns Stuck → Bouncer; the Bouncer then *judges* round N (`round-N-bouncer-verdict.md` `APPROVED`/`BLOCKING` + ledger + `round-N+1-focus.md`); `BLOCKING` is the bounce (Stuck → Burler round N+1), `APPROVED` is Done.
A Bouncer → Burler → Bouncer pass is therefore the minimum path, not a bounce.
In all nine segments the first judge verdict was `APPROVED`.
The harder hub3 brief drew only LOW/NIT findings from the webster reviewer (five, all fixed in-round) and an APPROVED judge.
Plainly: this round produced **no evidence of a bounce under either driver**; with a review-and-fix round ahead of every judge pass, a bounce needs the fixer's own output to still be blocking, and no run's fixer left that behind.
The bounce path itself (Stuck → Burler round 2) is exercised by `internal/shedadapters`' hermetic tests, not by this round.

## Findings (severity-ranked)

### F1 — MEDIUM — a multi-line plan-level `## verify:` is silently truncated to its first line — CONFIRMED (hub1, hub2)

- `internal/planparser/sections.go:68`: `plan.Verify = firstNonEmptyLine(extractSection(body, planVerifyHeading))`, and `plan.go:85` documents `Verify` as "the single command line".
- `contracts/stencils/loom/loom-template-plan.md:140` tells the planner the section is "one or more runnable shell commands", and both hub1's and hub2's planners wrote two lines (`go vet ./...`, `go test ./...`).
- Run scenario: Webster's integration fork prompt (`.lyx/webster/prompts/integration.md:19`, rendered by `websterengine.RenderIntegrationPrompt` from `plan.Verify`) carried `go vet ./...` only; the fork ran only that and reported `status: OK`; webster's `summary.md` (and therefore the squash commit message on `main`) records "it did not run `go test ./...` because its prompt listed only `go vet`". The bisect path (`websterengine/integration.go` `runVerifyCommand(plan.Verify)`) uses the same truncated string. The plan's own gate reports success without running the plan's tests; a test regression across batches would land.
- Fix: carry every non-empty line of the section, joined with ` && `, so a later command's failure fails the verify; update `Plan.Verify`'s doc, `loom-plan-spec.md`'s verify sentence, and add a planparser test.

### F2 — LOW — `lyx loom start --no-attach` returns exit 0 with no output — CONFIRMED (all three hubs)

- `internal/loomcli/start.go:404`: `if !mustAttach(noAttachFlag) { return nil }` after the driver is confirmed up; nothing is written to `out`.
- Run scenario: the operator's start step prints nothing; a script cannot tell "driver up" from a silent failure without a follow-up `lyx loom status`. The CLI/Cobra Invariant exempts only the terminal-handover tail from JSON, and `--no-attach` deliberately skips that tail.
- Fix: emit `{"ok":true,"attached":false,"driver":"<go|llm>","slug":...,"status_file":...}` on the no-attach return; the attach path stays JSON-free as before.

### F3 — LOW — the ly-drive wait rule names one self-matching spelling and the live driver used another — CONFIRMED (hub2 transcript)

- `plugins/ly/skills/ly-drive/SKILL.md` "How to invoke a step": "Never wait by matching a command line (`pgrep -f 'lyx shed step'`)". The hub2 driver ran `until ! pgrep -f "tmp.iSzqIsYDiy/loop.sh" ...` — the same self-match, different text — as a placeholder monitor and then relied on the harness's completion notice.
- Run scenario: not stalled this time (the completion notice fired), but the rule's example is too narrow to prevent the wait the fix was written for; the round-1 driver idled ten minutes on exactly this.
- Fix: state the rule as a class, not an example: never wait by matching any text of a command line (`pgrep -f`, `ps | grep`) — the waiting shell's own command line carries the same text; wait on the job's own completion notice, or `wait <pid>` in the shell that launched it.

### F4 — NIT — the ly-drive loop text admits batching many steps into one background job — CONFIRMED (hub2 transcript)

- The hub2 driver wrote `loop.sh` running steps 2..200 in one background job, breaking only when an envelope lacks `continue:true`; the skill's "After every step, read its `trace_file` right away, because traces are swept by retention" was not followed while the loop ran (17 steps, ~24 minutes), and an error envelope mid-loop would have been read only after the loop stopped.
- Run scenario: no harm on this run (no error envelope); on a run that needs a repair, the trace the repair depends on may already be swept.
- Fix: one sentence in "How to invoke a step": launch one `lyx shed step` per background job and read its envelope and trace before launching the next; never a loop that runs several steps without the session reading each one.

## Out-of-scope observations (one line repro, one line impact each)

1. `record-batch` warns "fork never returned a final report" for batches whose report file did land (hub1 batches 03–06; webster's `summary.md`). Repro: any webster run where an implementer fork ends its turn on a tool call. Impact: noise in `summary.md` and, through Finalize, in the squash commit message on `main`.
2. `shedadapters: focus directive names cluster excludes but the template profile has no cluster fan; dropping` (WARN) on every Burler round of every run. Repro: any loom run. Impact: the judge writes an `exclude_lenses` list nobody consumes; WARN noise in traces.
3. Finalize's squash commit message is webster's whole `summary.md`, "Deviations"/"Integration" paragraphs included (`summary.CommitMessage()`). Repro: any landing. Impact: `main`'s history carries process narration.
4. The ly-drive `claude` strand stays alive, idle in its pane, after the run is `done` (hub2 pid 1965494 until teardown). Repro: llm run to done with `--no-attach`. Impact: a paid session stays open until the operator closes it; nothing but `status` tells an unattended operator the run finished.
5. Bounces are structurally rare under loom's segment shape (review+fix round before every judge pass) — a design observation, not a defect; recorded because the round's third condition depends on it.
6. Harness (not lyx): the driver session's `sleep 100` was blocked by Claude Code's own sleep rule and its first `Monitor` call failed schema validation; both self-corrected. Impact: two wasted turns per drive.

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

### Log — hub3, the go-driver re-drive with the harder brief (appended live)

Board task `safe-store` (cross-process `flock` around every mutation with a 20-goroutine concurrent `Add` test; journal-backed `undo`; `go test -race`). Same sequence as hub1 (`--driver go`), `loom start --no-attach` exit 0 with no output.
Timeline: 18:18 Preflight → Discussion-Write → 18:21:37 Discussion-Bouncer seed → 18:22:17 Burler round 1 (APPROVED, six LOW/NIT fixed) → 18:26:23 judge APPROVED → 18:27:04 Plan-Write (8 cards, `## verify:` = one line `go vet ./... && go test -race ./...`) → 18:29:50 Plan-Bouncer seed → 18:30:30 Plan-Burler round 1 (APPROVED, four findings fixed) → 18:35:17 judge APPROVED → 18:36:17 Webster → 18:40:48 done (`batches_done: 8`, commits `60b7a74..6517414`; `go vet && go test -race` pass at HEAD; integration prompt carries the full one-line verify) → Webster-Bouncer seed → 18:41:56 Webster-Burler round 1 (APPROVED, F1–F5 LOW/NIT, five fix commits `74e9248..ebb4206`) → 18:44:46 judge APPROVED → Publish → Finalize → Friction-Reflect → 18:45:35 `state: done`, `history_length: 18`.
**Landed: YES** — `hub3/warp.git` `main` = `7a775c8 Concurrency-safe store with undo` over `0e2f16c`, 18 files (`store/{atomic,journal,lock,undo}.go` and tests, `task/{entry,undo}.go` and tests, `cmd/tasktool/process_test.go`); prime clean; `fabric status` clean. No bounce in any segment.

### Survivors at the end of Job 1 (torn down before Job 2's re-drives)

Three tmux servers (`lyx-warp-LYXHUB-59652fb8`, `-cba984e8`, `-55ef1e68`), three `lyx reed watchdog` daemons, and hub2's idle ly-drive `claude` strand (pid 1965494), all identifiable by the scratch path in their args.
