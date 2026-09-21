# batten review — sonnet-xhigh-r3

Round 3 of 4. Independent, clean-room review (no prior `_mill/batten-review-*` material read before
this file's findings section was drafted). Worktree: `/home/knatte/Code/loomyard/wts/crucible-batten-end-to-end`,
branch `crucible-batten-end-to-end`.

## Status

IN PROGRESS — this file is being built incrementally per the round prompt's "log as you go" rule.

## What was read (not a review; context)

- `CONSTRAINTS.md` (full), `docs/overview.md` batten entry + execution-stack section.
- `internal/battenshed/*.go` (create.go, seamchild.go, innerrun.go, teardown.go, ctx.go, deps.go, stuck.go, doc.go) and their tests.
- `internal/battenrecipe/*.go` (battenrecipe.go, names.go, doc.go) and recipe_test.go, fixture_test.go, seam_enforcement_test.go.
- `contracts/recipes/batten-recipe.yaml`.
- `internal/battencli/*.go` (cli.go, arm.go, wire.go, paths.go, refusal.go, commitstatus.go, bootstrapverb.go) and lifecycle_integration_test.go, testmain_integration_test.go; test function names surveyed for the rest.
- `internal/shedrun/{seed,paths,runid}.go`, `internal/shedengine/run.go` (stepLocked/Run/Step).
- `internal/loomcli/{start,sharedbootstrap,driverlaunch,driverspec}.go`, `internal/loomengine/driver.go` (the `--driver llm` boundary batten's Spawn seam crosses into).
- Recovered design doc: `git show 8ac857ce1~1:manifest/designs/seeded-shed.md`.
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md` F22 (scenario ideas only).

## What was tested (commands + observations, appended live)

### Environment setup

- `./deploy-dev` — deployed `lyx @ a7a440265` to `.dev-bin/lyx`. Re-run after every source change per the round's footgun warning.
- Disposable fixture hub built by hand in `/tmp/claude-1000/.../scratchpad/batten-r3-hub` (outside both the loomyard tree and `$HOME/Code`):
  bare `weft-fixture.git` (empty, HEAD=main) and `warp-fixture.git` (seeded with a minimal `go.mod`/`main.go`), then
  `lyx fabric clone <weft-bare> <warp-bare>` into `hubparent/warp-fixture-LYXHUB` — this single command also materializes `_board`
  (fabric clone's own Long text confirms `_board` is wired automatically; the round prompt's own step-3 instruction to materialize it
  separately is unnecessary against the current `fabric clone`).
  Exit 0, full mutation list logged (worktrees + junctions + board commit + weft config commit).

### Scenario: item 11 — `lyx shed seed <slug> --recipe batten` pre-seeding ahead of batten's own auto-seed

- `lyx shed seed disagreeing-slug --recipe loom --param parent=main` then `lyx batten run disagreeing-slug`:
  refused (`already seeded with recipe "loom", not "batten"`), seed left untouched (`cat` confirms `recipe: loom` survives),
  no task worktree created. Matches spec — CONFIRMED non-defect.
- `lyx shed seed pre-seeded-slug --recipe batten` (agreeing shape), then seeded a real Board task `pre-seeded-slug` (`type: loom`),
  then `lyx batten step` x2 (Worktree-Create, Seed-Child both `done`): prime's own seed file for the slug is verified byte-identical
  after both steps (`{"recipe":"batten","driver":"go"}`, never rewritten to add e.g. a `params` key) — the hand-seed stayed authoritative.
  CONFIRMED non-defect.

### Scenario: item 1 — Batten Bookend Invariant

Drove `lyx batten status/run/step pre-seeded-slug` from inside the just-created task worktree (`$HUB/pre-seeded-slug`): all three refuse,
naming both worktree names (`"pre-seeded-slug" is not the prime worktree ("warp-fixture" is)`) and telling the operator to re-run from there.
Drove `status` from the weft sibling (`pre-seeded-slug-weft`), from `_board`, and from prime's OWN weft sibling (`warp-fixture-weft`): all
three refuse via `fabricengine.RequireWarpWorktree`, naming the specific non-warp checkout kind (weft sibling / `_board` checkout).
Also confirmed the refusal names the WORKTREE the operator is standing in, not the slug argument they typed (ran `batten status some-other-slug`
from inside `pre-seeded-slug` — refusal still names `"pre-seeded-slug"`, not `"some-other-slug"`), so the message is never ambiguous about
which fact it is reporting. CONFIRMED non-defect — all of item 1's required refusals hold.

(remainder appended as scenarios run)

## Findings (provisional; severity/ordering finalized at the end)

(none recorded yet)

## Scope assessment

(pending)

## Docs & operability findings

(pending)

## Executive summary

(pending — written last)
