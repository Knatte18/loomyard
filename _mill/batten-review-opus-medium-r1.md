# batten follow-up review — opus-medium-r1

Round 1 of the follow-up campaign. Reviewer tag `opus-medium-r1`.
Worktree `/home/knatte/Code/loomyard/wts/crucible-batten-followup`, branch `crucible-batten-followup`.

## Executive summary

(pending; written once the review is complete)

## Scope assessment

(pending)

## Code findings (provisional, severity ordering finalized at the end)

### F1 — Run-Shed's read-before-spawn treats a failed child bootstrap's seeded status as "spawn already happened" (provisional MEDIUM)

`internal/battenshed/innerrun.go` (Call, `!found` arm) and `internal/battencli/wire.go` (Spawn).
`lyx loom start` seeds the child's `status.json` (state `running`) and commits it in its step 1, BEFORE its driver spawn and readiness wait (steps 5-6, `internal/loomcli/start.go`).
So a `loom start` that fails after step 1 — `ensureStatusStrand` failing, the go arm's run-lock handshake refusing, or the llm arm's `StartDriver` returning a startup-gate/not-ready error — leaves the child with a `running` status file and no driver.
The first `Run-Shed` Call reports that honestly as a hard error (`spawn inner shed run: exit status 1: ...`), but the error names no remedy.
The operator's natural next move, resuming `lyx batten run <slug>`, finds the status file present, never re-spawns, and self-bounces "inner shed run still running" for the whole 1440-bounce (12h) window with no driver behind it.
Status: CONFIRMED live (see What was tested, scenario L3): with `LYX_REED_TMUX=/nonexistent/tmux` the child's `loom start` fails at step 4 after seeding `status.json` (`current_producer: Preflight`, `state: running`); the `Run-Shed` step reports `kind: producer` (so ly-drive retries it once); the very next `lyx batten step t2` without the bad env reports `outcome: stuck`, "inner shed run still running; sleeping 30s", with no `lyx loom run` process and no tmux session anywhere for the fixture.
Every further bounce repeats that for the 12h budget.
Suggested fix: record a spawn-confirmed marker in the row's own scratch directory only after `Spawn` returns success (cleared before each attempt), and spawn whenever the child has no status file OR (its status is `running` AND no confirmed spawn is recorded); `loom start --no-attach` is idempotent against a live driver, so the retry is safe.

### F2 — `driverFieldReadCarveOuts` is file-granular, so a second, illegitimate Driver read in `internal/battencli/arm.go` passes the tripwire silently (provisional LOW)

`internal/loomcli/bootstrap_test.go:694` keys the carve-out on the whole file.
Its justification names one site (`refuseAdoptedSeed`'s comparison), but any other `<seed>.Driver` read added anywhere in `arm.go` — including one that gates behaviour, which is exactly what the Driver Choice Single-Site Invariant forbids — is skipped by `if driverFieldReadCarveOuts[f.relPath] { continue }`.
Suggested fix: key the carve-out on file plus enclosing function (`internal/battencli/arm.go` + `refuseAdoptedSeed`) and have the scanner stamp each hit with its enclosing `FuncDecl` name.
Status: CONFIRMED by reading (demonstration below in What was tested once run).

## Focus-1 enumeration table

(pending)

## Focus-2 enumeration table

(pending)

## Docs & operability findings

(pending)

## What was tested

- Read: recovered SPEC (`git show 8ac857ce1~1:manifest/designs/seeded-shed.md`), `internal/battenshed/{doc,deps,innerrun,seamchild}.go`, `internal/battencli/{wire,arm}.go`, `internal/shedrun/seed.go`, `internal/loomcli/{start,driverlaunch,sharedbootstrap}.go`, `internal/shedcli/seed.go`.
- `git show --format= 36900c6f5 -- <the six non-batten files>`: the whole predecessor hardening landed in one squash commit; its non-batten code hunks are `shedrun.ErrDisagreeingSeed` (+ message reword), `fabricengine.RequireDrivableWorktree`, the `driverFieldReadCarveOuts` allowlist, a comment fix in `shell/posix.go`, two help lines in `boardcli/cli.go`, and a doc-comment edit in `shedrecipe/entries_batten.go`.
- `grep -rn 'signal.Notify\|NotifyContext\|os/signal'` repo-wide: no signal handling anywhere in lyx; `cmd/lyx/main.go` runs the root on a never-cancelled context.
- Hermetic at start of Job 1: `go build ./... && go vet ./... && go test -count=1 ./...` — all green (EXIT 0).
- Third filing path sweep: `ls -d internal/*selfreport* internal/*friction*` (selfreportcli, selfreportengine, friction, frictionengine) and `grep -rln 'CreateIssue\|issue create\|/issues'` over `internal cmd`: only `selfreportengine.CreateIssue` (reached from `loomcli/arm.go` Tier 1 and `selfreportcli`); `landingshed/publish.go` opens PRs, not issues. The only prose path is `ly-drive`'s operator-gated `lyx selfreport create`, which the autonomous section keeps gated (it becomes a report line). No third automatic path. As belt-and-braces every live command ran with `GH_TOKEN=GITHUB_TOKEN=invalid-crucible-guard`.
- L0 fixture: bare `warp.git` (go.mod + main.go) and an EMPTY bare `weft.git` (a weft with a non-empty initial commit is refused by clone: "its history carries neither .lyx-anchor nor an empty tree") under the session scratchpad `.../scratchpad/fx/`; `lyx fabric clone --into .../fx/hubs weft.git warp.git` built `warp-LYXHUB/{warp,warp-weft,_board}`. Committed onto prime's weft (`09352c2`) BEFORE any Board task: `selfreport: false`, `friction: ""` (loom.yaml), `require_pr_to_base: []` (landing.yaml — the key lives there, not in loom.yaml), plus `sonnet[effort=low]` for discussion/plan/review/driver/conflict; verified with grep. Board tasks `t1` (`type: loom`), `t2` and `t3` (type empty).
- L1 bookend floor: `lyx batten status t2` from `_board` and `lyx batten run t2` from `warp-weft` both refuse naming the checkout ("is the hub's _board checkout" / "is the weft sibling of a pair"); from prime, `status t2` before any seed refuses with the missing-seed message; `lyx batten run` (no slug) and `lyx batten run self` refuse with their two distinct texts.
- L2 `t2` (type empty) stepped: `lyx batten step t2 --child-driver go` → Worktree-Create done (0.10s); `lyx batten step t2` → Seed-Child done; child seed `{"recipe":"loom","driver":"go","params":{"parent":"main"}}` committed as `batten: seed child t2` on `t2-weft`; prime seed `{"recipe":"batten","driver":"go","params":{"child_driver":"go"}}`. Empty Board type defaulted to `loom` as designed.
- L3 (F1): `LYX_REED_TMUX=/nonexistent/tmux lyx batten step t2` → `{"error":"battenshed: Run-Shed: spawn inner shed run: exit status 1: {\"error\":\"run -V: fork/exec /nonexistent/tmux: no such file or directory\"...}","kind":"producer"}`; child `status.json` left at `Preflight`/`running`. Then plain `lyx batten step t2` → 30.0s, `outcome: stuck`, "inner shed run still running", no driver process, no tmux session for the fixture. F1 confirmed.
- L4 focus-2 timing, go child: `lyx -v batten step t3` (3rd step, Run-Shed first Call) logged `spawning loom session` 11:27:49.163 → `loom session wait complete` 11:27:49.639: Spawn blocked 0.48s for a `go` child (the go arm's run-lock handshake). Then `lyx batten run t3` backgrounded to completion (output to scratch `t3-run.out`).

## Teardown

(pending)
