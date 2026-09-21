# batten — independent review (round `opus-high-r1`)

Round tag: `opus-high-r1`.
Worktree: `/home/hanf/Code/loomyard/wts/crucible-batten-end-to-end`, branch `crucible-batten-end-to-end`.
Clean-room: findings below were formed from the code, the recovered design doc (`git show 8ac857ce1~1:manifest/designs/seeded-shed.md`), `CONSTRAINTS.md`, `docs/overview.md`, and live driving — with no prior `_mill/batten-review-*` material read first.

> Status: IN PROGRESS — appended as scenarios return, per the round prompt's log-as-you-go rule.

## Executive summary

Placeholder — filled once the findings list is closed.

## Scope assessment (design doc vs shipped)

Placeholder.

## Code findings

### F1 — Seed-Child writes a child seed the child's own bootstrap refuses (BLOCKING)

`internal/battencli/wire.go:187`

Batten's `SeedChild.WriteSeed` seam writes the child's `_lyx/shed/self/seed.json` as:

```go
shedrun.WriteSeed(childLocation, shedrun.SelfRunID, shedrun.Seed{Recipe: recipe, Driver: driver})
```

with **no `Params`**.

`Run-Shed`'s `Spawn` seam (`internal/battencli/wire.go:133`) then runs `lyx loom start --no-attach` inside that child.
`loom start`'s bootstrap (`internal/loomcli/sharedbootstrap.go:104`) writes its own seed via `loomSeedFor(parent, driver)`, which **always** sets `Params: {"parent": parent}` (`sharedbootstrap.go:184-190`).

`shedrun.WriteSeed` (`internal/shedrun/seed.go:138-176`) is idempotent only against an *agreeing* seed, and agreement includes `paramsEqual(existingSeed.Params, seed.Params)`.
`paramsEqual` compares by length first (`seed.go:180`), so `nil` (batten's write) vs `{"parent": "<branch>"}` (loom's write) is a disagreement, and `WriteSeed` returns:

> `shedrun: run "self" is already seeded with {...}; refusing to overwrite with disagreeing seed {...}`

Failure scenario (the ordinary, only path): `lyx batten run <slug>` → `Worktree-Create` Done → `Seed-Child` Done (writes the params-less seed, commits, pushes) → `Run-Shed` spawns `lyx loom start --no-attach` → loom's bootstrap refuses at `bootstrapStageSeed` → non-zero exit → `innerRunProducer.Call` returns the hard error at `internal/battenshed/innerrun.go:97` → `shedengine` marks Run-Shed `StateFailed` and halts.
The task worktree is created and seeded and then never runs, and `Worktree-Teardown` is never reached.

Severity BLOCKING: it breaks batten's whole reason to exist on the first, ordinary invocation, for both `--child-driver go` and `--child-driver llm`.
It has never been caught because every batten test stubs `InnerRunDeps.Spawn` at the field level, so no test has ever executed the real `lyx loom start` this seam runs.

Status: see "What was tested" for the live confirmation.

Suggested fix: make the seed batten writes for the child the seed the child's own bootstrap agrees with, rather than one it must overwrite.

### (further findings appended below as the review proceeds)

## Docs & operability findings

Placeholder.

## What was tested

### Hermetic baseline (before any edit)

- `go build ./...` → clean.
- `go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...` → clean.
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...` → all `ok`
  (`battenshed` 0.075s, `battencli` 0.358s, `battenrecipe` 0.040s, `cmd/lyx` 6.782s).

### Live driving

Pending — appended as each scenario returns.

## What could NOT be verified

Pending.
