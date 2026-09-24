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
