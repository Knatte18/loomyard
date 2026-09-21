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

### F0 — Run-Shed's first `ReadStatus` hard-errors: nothing creates the child's ephemeral status-lock directory (BLOCKING, CONFIRMED live)

`internal/battencli/wire.go:114-142`, failing at `internal/battenshed/innerrun.go:84-87`.

`InnerRunDeps.ReadStatus` is wired to `state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)` (`wire.go:140-142`), and `ResolveStatus` (`wire.go:114-120`) hands it `shedrun.StatusLock(taskLocation, shedrun.SelfRunID)` — the child worktree's **ephemeral** `.lyx/shed/self/status.json.lock`.
`state.ReadJSONStrict` deliberately never creates the lock's directory, so the read fails outright when `<task-worktree>/.lyx/shed/self/` does not exist.

Nothing creates it. `Worktree-Create` creates the pair (which yields `<task>/.lyx` with only `logs/` in it); `Seed-Child` writes only the **durable** `_lyx/shed/self/seed.json` (via `shedrun.WriteSeed`, which MkdirAlls `RunDir`, not `ScratchDir`).
So on `Run-Shed`'s very first Call, the read-before-spawn check — the producer's own re-entry-safety mechanism — hard-errors before `Spawn` is ever reached.

This is precisely the failure `battencli`'s own `battenPreRun`/`battenPreStep` already guard against one level up (`arm.go:321-324`, `arm.go:378-381` both `os.MkdirAll(filepath.Dir(...StatusLockPath))` with a comment naming the bare "no such file or directory" this avoids) — the same guard was never carried down to the child's status-lock path.

Reproduced live, on a fresh disposable hub, `lyx batten step fixtask` at the `Run-Shed` row:

```
{"error":"battenshed: Run-Shed: read status: acquire read lock: acquire read lock: open
 .../fixtask/.lyx/shed/self/status.json.lock: no such file or directory","kind":"producer","ok":false}
```

and the prime status went `state: "failed"` at `current_producer: "Run-Shed"`.

Severity BLOCKING: `Run-Shed` can never advance on a freshly created task worktree, for either child driver.
Batten has therefore never completed a single end-to-end pass; the campaign's premise is confirmed.

It is invisible to every existing test because `lifecycle_integration_test.go` replaces `InnerRunDeps.ReadStatus` at the field level with an in-memory stub, so the real `state.ReadJSONStrict`-over-a-real-child-path seam has no coverage at all.

Suggested fix: create the child's status-lock directory in the seam that owns the child's real filesystem paths — `wire.go`'s `ResolveStatus` closure — mirroring `arm.go`'s own MkdirAll for prime, with a regression test that drives the real `ResolveStatus`/`ReadStatus` pair against a task-worktree-shaped temp tree.

### F1 — Seed-Child writes a child seed the child's own bootstrap refuses (BLOCKING, CONFIRMED live)

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

Reproduced live, after hand-creating the directory F0 is missing so the run could reach `Spawn` at all:

```
$ lyx batten step fixtask
{"error":"battenshed: Run-Shed: spawn inner shed run: exit status 1","kind":"producer","ok":false}

$ cd <hub>/fixtask && lyx loom start --no-attach
{"error":"shedrun: run \"self\" is already seeded with {Recipe:loom Driver:llm Params:map[]};
 refusing to overwrite with disagreeing seed {Recipe:loom Driver:llm Params:map[parent:main]}","ok":false}
```

The child's seed on disk after `Seed-Child` was exactly `{"recipe":"loom","driver":"llm"}` — no `params` key.

Suggested fix: make the seed batten writes for the child the seed the child's own bootstrap agrees with, rather than one it must overwrite — i.e. carry the child's recorded parent branch into `params.parent` at `Seed-Child` time, read from the origin record `Worktree-Create` has already written into the pair.

### F2 — `Spawn` discards the child bootstrap's output, so a real failure reports only "exit status 1" (MEDIUM, CONFIRMED live)

`internal/battencli/wire.go:121-139`

The `Spawn` closure builds `exec.Command(exe, "loom", "start", "--no-attach")` and calls `cmd.Run()` without setting `Stdout` or `Stderr`, so the child's JSON error envelope is discarded.
`innerRunProducer.Call` then wraps the bare `*exec.ExitError` (`innerrun.go:97`), and the operator's envelope and the persisted `status.error` both read:

> `battenshed: Run-Shed: spawn inner shed run: exit status 1`

That is the entire diagnosis batten offers for any child-bootstrap failure — a refused seed, a missing parent branch, an unparseable module config, a dead provider binary all look identical.
Confirmed live: the F1 refusal above is completely invisible through batten's own surface; it took a hand-run of `lyx loom start` inside the child to see it.

This also reads against the spirit of the Live-Substrate Spawn Observability invariant: batten logs that it spawned and that the wait completed, but throws away the one thing the spawn said.

Suggested fix: capture the child's combined output and fold it into the returned error (and/or tee it to the child's own log directory), so the persisted `status.error` names the real cause.

### F3 — A disagreeing hand-written prime seed is silently adopted rather than refused (MEDIUM)

`internal/battencli/arm.go:127-164`

`armSeed` reads the seed at the addressed run-id and, when one is found, returns `nil` immediately — it never checks that the found seed is a **batten** seed.
`armAt` then wires `battenrecipe.New` regardless (`arm.go:232-236`, `specFor` at `arm.go:277`).

Failure scenario: an operator pre-seeds per F22's own extended scenario but names the wrong recipe —
`lyx shed seed some-slug --recipe loom` in prime — and then types `lyx batten run some-slug`.
Batten arms its own recipe and drives create/seed/run/teardown against a run whose recorded identity says `loom`.
The two disagree permanently and silently: `lyx shed run some-slug` from the same prime would arm `loomcli` against prime for the same run-id, while `lyx batten run some-slug` arms batten — the same run directory driven by two different recipes depending on which verb was typed.

The round prompt's focus item 11 asks exactly this: a hand-written seed must be authoritative, and a genuinely disagreeing one must be refused rather than silently adopted. Today only the first half holds.

Suggested fix: when a seed is found, refuse unless `seed.Recipe == shedrun.RecipeBatten`, naming both the recorded recipe and the verb that would honour it.

### F4 — `--driver` / `--child-driver` are silently ignored once a seed exists (LOW)

`internal/battencli/arm.go:136-163`

Both flags are read only inside `armSeed`'s absent-seed arm. On every later invocation the seed is found, `armSeed` returns early, and the flags are dropped without a word.

Failure scenario: an operator starts a run with the default (`lyx batten run some-slug`), sees the child come up with the Go driver, and re-runs `lyx batten run some-slug --child-driver llm` expecting the child to switch.
Nothing changes, nothing is said, and `lyx batten status` does not report the effective child driver either, so there is no surface that reveals the flag had no effect.

This is the right *behaviour* — a seed is a write-once startup record — but the wrong *reporting*: a flag that cannot take effect should say so.

Suggested fix: when a seed already exists and an explicitly-set `--driver`/`--child-driver` disagrees with it, refuse by name (the seed is authoritative and the flag is a mistake); when it agrees, proceed silently.

### F5 — The prime lock's own directory is created only as a side effect of a deeper MkdirAll (LOW)

`internal/battencli/wire.go:52-64`

`PrimeLock.Acquire` does `os.MkdirAll(shedrun.ScratchDir(location, slug))` — the **per-slug** scratch directory — and then acquires `shedrun.PrimeRunLock(location)`, which lives one level *above* it at `.lyx/shed/run.lock`.
The prime lock's parent directory therefore exists only because creating `.lyx/shed/<slug>/` happens to create `.lyx/shed/` on the way.

The two paths are produced by two independent `shedrun` constructors with no stated relationship. If either is ever re-rooted the `Acquire` starts failing with a bare ENOENT on the very first `Worktree-Create`, and the coupling is invisible at the call site.

Suggested fix: `os.MkdirAll(filepath.Dir(primeRunLockPath), 0o755)` for the lock it is actually about to take, keeping the per-slug MkdirAll only if the scratch dir is separately needed here.

### F6 — `Run-Shed`'s poll sleep also fires in `step` mode, making every step cost 30 s of dead time (LOW)

`internal/battenshed/innerrun.go:119-126`

The `StateRunning` arm sleeps `p.pollInterval` (30 s from the recipe) *after* reading the child's status and *before* returning `Stuck`.
In `run` mode that is the intended pacing of the self-bounce loop.
In `step` mode — the single-producer primitive an external supervisor calls one at a time — the cadence is the supervisor's, so the sleep buys nothing and simply makes every `lyx batten step <slug>` against a running child block for 30 s to report "still running".

Over a real 12-hour watch driven by `ly-drive`, that is 30 s of wall clock burned per poll for no benefit, and it makes `step` feel hung.

Suggested fix: make the pacing sleep a `run`-mode concern rather than a producer-body concern, or at minimum make it cancellable against the context so an operator stop is not held for the full interval (today `cancelErr` is only consulted *after* the sleep has already run to completion).

### F7 — The self-bounce grows prime's durable status file without bound, and `lyx batten status` prints all of it (LOW)

`internal/battenshed/innerrun.go:126` + `contracts/recipes/batten-recipe.yaml:32-38` + `internal/battencli/arm.go:446-452`

Every `Run-Shed` self-bounce appends a `HistoryEntry` (`shedengine/run.go:252-256`), and the row's budget is `max_bounces: 1440`.
A child that runs the full 12-hour window therefore leaves ~1440 identical `{"producer":"Run-Shed","outcome":"stuck"}` entries in prime's durable, fabric-synced `_lyx/shed/<slug>/status.json`, and `battenStatusExtras` returns the whole slice on every `lyx batten status <slug>` envelope.

The status file is committed onto prime's pair, so the noise is durable and travels. The envelope becomes unreadable long before the budget is exhausted.

Suggested fix: it is not batten's place to change `shedengine`'s history contract, but batten's own status envelope can stop being unusable — report the history in a bounded form (most recent N entries plus a count) rather than the raw slice.

### F8 — `InnerRunDeps.Spawn`'s "blocks until it exits" does not say what "it" is (NIT)

`internal/battenshed/deps.go:43-48`

The contract reads "Spawn starts the inner shed run and blocks until it exits", which a reader can fairly take to mean the inner **run** (the whole nested campaign) rather than the bootstrap **process**.
Traced and confirmed live: the command is `lyx loom start --no-attach`, which returns after a bounded bootstrap handshake — ≤30 s for `driver: go` (`internal/loomcli/start.go:38-41`, 300 attempts × 100 ms; the runner itself is `cmd.Start()` + `proc.Detach`, never waited) and ≤5 s for `driver: llm` (`internal/loomcli/driverlaunch.go:96-98`, 50 attempts × 100 ms; `Runner.Start` returns a handle without blocking).

So the answer to the round prompt's focus item 4 is: the first `InnerRun.Call` blocks only for the child's bootstrap launch, never for the nested campaign — `lyx batten step`'s first advancing call cannot hang for a whole loom run.
The doc comment is merely ambiguous about which process it means, which is worth one sentence given how load-bearing the answer is.

Suggested fix: say "blocks until that bootstrap process exits, which is not the inner run's own completion", and name the bound.

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
