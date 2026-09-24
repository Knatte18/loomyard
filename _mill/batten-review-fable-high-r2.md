# batten — independent review, round fable-high-r2

Reviewer tag: `fable-high-r2`. Worktree `/home/knatte/Code/loomyard/wts/crucible-batten-followup`, branch `crucible-batten-followup`, HEAD at review start `647220980`.
Clean-room: written before opening any `batten-review-*` file other than the prompt, and before reading any predecessor commit body.

## Executive summary

(filled at the end of Job 1)

## Scope assessment

(filled at the end of Job 1)

## Code findings (severity-ranked)

(provisional entries appended as spotted; ordered at the end of Job 1)

## Focus-1 table: what a loom child writes under `_lyx`/`.lyx`, and who commits it

(filled during Job 1)

## Focus-2 table: crash-window re-entry of every batten row

(filled during Job 1)

## Docs and operability findings

(filled during Job 1)

## What was tested

Hermetic baseline at HEAD `647220980`:

- `go build ./... && go vet ./... && go test -count=1 ./...` — exit 0, 92 packages `ok`.

## Teardown

(filled at the end)

### Environment and fixture

- `which claude tmux gh` → all present (`claude` 2.1.281). `~/.claude/plugins/installed_plugins.json` has no `ly@loomyard` → **focus 5 (`--child-driver llm` drive) is an environment gap: the `ly-drive` skill is not installed; no time spent, no plugin installed.**
- `GOPROXY=https://proxy.golang.org,direct` (not `direct`), so fixture builds can fetch.
- Third issue-filing path grep (`CreateIssue`, `selfreport create`, `githubclient` issue callers): the only Go callers are Tier 1 (`internal/loomcli/arm.go` → `selfreportengine.CreateIssue`) and the `lyx selfreport create` CLI; Tier 2 is the friction reflection stencil. A third, non-Go path exists in `plugins/ly/skills/ly-drive/SKILL.md` (§ Self-report): the ly-drive driver may call `lyx selfreport create` once per supervised run, explicitly operator-approval-gated (autonomous mode turns it into a stop-report line). It is neither automatic nor reachable on this host (no `ly` plugin), so it is recorded here rather than as BLOCKING.
- Deployed the dev binary: `./deploy-dev` → `.dev-bin/lyx @ 647220980`.
- Fixture hub (disposable, in the session scratchpad, never used before): bare warp `fx/remotes/fxapp.git` (go.mod + main.go + .gitignore), empty bare weft `fx/remotes/fxapp-weft.git`; `lyx fabric clone --into fx/hub file://…fxapp-weft.git file://…fxapp.git` → `fx/hub/fxapp-LYXHUB/{fxapp,fxapp-weft,_board}`.
- BEFORE any Board task or create: committed and pushed onto prime's weft (`main-weft`) `loom.yaml` `selfreport: false` + `friction: ""`, `landing.yaml` `require_pr_to_base: []`, all verified by grep; plus `discussion/plan/review/conflict/recovery: sonnet` to keep the loom drive cheap.
- Board tasks: `fx-typed` (`type: loom`) and `fx-untyped` (no `type`).

### fx-untyped — Worktree-Create and Seed-Child (type empty → loom default)

- `lyx batten step fx-untyped` #1 → `producer: Worktree-Create, outcome: done, next: Seed-Child`. Prime's `_lyx/shed/fx-untyped/{seed.json,status.json}` written; seed = `{recipe: batten, driver: go, params: {child_driver: go}}`; prime weft commit `batten: Seed-Child -> running` carries both files. Pair `fx-untyped` + `fx-untyped-weft` on disk.
- `lyx batten step fx-untyped` #2 → `producer: Seed-Child, outcome: done, next: Run-Shed`. Child seed `_lyx/shed/self/seed.json` = `{recipe: loom, driver: go, params: {parent: main}}` — the empty Board type resolved to `loom`, driver inherited from prime's `child_driver`. Committed on the child's weft (`batten: seed child fx-untyped`, on top of fabric's `record parent branch`), both halves' porcelain empty, `origin/fx-untyped` and `origin/fx-untyped-weft` pushed. Child's `loom.yaml` carries the `selfreport: false`/`friction: ""` override (forked from prime's weft after the override commit).
- Prime weft commit `batten: Run-Shed -> running` landed.

### fx-untyped — Run-Shed against a real child bootstrap whose loom Preflight blocks

- Dirtied the child warp with an untracked `dirt.txt`, then `lyx -v batten step fx-untyped`: logs `battenshed: spawning inner shed run status_found=false` → `battencli: spawning loom session` → `loom session wait complete` (0.43s wall). The real `lyx loom start --no-attach` seeded the child (`loom: seed session bootstrap for fx-untyped` on the child weft), spawned the detached `lyx loom run`, whose Preflight hit the dirt and persisted `blocked` (`loom: Preflight -> blocked` committed on the child weft). Run-Shed's post-spawn read saw `blocked` → hard error carrying `state/error/current_producer` + the resume-inside-the-child remedy; envelope `kind: producer`; prime status `failed` at `Run-Shed` and committed (`batten: Run-Shed -> failed`). Pair intact; no teardown. Marker `.lyx/shed/fx-untyped/Run-Shed-spawned` written.
- Substrate left behind by design: tmux session `fx-untyped` (its `loom status --watch` pane) and the per-hub watchdog; the `lyx loom run` driver exited.

### Batten Bookend refusal floor + `lyx shed seed` in every fabric checkout kind

- From the task worktree `fx-untyped`: `batten status|step|run|pause` all refuse naming both `fx-untyped` and `fxapp`.
- From the weft prime `fxapp-weft`, from `_board`, and from the task pair's weft half `fx-untyped-weft`: every verb refuses with `RequireDrivableWorktree`'s own text (weft sibling / `_board` checkout).
- `lyx shed seed zz1 --recipe loom` from `_board`, from `fxapp-weft`, and from `fx-untyped-weft`: all refuse; no `_lyx/shed/zz1` left in any of the three.
- `lyx shed status fx-untyped` from prime arms batten through `ArmAt` and reports the same failed status.
- **Observation (recorded as finding F-A below):** `lyx shed seed zz2 --recipe batten` from the TASK worktree `fx-untyped` is accepted and writes `_lyx/shed/zz2/seed.json` into the task pair's weft, although every batten verb refuses there (`lyx shed status zz2` → non-prime refusal). The seed is uncommitted dirt no verb can ever drive.

### PrimeRunLock scope across two slugs

- Held `fxapp/.lyx/shed/run.lock` by hand (`flock -x … -c 'sleep 40'`), then `lyx batten step fx-typed` (first row): `Worktree-Create` → `stuck` → `state: blocked`, `stuck_reason` names that lock path; no `fx-typed` pair created. The lock is hub-wide (`.lyx/shed/run.lock`, no slug segment), so a second slug's create/teardown is serialised against any other slug's. Consequence worth noting: contention is a `blocked` halt (no `on_stuck` on the bookend rows), not a wait — the operator re-steps. Each re-step commits `Worktree-Create -> running` then `-> blocked` on prime's weft (two commits per attempt).
- Released the hold; `lyx batten step fx-typed` resumed silently → create done → `Seed-Child` done; child seed `{loom, go, parent: main}` committed on `fx-typed-weft`.

### fx-typed — the full `--child-driver go` drive (background step loop)

- `~/.claude.json` `.projects` had no entry for the child path before launch (946 entries).
- Launched `drive-fx-typed.sh`: `lyx -v batten step fx-typed` in a loop, stopping when `next != Run-Shed`, i.e. right before `Worktree-Teardown`. First step: `spawning inner shed run status_found=false`, bootstrap returned in 0.23s, child at `Discussion-Write/running`, a `claude … --model sonnet` provider alive in the child's tmux session.

### fx-untyped — Run-Shed re-entry with status `running` and no spawn marker (emulated kill-before-marker)

- Rewound the child's committed status to `Preflight/running`, removed `.lyx/shed/fx-untyped/Run-Shed-spawned`, resumed with `lyx -v batten step fx-untyped`: `spawning inner shed run status_found=true` → real `lyx loom start --no-attach` → a fresh detached driver ran Preflight, blocked again on the dirt (`loom: Preflight -> blocked` committed a second time on the child weft) → Run-Shed hard error, marker rewritten. The re-spawn-on-running-without-marker path is live and idempotent against the real bootstrap.

### fx-untyped — Worktree-Teardown re-entered against a half-torn pair (CONFIRMED finding F-B)

- Removed the dirt, hand-wrote the child's status `done` and committed it; `lyx batten step fx-untyped` → `Run-Shed` `done`, `next: Worktree-Teardown`.
- Emulated the crash window between fabric's warp-worktree removal and its weft teardown (the exact order `Topology.Remove` runs: portal → launchers → gates → junctions → warp dir → weft): `git -C fxapp worktree remove fx-untyped`, leaving `fx-untyped-weft`, both branches (local + remote), `_portals/fx-untyped`, `_launchers/fx-untyped` and the tmux session `fx-untyped` behind.
- `lyx -v batten step fx-untyped` → `session shutdown skipped, the task worktree is already gone` → `teardown worktree skipped, the task worktree is already gone` → `outcome: done, state: done`. The run is `done` with `fx-untyped-weft` still on disk, both branches present, the portal/launcher entries present, and the tmux session `fx-untyped` still alive (Shutdown was skipped because reed's config is resolved through the absent warp). `lyx fabric pairs` still lists the half pair; `lyx fabric prune` (dry run) is the fabric verb that exists for exactly this orphan-weft debris.
- The same state is reachable WITHOUT a crash: fabric's own `Remove` has an error path "warp worktree removed, but weft teardown failed and the weft worktree remains" — the row goes Stuck on it (correct), and the operator's re-step then reports `done` over the leftover.

### fx-typed — pause during the watch

- `lyx batten pause fx-typed` mid-`Run-Shed` → ok; the step loop will show whether the flag is consumed at the next producer boundary.
- Mid-campaign porcelain sample (child at `Plan-Bouncer`): child warp clean, child weft clean, prime weft `M _lyx/shed/fx-typed/status.json` (the documented uncommitted self-bounce history). Child weft log shows every loom transition and both artifact commits (`discussion artifacts`, `plan artifacts`) landing as Go commits; `.lyx/{burler,logs,loom,reed.json,shed,shuttle}` all ephemeral.

### fx-typed — success terminal state reached (`Worktree-Teardown` → `done`, no operator step)

- Child campaign (go driver, sonnet agents) ran `Discussion-Write → Discussion-Bouncer → Discussion-Burler → Discussion-Bouncer → Plan-Write → Plan-Bouncer → Plan-Burler → Plan-Bouncer → Batchifier → Webster → Webster-Bouncer → Webster-Burler → Webster-Bouncer → Publish → Finalize/done` in ~4 min. Prime warp `main` received the squash `Print \`fxapp v1\`` (`main.go` prints `fxapp v1`), prime warp porcelain clean.
- `lyx batten pause fx-typed` mid-watch was consumed at the next boundary (one `state: paused` envelope in the loop log, prime weft commit `batten: Run-Shed -> paused`); a second pause I issued during the busy experiment was consumed by the first manual step after the child finished (`state: paused`, no producer ran), and the following step ran Run-Shed → `done`, `next: Worktree-Teardown`.
- **Focus-1 live proof, right before teardown:** child warp `git status --porcelain` empty (`--ignored` shows only the `_lyx`/`.lyx` junctions and the fixture's gitignored `fxapp` binary built by the implementer), child weft porcelain empty (`--ignored`: `.lyx/`, `.weft/`), every `_lyx` artifact tracked: `discussion/*`, `plan/*`, `fabric/origin.json`, `shed/self/{seed,status}.json`, `webster/{outcome.yaml,state.json,summary.md,reports/*}` (commits: `loom: discussion artifacts`, `loom: plan artifacts`, `webster: begin-batch`/`record-batch`, `loom: webster run record`, one `loom: <row> -> <state>` per transition). Prime weft: `M _lyx/shed/fx-typed/status.json` only (the documented self-bounce history).
- `lyx -v batten step fx-typed` (teardown): `reed: down complete session=fx-typed` (reed also tore down the now-empty per-hub tmux server), `battencli: teardown worktree` mutations recorded, envelope `producer: Worktree-Teardown, outcome: done, state: done`. Hub afterwards: `fx-typed`/`fx-typed-weft` gone, `_portals/fx-typed` + `_launchers/fx-typed` gone, no tmux server, `abandonedSession` absent, prime weft `batten: Worktree-Teardown -> done` committed and clean. Left by fabric's design: local warp branch `fx-typed`, remote `origin/fx-typed` + `origin/fx-typed-weft` (`Remove(…, remote=false)`); local `fx-typed-weft` deleted.
- Substrate after teardown: only the per-hub `lyx reed watchdog` (pid 320077) remains; no `loom run`, no `claude` for the fixture.
- `~/.claude.json`: one new `projects` entry for the child path appeared (946 → 947), `hasTrustDialogAccepted: null` — the agents ran under `--dangerously-skip-permissions`, no trust dialog was involved for the go-driven child. File left alone.
- Hermetic gates: `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./internal/shedrun/... ./cmd/lyx/...` exit 0; `go test -tags integration -count=1 ./internal/battencli/...` exit 0.
