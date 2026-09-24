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
Status: CONFIRMED by reading; live repro pending.

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

## Teardown

(pending)
