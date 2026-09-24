# batten — independent review, round fable-high-r2

Reviewer tag: `fable-high-r2`. Worktree `/home/knatte/Code/loomyard/wts/crucible-batten-followup`, branch `crucible-batten-followup`, HEAD at review start `647220980`.
Clean-room: the findings below were written before opening any `batten-review-*` file other than the prompt, and before reading any predecessor commit body.
Not touched: Windows path behaviour (Linux host).

## Executive summary

Batten's own four packages held up under a real end-to-end drive: one `--child-driver go` run went from `Worktree-Create` through a full loom campaign (discussion, plan, webster, publish, finalize) to `Worktree-Teardown → done` with no operator step, both halves of the child pair clean at the moment of teardown, and every refusal at the Bookend floor firing where it should.
The defects this round found are again at batten's boundary with fabric, and two of them are the same shape as the previous round's F3 — a remedy that is right for a task-worktree caller but wrong from prime, and a "post-condition satisfied" probe that answers for the warp alone:

- **F1 (MEDIUM, CONFIRMED)** — `Worktree-Teardown` re-entered against a half-torn pair (warp gone, weft still on disk) reports `done` and leaves the weft worktree, both branches, the portal/launcher entries and the tmux session behind. Reachable by a kill between fabric's warp-dir removal and its weft teardown, and, without any crash, by fabric's own "warp worktree removed, but weft teardown failed" error followed by the operator's re-step.
- **F2 (MEDIUM, CONFIRMED)** — `Worktree-Create` passes fabric's branch-exists remedy verbatim into `stuck_reason`; followed from prime — the only place a batten verb runs — `lyx fabric checkout <slug>` switches prime's own pair onto the task branches. Reachable after every re-run of a torn-down slug (fabric leaves the warp branch behind by design) and after the known rollback item.
- **F3 (LOW, CONFIRMED)** — `lyx shed seed --recipe batten` is accepted from a task worktree, writing a seed no verb can ever drive there.
- **F4 (NIT, CONFIRMED)** — the done-slug refusal's remedy ("delete its run directory to run it again") is incomplete: the re-run refuses on the leftover branches, and deleting only the local one fails at push against the remote copy.
- **F5 (LOW, PLAUSIBLE — traced, not driven)** — batten reads the child's `done` before loom's driver has finished its post-run friction reflection (Tier 2), so with `friction` enabled the teardown row kills the reflection agent mid-flight. A design tradeoff the shipped residual half-covers; recorded for an operator decision, not fixed here.

Merge-readiness opinion: mergeable for the normal single-instance flow once F1 and F2 are fixed (both are scoped closures in `internal/battencli/wire.go` plus one fabric sentinel); F3/F4 are small. The success terminal state is proven live.

## Scope assessment

Design-promised (recovered `manifest/designs/seeded-shed.md`) vs shipped:

| Promise | Shipped | Note |
|---|---|---|
| Four rows Worktree-Create / Seed-Child / Run-Shed / Worktree-Teardown | yes (`contracts/recipes/batten-recipe.yaml`, names pinned) | proven live |
| Seed-Child copies recipe from the Board `type`, driver from prime's `child_driver` | yes; empty type → `loom` | proven live on `fx-untyped` |
| Run-Shed spawns the child's bootstrap and watches the child's status, self-routed Stuck, 1440×30s | yes | proven live incl. pause at a boundary |
| Halted child is a hard error, never a route to teardown | yes | proven live (`blocked` child) |
| Durable `_lyx/shed/<slug>/` run dir in prime, committed per transition, self-bounce skipped | yes | proven live |
| Batten `go`-only; `--driver llm` refused by name | yes | proven live |
| `driver: llm` for the child via loom's bootstrap | wired; not runnable on this host (no `ly` plugin) | environment gap |
| Two residuals (dead driver undetected; finished driver's strand left until teardown) | documented in `battenshed/doc.go` | still accurate; F5 is the flip side of the second |
| Relay-stepping | rejected, absent | correct |

No silently-dropped requirement found. Over-reach: none.

## Code findings (severity-ranked)

### F1 — MEDIUM — `Worktree-Teardown` reports `done` over a half-torn pair (CONFIRMED live)

`internal/battencli/wire.go:180-231` (`Teardown.Shutdown` / `Teardown.Remove` closures) and `taskWorktreePresent` (`wire.go:86`).
Both halves treat "warp worktree path absent" as "the pair is gone" and skip. `fabricengine.Topology.Remove` (`remove.go:57-165`) runs portal → launchers → gates → junctions → **warp dir** → **weft worktree + branches**; a kill in the last gap, or its own error return "warp worktree removed, but weft teardown failed and the weft worktree remains", leaves the weft worktree, `_portals/<slug>`, `_launchers/<slug>`, both branches and the reed session behind with the warp gone.
Scenario driven: `git worktree remove fx-untyped` (warp only) then `lyx batten step fx-untyped` → `session shutdown skipped` → `teardown worktree skipped` → `state: done`, with `fx-untyped-weft`, both branches, portal/launcher entries and the tmux session still present. `lyx fabric pairs` keeps listing the half pair; `lyx fabric prune` is the fabric verb for exactly this debris.
Suggested fix: the Remove closure's "already gone" answer must be about the pair: when the warp is absent but the pair's sibling worktree is still on disk, return Stuck with a reason naming the leftover and `lyx fabric prune --apply` as the remedy (re-step then reports done), through a vocabulary-neutral fabric accessor for the sibling's presence. Update `docs/overview.md`'s teardown-idempotence sentence. Integration test: half-tear a pair, step teardown, expect `blocked` with the remedy, then prune and expect `done`.

### F2 — MEDIUM — `Worktree-Create`'s verbatim fabric remedy switches prime when followed (CONFIRMED live)

`internal/battencli/wire.go:158-170` (`CreateWorktree` closure) passes `Topology.Add`'s refusal through unchanged (by design, `create.go` doc). `fabricengine/add.go:73-75` words it for a task-worktree caller: `switch a pair onto it with "lyx fabric checkout <slug>", or delete it first with "git branch -D <slug>"`.
Every batten verb runs from prime, and `lyx fabric checkout` has no prime guard: driven live, `lyx fabric checkout fx-typed` from prime returned ok with `worktree_switched fxapp → fx-typed`, `worktree_switched fxapp-weft → fx-typed-weft`, `branch_created fx-typed-weft` — prime became the task pair and batten's own `_lyx/shed/` view changed underneath it (restored with `lyx fabric checkout main`).
The other arm is also incomplete: after `git branch -D fx-typed` alone, create proceeds to the push, which the remote's leftover branch rejects (non-fast-forward); fabric rolls back and its destructive gate refuses to delete the new warp branch (the known rollback item), so the next step is back at "already exists". Only deleting the local branch AND both remote branches lets the slug run again.
This is reachable in the everyday flow: every successful teardown leaves the local warp branch and both remote branches (`Remove(…, remote=false)`), so any re-run of a slug lands here; so does the known failed-create rollback.
Suggested fix: fabric wraps its re-add refusal in an exported sentinel; batten's create closure recognises it and reports its own remedy — delete the leftover branch locally and on the remote (both sides of the pair), then re-step; never a `lyx fabric checkout` from prime — the same shape `taskWorktreeLocation`'s absent-worktree text already takes. Unit test on the closure's classification; F22 line for the live behaviour.

### F3 — LOW — `lyx shed seed --recipe batten` accepted from a task worktree (CONFIRMED live)

`internal/shedcli/seed.go:62-85` (`writeSeed`) refuses fabric's own checkouts but nothing recipe-specific. From `fx-untyped` (a task warp): `lyx shed seed zz2 --recipe batten` → ok, `_lyx/shed/zz2/seed.json` written into the task pair's weft; `lyx shed status zz2` there → batten's non-prime refusal. The seed is uncommitted dirt no verb can drive (it would block that pair's teardown until removed), and the batten module's own rule (prime only) is not consulted at the one site that writes batten seeds by hand.
Suggested fix: the shedcli table entry carries a per-recipe seed-location refusal (`battencli` exports the prime-only check it already applies in `armAt`; loom's entry leaves it nil), and `writeSeed` calls it after the drivable-worktree check.

### F4 — NIT — the done-slug remedy is incomplete (CONFIRMED live)

`internal/battencli/arm.go:341-344` and `:397-400` (`battenPreRun`/`battenPreStep`): "delete its run directory … (a change on the pair's fabric sibling) to run it again". Followed verbatim, the re-run refuses at `Worktree-Create` on the leftover branches (F2). The absent-worktree text in `wire.go` already spells the complete abandon path ("run directory … and the pair's branches, local and remote"); the done remedy should say the same, from one shared constant instead of two copies.

### F5 — LOW — teardown can kill loom's post-run friction reflection (PLAUSIBLE, traced; NOT-FIXED-THIS-ROUND)

`internal/loomcli/run.go` `reflectFriction` runs after `shed.Run` has returned — i.e. after `Finalize -> done` is persisted and committed and the run lock is released. `Run-Shed` reads that `done` within one poll and routes to `Worktree-Teardown`, whose `reedEngine.Down()` ends the child's session — the reflection agent's pane included — and whose `Remove` deletes `.lyx/loom/friction/` with the worktree. With the shipped default `friction: opus[effort=high]` the operator gets an `abandonedSession` and no Tier 2 report. Not driven here (Tier 2 files real issues; the fixture keeps it off). Whether batten should wait for the driver's post-run bookkeeping contradicts the shipped residual "the row watches the child's persisted status file, never the driver's own liveness" — an operator decision on the design, so recorded rather than fixed.

## Focus-1 table: what a loom child writes under `_lyx` / `.lyx`, and who commits it

Enumeration: `grep -rn 'lyxdirs.LyxDirName\|lyxdirs.DotLyxDirName' internal --include=*.go | grep -v _test.go` for every path accessor, `grep -n 'CommitAnchoredPaths\|fabricSync\|f.Commit(' internal/loomcli internal/loomshed internal/webstercli internal/landingshed` for the commit seams, `contracts/recipes/loom-recipe.yaml` for the rows, and the live listing `git -C fx-typed-weft ls-files` + `find .lyx` after the campaign. What this cannot see: files an agent writes outside the paths its stencil names (the stencils forbid it; the teardown gate catches it), Master's forks (they commit source to warp themselves), and anything routed through a closure not named here.

| Row / actor | Writes | Durable? | Committed by | When |
|---|---|---|---|---|
| `lyx loom start` (Run-Shed's spawn) | `_lyx/shed/self/{seed,status}.json`, `_lyx/fabric/origin.json` | tracked | `seedAndCommitBootstrap` (`bootstrapCommitPaths`) | every bootstrap, self-healing |
| Seed-Child (batten) | child `_lyx/shed/self/seed.json` | tracked | `CommitSeed` closure (`wire.go`) | at Seed-Child; idempotent on re-entry |
| shedengine persist (child) | `_lyx/shed/self/status.json` | tracked | loom `CommitStatus` seam, every transition (no skip) | live: one `loom: <row> -> <state>` commit each |
| Preflight, Loom-Preflight, Batchifier | nothing durable | — | — | gates |
| Discussion-Write (agent) | `_lyx/discussion/{decision-record,support-log}.md` (+ archive on re-run) | tracked | `CommitDiscussion` (whole dir) on Done | live: `loom: discussion artifacts` |
| Discussion-Bouncer / Discussion-Burler | `.lyx/loom/reviews/discussion/*`, `.lyx/burler/round-*/` ; overlay fixes into `_lyx/discussion` | ephemeral / tracked | `commit_seam: discussion` → `CommitDiscussion` | live: second `discussion artifacts` commit |
| Plan-Write (agent) | `_lyx/plan/*.md` | tracked | `CommitPlan` (whole dir) on Done | live: `loom: plan artifacts` |
| Plan-Bouncer / Plan-Burler | `.lyx/loom/reviews/plan/*`, `.lyx/burler/*`; overlay fixes + `approved:` flag | ephemeral / tracked | `approve_seam` then `commit_seam: plan` → `CommitPlan` | live: second `plan artifacts` commit |
| Webster (Master + `lyx webster` verbs) | `_lyx/webster/{state.json,outcome.yaml,summary.md,reports/*}`; `.lyx/webster/prompts/*`, `.lyx/shuttle/*` | tracked / ephemeral | `webstercli.fabricSync` at batch boundaries (`webster: begin-batch`, `record-batch`), then `CommitWebster` on Done (`loom: webster run record`) | live: all four present and clean |
| Webster forks / Webster-Burler (fix-scope: source) | warp source files | tracked (warp) | the agent's own commits on the task branch | live: `1: print-version` |
| Webster-Bouncer | `.lyx/loom/reviews/webster/*` | ephemeral | — | — |
| Publish | remote push of the task branch; PR (skipped: `require_pr_to_base: []`) | — | — | — |
| Finalize | merge-in into the task worktree; squash merge into the PARENT pair (prime `main`) | warp | fabric merge in the parent pair | live: prime `main` gained `Print \`fxapp v1\``, prime clean |
| Driver bookkeeping | `.lyx/loom/{driver.log,bootstrap.lock,step-handoff.json,selfreport-filed.json,friction/}` , `.lyx/reed.json`, `.lyx/logs/*`, `.lyx/shed/self/*.lock` | ephemeral | never | excluded via `.git/info/exclude` |
| llm driver (not runnable here) | `.lyx/shed/self/drive-report-*.md` | ephemeral | never | `driverReportPath` under `shedrun.ScratchDir` |
| Implementer build output | e.g. `fxapp` binary in the warp | untracked unless ignored | nobody | live: only the fixture's `.gitignore` kept the pair clean — an agent that builds without an ignore rule blocks teardown, as `docs/overview.md` already says |

Live proof: both halves' `git status --porcelain` empty immediately before `Worktree-Teardown` (see "What was tested").

## Focus-2 table: crash-window re-entry of every batten row

Enumeration: each row's `Call` in `internal/battenshed/*.go` and its closures in `internal/battencli/wire.go`; shedengine persists a transition only after `Call` returns (`run.go` `persist`), and batten's commit seam runs after the persist.

| Row | Side effects, in order | Gap | Re-entered Call does |
|---|---|---|---|
| Worktree-Create | (1) `Topology.Add`: branches, worktrees, junctions, origin record, portal/launchers, pushes; (2) persist `Seed-Child/running`; (3) commit+push prime status | kill in (1) | fabric rolls back; warp branch may be left (known item) → next create refuses with F2's text |
| | | after (1), before (2) | `taskWorktreePresent` true → Done (r1-proven; re-confirmed by reading) |
| | | after (2), before (3) | next persist commits status+seed (paths always included) |
| Seed-Child | (1) `WriteSeed` child; (2) `CommitSeed` child pair; (3) `PushSeed`; (4) persist | after (1) | `WriteSeed` idempotent on the same seed → commit → Done |
| | | after (2) | commit of a clean tracked path is a no-op → Done |
| | | after (3), before (4) | same; Board type re-read fresh — a type changed meanwhile → disagreeing seed → Stuck (documented) |
| Run-Shed | (1) clear marker; (2) `loom start --no-attach` (child seed/status/origin committed, reed up, watchdog, driver spawn, handshake); (3) write marker; (4) re-read; (5) sleep; (6) persist Stuck/running | in (2) before the child's status is seeded | status absent → spawn again |
| | | in (2) after seeding, before the driver is up; or after (2) before (3) | status `running`, no marker → spawn again; bootstrap idempotent against a live driver — **driven live** (`status_found=true`, fresh driver) |
| | | after (3) | marker present → plain read → Stuck/Done/error |
| | | marker's scratch dir removed while the driver is alive (other machine / rm) | spawn again → `loom start` finds the run lock held → no second driver (traced; the same bootstrap path as above) |
| | | after (5) before (6) | one extra sleep; harmless |
| Worktree-Teardown | (1) `reed Down`; (2) `Topology.Remove`: portal, launchers, junctions, warp dir, weft+branches; (3) persist done; (4) commit+push | after (1) | Shutdown again: reed Down on a session already gone → ok (r1-proven), Remove proceeds |
| | | in (2) after the warp dir, before the weft | **F1**: warp absent → both halves skip → `done` over the leftover — **driven live** |
| | | after (2) before (3) | warp absent, weft absent → Done (r1-proven) |
| Child bootstrap (`loom start`) | seed → status seed → ownership check → commit → bootstrap lock → reed up/status strand → watchdog → driver spawn → handshake | any | every step idempotent: seed write no-op, `ErrSeedExists` tolerated, commit of clean paths no-op, live driver detected by the run lock (`mustSpawnDriver`) |

## Docs and operability findings

- `docs/overview.md` batten entry: "`Worktree-Teardown` is idempotent against its own post-condition … finds the task worktree gone and reports done" — after F1's fix this must say the pair, not the task worktree (same commit as the fix).
- The done-slug remedy text (F4) and the create-row remedy (F2) are the two operator-facing texts that lead prime's operator astray; the absent-worktree text fixed in r1 is the model for both.
- `internal/battenshed/doc.go`'s two residual paragraphs are still accurate; F5 is the operational face of the second one and is worth a sentence there if the operator keeps the residual.
- The `paused` step envelope carries `producer: ""`/`outcome: ""` — shedverbs' generic shape; fine.
- PrimeRunLock contention halts the second slug `blocked` rather than waiting (no `on_stuck` on the bookend rows); operator re-steps. Deliberate; noted for operability, not a finding.

## What was tested

Hermetic baseline at HEAD `647220980`:

- `go build ./... && go vet ./... && go test -count=1 ./...` — exit 0, 92 packages `ok`.
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./internal/shedrun/... ./cmd/lyx/...` — exit 0.
- `go test -tags integration -count=1 ./internal/battencli/...` — exit 0.

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
- F3 observed: `lyx shed seed zz2 --recipe batten` from the TASK worktree `fx-untyped` is accepted and writes `_lyx/shed/zz2/seed.json` into the task pair's weft, although every batten verb refuses there (`lyx shed status zz2` → non-prime refusal). Removed by hand afterwards.

### PrimeRunLock scope across two slugs

- Held `fxapp/.lyx/shed/run.lock` by hand (`flock -x … -c 'sleep 40'`), then `lyx batten step fx-typed` (first row): `Worktree-Create` → `stuck` → `state: blocked`, `stuck_reason` names that lock path; no `fx-typed` pair created. The lock is hub-wide (`.lyx/shed/run.lock`, no slug segment), so a second slug's create/teardown is serialised against any other slug's. Contention is a `blocked` halt (no `on_stuck` on the bookend rows), not a wait — the operator re-steps. Each re-step commits `Worktree-Create -> running` then `-> blocked` on prime's weft (two commits per attempt).
- Released the hold; `lyx batten step fx-typed` resumed silently → create done → `Seed-Child` done; child seed `{loom, go, parent: main}` committed on `fx-typed-weft`.

### fx-typed — the full `--child-driver go` drive (background step loop)

- `~/.claude.json` `.projects` had no entry for the child path before launch (946 entries).
- Launched `drive-fx-typed.sh`: `lyx -v batten step fx-typed` in a loop, stopping when `next != Run-Shed`, i.e. right before `Worktree-Teardown`. First step: `spawning inner shed run status_found=false`, bootstrap returned in 0.23s, child at `Discussion-Write/running`, a `claude … --model sonnet` provider alive in the child's tmux session.

### fx-untyped — Run-Shed re-entry with status `running` and no spawn marker (emulated kill-before-marker)

- Rewound the child's committed status to `Preflight/running`, removed `.lyx/shed/fx-untyped/Run-Shed-spawned`, resumed with `lyx -v batten step fx-untyped`: `spawning inner shed run status_found=true` → real `lyx loom start --no-attach` → a fresh detached driver ran Preflight, blocked again on the dirt (`loom: Preflight -> blocked` committed a second time on the child weft) → Run-Shed hard error, marker rewritten. The re-spawn-on-running-without-marker path is live and idempotent against the real bootstrap.

### fx-untyped — Worktree-Teardown re-entered against a half-torn pair (F1)

- Removed the dirt, hand-wrote the child's status `done` and committed it; `lyx batten step fx-untyped` → `Run-Shed` `done`, `next: Worktree-Teardown`.
- Emulated the crash window between fabric's warp-worktree removal and its weft teardown: `git -C fxapp worktree remove fx-untyped`, leaving `fx-untyped-weft`, both branches (local + remote), `_portals/fx-untyped`, `_launchers/fx-untyped` and the tmux session `fx-untyped` behind.
- `lyx -v batten step fx-untyped` → `session shutdown skipped, the task worktree is already gone` → `teardown worktree skipped, the task worktree is already gone` → `outcome: done, state: done`. The run is `done` with `fx-untyped-weft` still on disk, both branches present, the portal/launcher entries present, and the tmux session still alive at that moment (the per-hub watchdog reaped the session a few minutes later once its worktree was gone — observed, not relied on). `lyx fabric pairs` still lists the half pair; `lyx fabric prune` (dry run) is the fabric verb that exists for exactly this orphan-weft debris.

### fx-typed — pause during the watch, mid-campaign porcelain

- `lyx batten pause fx-typed` mid-`Run-Shed` → ok; consumed at the next boundary (one `state: paused` envelope in the loop log, prime weft commit `batten: Run-Shed -> paused`, then `-> running` on the next step).
- Mid-campaign porcelain sample (child at `Plan-Bouncer`): child warp clean, child weft clean, prime weft `M _lyx/shed/fx-typed/status.json` (the documented uncommitted self-bounce history). `.lyx/{burler,logs,loom,reed.json,shed,shuttle}` all ephemeral.

### Done-slug, busy and typed-flag refusals

- `lyx batten run|step fx-untyped` on the done slug: both refuse naming the run directory (`step` with `kind: bootstrap`).
- Holding `fxapp/.lyx/shed/fx-typed/run.lock` by hand: `step` → `kind: busy`, `run` → busy text, `pause` → ok (a flag write, correctly allowed). This hold collided with the background loop's next step (its step#9 got the busy refusal and the loop stopped as designed) — a fixture artefact.
- `lyx batten step fx-typed --child-driver llm` → refused (seeded child driver `go`); `--driver llm` → "batten has no bootstrap verb"; `lyx batten step nonexistent-slug --driver llm` → same refusal and no `_lyx/shed/nonexistent-slug` written.

### fx-typed — success terminal state reached (`Worktree-Teardown` → `done`, no operator step)

- Child campaign (go driver, sonnet agents) ran `Discussion-Write → Discussion-Bouncer → Discussion-Burler → Discussion-Bouncer → Plan-Write → Plan-Bouncer → Plan-Burler → Plan-Bouncer → Batchifier → Webster → Webster-Bouncer → Webster-Burler → Webster-Bouncer → Publish → Finalize/done` in ~4 min. Prime warp `main` received the squash `Print \`fxapp v1\`` (`main.go` prints `fxapp v1`), prime warp porcelain clean.
- **Focus-1 live proof, right before teardown:** child warp `git status --porcelain` empty (`--ignored` shows only the `_lyx`/`.lyx` junctions and the fixture's gitignored `fxapp` binary built by the implementer), child weft porcelain empty (`--ignored`: `.lyx/`, `.weft/`), every `_lyx` artifact tracked: `discussion/*`, `plan/*`, `fabric/origin.json`, `shed/self/{seed,status}.json`, `webster/{outcome.yaml,state.json,summary.md,reports/*}` (commits: `loom: discussion artifacts`, `loom: plan artifacts`, `webster: begin-batch`/`record-batch`, `loom: webster run record`, one `loom: <row> -> <state>` per transition). Prime weft: `M _lyx/shed/fx-typed/status.json` only.
- `lyx batten step fx-typed` → Run-Shed read the child's `done` → `next: Worktree-Teardown`; `lyx -v batten step fx-typed` (teardown): `reed: down complete session=fx-typed` (reed also tore down the now-empty per-hub tmux server), `battencli: teardown worktree` mutations recorded, envelope `producer: Worktree-Teardown, outcome: done, state: done`. Hub afterwards: `fx-typed`/`fx-typed-weft` gone, `_portals/fx-typed` + `_launchers/fx-typed` gone, no tmux server, `abandonedSession` absent, prime weft `batten: Worktree-Teardown -> done` committed and clean. Left by fabric's design: local warp branch `fx-typed`, remote `origin/fx-typed` + `origin/fx-typed-weft` (`Remove(…, remote=false)`); local `fx-typed-weft` deleted.
- Substrate after teardown: only the per-hub `lyx reed watchdog` remains; no `loom run`, no `claude` for the fixture.
- `~/.claude.json`: one new `projects` entry for the child path appeared (946 → 947), `hasTrustDialogAccepted: null` — the agents ran under `--dangerously-skip-permissions`; no trust dialog for the go-driven child. File left alone.

### Re-running a torn-down slug (F2, F4)

- Followed the done remedy: `git rm -r _lyx/shed/fx-typed` on prime's weft, committed. `lyx batten step fx-typed` → `Worktree-Create` `blocked`, `stuck_reason` = fabric's `branch "fx-typed" already exists; switch a pair onto it with "lyx fabric checkout fx-typed", or delete it first with "git branch -D fx-typed" …`.
- Followed the first arm from prime: `lyx fabric checkout fx-typed` → ok, mutations `worktree_switched fxapp → fx-typed`, `worktree_switched fxapp-weft → fx-typed-weft`, `branch_created fx-typed-weft`; prime's `_lyx/shed` now the task branch's view. Restored with `lyx fabric checkout main` (both halves back, porcelain clean).
- Followed the second arm: `git branch -D fx-typed` (and the weft branch) → step → create ran to the push, `! [rejected] fx-typed -> fx-typed (non-fast-forward)` against `origin/fx-typed`; fabric's rollback logged `warp-branch deletion was refused by the destructive gate; the branch is left behind` (matches the known fabric-rollback item) → `blocked`. Deleted both remote branches → step → "already exists" again (the rolled-back local branch) → `git branch -D fx-typed` once more → step → `Worktree-Create` done, fresh pair created (kept for fix verification).

### Not verified, and why

- `--child-driver llm` end to end: `ly@loomyard` plugin absent on this host (environment gap; the prompt's own deferred item).
- F5 live: Tier 2 friction reflection files real GitHub issues through `lyx selfreport create` irrespective of `selfreport: false`; deliberately kept off. Traced through `loomcli/run.go` + `arm.go` instead.
- Windows path behaviour: Linux host.
- A real SIGKILL inside the ~0.4s `loom start` window: emulated by reconstructing the exact on-disk state (status `running`, no marker) instead; the resumed code path is the same read-before-spawn branch and was driven against the real bootstrap.

## Teardown

(filled at the end of the round)
