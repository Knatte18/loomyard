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
Status: CONFIRMED — demonstrated on a `git archive HEAD` copy in the scratchpad (L5).

### F3 — the absent-task-worktree recovery advice is false and, followed verbatim, destructive for no benefit (provisional MEDIUM)

`internal/battencli/wire.go:57` (`taskWorktreeLocation`'s refusal text).
The advice says "resolve this by hand, deleting the pair's branches so a resumed run reaches a fresh create".
Deleting branches does not rewind batten's durable status: the resumed run resumes at its persisted `current_producer` (`Run-Shed` here) and hits the identical refusal again, so it never "reaches a fresh create".
What actually rewinds to a fresh create is deleting prime's batten run directory (`_lyx/shed/<slug>/`), which the message never names; and "the pair's branches" silently means four refs (both local branches AND both `origin` branches `Seed-Child`/`Worktree-Create` pushed), since a re-create with only the locals deleted is refused by fabric's own push ("non-fast-forward" on `<slug>-weft`).
Meanwhile deleting the branches throws away the task's unmerged work — the destructive half of the advice buys nothing.
Also the refusal is surfaced as a hard error with `kind: producer`, which ly-drive retries once — harmless (it refuses again) but noted.
Status: CONFIRMED live (L6).
Suggested fix: reword to the true recovery: either restore the pair by hand from its branches (non-destructive, work preserved), or abandon this run by deleting prime's run directory (name its path) AND the pair's local and remote branches, after which "lyx batten run <slug>" re-seeds and starts from a fresh create — stating that the second path discards the task's unmerged work.

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
- L5 (F2 demo): `git archive HEAD | tar -x -C scratch/f2copy`; appended `func illegitimateDriverGate(seed shedrun.Seed) bool { return seed.Driver == shedrun.DriverLLM }` to the copy's `internal/battencli/arm.go`; `go test -run TestDriverChoiceSingleSiteInvariant ./internal/loomcli/` → `ok` (planted gate passes silently). Control: the same function in a new `internal/boguspkg/b.go` → FAIL. The working tree was never touched.
- L6 (F3): moved `t2` and `t2-weft` out of the hub (simulating "on another machine"); `lyx batten step t2` → hard error (kind `producer`) with the recorded advice. Followed it verbatim: `git worktree prune` (needed first — `git branch -D t2` refuses while git still records the moved worktree), `git -C warp branch -D t2`, `git -C warp-weft branch -D t2-weft`; `lyx batten step t2` → the IDENTICAL refusal at `Run-Shed`; `lyx batten status t2` still `current_producer: Run-Shed`. Only after also deleting prime's `_lyx/shed/t2/` did `lyx batten step t2 --child-driver go` reach `Worktree-Create`, which then blocked: first on my simulation's leftover portal link (artefact of moving dirs, not a defect), then on `push weft branch "t2-weft" failed ... non-fast-forward` because the origin branches `Seed-Child`/create pushed still exist. `t2` is left blocked at create and is not reused (deleting the bare repos' branches was refused by this session's permission gate).
- L7 PrimeRunLock/dirty-status interplay: while `t3` was watching (prime weft carrying the documented uncommitted `M _lyx/shed/t3/status.json`), `t2`'s Worktree-Create proceeded past fabric's clean-tree checks all the way to the push — a watching slug's uncommitted status does not block another slug's create.

## Teardown

(pending)
