# batten — independent review (round `opus-high-r1`)

Round tag: `opus-high-r1`.
Worktree: `/home/hanf/Code/loomyard/wts/crucible-batten-end-to-end`, branch `crucible-batten-end-to-end`.
Clean-room: findings below were formed from the code, the recovered design doc (`git show 8ac857ce1~1:manifest/designs/seeded-shed.md`), `CONSTRAINTS.md`, `docs/overview.md`, and live driving — with no prior `_mill/batten-review-*` material read first.

> Status: REVIEW CLOSED. Job 1 is complete and this file was committed before any production or test file was touched, per the round prompt's sequencing rule.
> A clearly-marked post-fix addendum at the bottom records the scenarios that were structurally unreachable until the blocking findings were fixed.

## Executive summary

**Batten has never completed a single end-to-end pass, and cannot.**
Two independent BLOCKING defects sit back to back on the one path the module exists to walk, and both were reproduced live on a disposable fixture hub this round:

1. **F0** — `Run-Shed`'s first `ReadStatus` hard-errors, because nothing ever creates the child worktree's ephemeral status-lock directory. The row fails before `Spawn` is reached, for either child driver.
2. **F1** — with F0 hand-patched, `Seed-Child` writes the child a seed carrying no `params`, and the child's own `lyx loom start` bootstrap then refuses to overwrite it with its own `params.parent`-carrying seed. `Run-Shed` dies at `exit status 1`.

Both survived because every batten test stubs `InnerRunDeps.Spawn` and `InnerRunDeps.ReadStatus` at the field level, and because two separate documents (F12) state that the driven path is covered by an integration test that in fact replaces exactly those two seams.

Three things did hold up well under adversarial driving, and are worth saying: the **Batten Bookend Invariant** refuses correctly from inside the managed worktree for all four verbs, naming both worktrees; `Run-Shed`'s failure really does halt at `StateFailed` without routing to the destructive teardown row, leaving the pair intact; and a re-driven `Worktree-Create` does not double-create.

The rest of the findings are about batten being unreadable when it fails: a child bootstrap failure reports only `exit status 1` (F2), a blocked run's status verb reports only `stuck with no OnStuck target` while the real reason sits in a file batten itself wrote (F10), and a cold machine dies on a raw `chdir` ENOENT (F9) against an obligation the design doc names explicitly.

**Merge-readiness (pre-fix): NOT MERGEABLE.**
Counts: 2 BLOCKING, 4 MEDIUM, 5 LOW, 2 NIT, 1 verified-no-defect.

## Scope assessment (design doc vs shipped)

Against `git show 8ac857ce1~1:manifest/designs/seeded-shed.md`:

| Design commitment | Shipped? |
| --- | --- |
| Four rows: `Worktree-Create`, `Seed-Child`, `Run-Shed`, `Worktree-Teardown` | Yes — `contracts/recipes/batten-recipe.yaml`, names pinned against `battenrecipe`'s constants |
| `Seed-Child` copies the recipe from the Board task's `type` field, defaulting to `loom` | Yes, and read fresh on every `Call` — but the seed it writes is unusable (F1) |
| Driver copied from batten's own seed params, never the Board's | Yes — `wire.go:162-174` reads `params.child_driver` |
| "`go` stays the only driver batten runs use" | Yes — `refuseBattenOwnDriverLLM` refuses `--driver llm` by name; confirmed live |
| `Run-Shed` self-routed on `Stuck`, `max_bounces: 1440`, `poll_interval_s: 30`, never routing to teardown on failure | Yes — and confirmed live that a `Run-Shed` hard error halts at `StateFailed` with the pair intact |
| Durable `_lyx/shed/<run-id>/` for seed and status; locks at the mirrored `.lyx` subpath | Yes — `shedrun` is sole declarer; batten's `paths.go` forwards only |
| Teardown as **one** row sequencing shutdown before removal, never two | Yes — `teardown.go:76-96`, `Remove` unreachable on a `Shutdown` failure |
| Relay-stepping deliberately not built | Correctly absent |
| **"batten's rows must self-heal machine-local resources a durable status promises but a cold machine lacks (recreate the child worktree from its branch before watching it)"** | **No — not shipped at all (F9).** This is the one named obligation the durable-status decision took on, and it is missing |

The two residuals the design doc shipped consciously as-is were re-verified against the current code and both still hold:

- **A dead driver strand is not detected, reported, or recovered by batten.** Still true. `InnerRun` reads the child's persisted `State`/`Error`/`CurrentProducer` and nothing else; there is no liveness probe anywhere in `battenshed` or `battencli`. It is now *partly* mitigated one level down — loom's own bootstrap runs `awaitDriverPane` (`internal/loomcli/driverlaunch.go:128-140`) to prove the pane came up live at launch — but nothing watches liveness across the 12-hour window, so a strand that dies at hour two still looks merely slow until the bounce budget runs out.
  **Judgment: keep as an accepted gap, not a new finding.** Choosing what batten should *do* about a dead strand (bounce, fail, respawn, escalate) is a real design decision, not a bug fix. What is missing and fixable is that the module's own documentation does not warn an operator that the watch cannot tell "slow" from "dead" — that gap is closed as part of this round's doc fixes.
- **A cleanly finished driver's strand and run directory are not torn down until the whole-worktree teardown row.** Still true, and still harmless: `Worktree-Teardown` removes the pair, and the strand goes with `reedEngine.Down()`. **Judgment: accepted gap, unchanged.**

Nothing shipped beyond scope.

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

### F9 — No cold-machine self-heal, and the failure is a bare `chdir ... no such file or directory` (MEDIUM, CONFIRMED live)

`internal/battencli/wire.go:42-44` (`taskWorktreeLocation`), reached from every seam past `Worktree-Create`.

The recovered design doc makes this an explicit, named obligation of the durable-status decision:

> "The cost is that batten's rows must self-heal machine-local resources a durable status promises but a cold machine lacks (recreate the child worktree from its branch before watching it) — the same pattern `loom start` already applies to a cold task worktree."

Nothing in batten does this. `taskWorktreeLocation` calls `lyxcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))` and every caller propagates the resolver's error as a hard error.

Reproduced live by emulating a cold machine — removing a slug's local pair while leaving prime's durable batten status pointing past `Worktree-Create`:

```
$ lyx fabric remove handseed --force      # pair gone, durable batten status intact at Seed-Child
$ lyx batten step handseed
{"error":"battenshed: Seed-Child: write seed: not a git repository: chdir .../handseed: no such file or directory",
 "kind":"producer","ok":false}
```

Two distinct problems: the self-heal is absent (scope gap), and the report is dishonest about what state the run is in — an operator reading this cannot tell "this machine has never materialized this task's worktree" from "something is broken with the resolver".
The round prompt's focus item 7 asks for exactly the second property: "reports honestly what state it's actually in."

Suggested fix, split: (a) detect an absent task worktree at the seam and refuse by name, stating that the pair is not materialized on this machine and what to do about it; (b) the recreate-from-branch half needs a fabric capability to materialize a pair from branches that already exist (`Topology.Add` refuses a pre-existing branch by design, as F14's own live evidence shows) — that is a fabric-side design decision, outside this round's declared scope.

### F10 — `lyx batten status` never surfaces the producer's own stuck reason, although batten wrote it to a file it knows the path of (MEDIUM, CONFIRMED live)

`internal/battencli/arm.go:446-452` (`battenStatusExtras`) + `internal/battenshed/stuck.go:35-48`.

`shedengine` persists the fixed string `"stuck with no OnStuck target"` as `status.error` for every stuck verdict, which `stuck.go`'s own header acknowledges — that is why `reportStuck` writes the real reason to `<scratchDir>/<producer>-stuck.md`.
But nothing reads it back: `battenStatusExtras` returns `found`, `status_path` and `history` only.

Reproduced live (crash-before-persist resume at `Worktree-Create`, see What-was-tested):

```
$ lyx batten status crashcreate
... "error":"stuck with no OnStuck target", "state":"blocked" ...

$ cat <prime>/.lyx/shed/crashcreate/Worktree-Create-stuck.md
branch "crashcreate" already exists; switch a pair onto it with "lyx fabric checkout crashcreate",
or delete it first with "git branch -D crashcreate" if it is a leftover from a removed pair
```

The operator's own status verb tells them nothing actionable, while a perfectly actionable remedy sits in a file batten wrote, in a directory batten can name, one `status` call away.
An unattended run is exactly the case `stuck.go` says the file exists for — and it is exactly the case where nobody is reading the scrolled-away log line either.

Suggested fix: have `battenStatusExtras` read the current producer's stuck-reason file when the state is blocked and surface it as its own envelope key.

### F11 — `step`-driven teardown never surfaces the abandoned session (LOW)

`internal/battencli/arm.go:264-269` and `arm.go:435-440`.

`abandonedSession` reaches the envelope through `Hooks.PostRun`, which `shedverbs` calls from `run`'s body only (`internal/shedverbs/run.go:53-54`).
`step` has its own `Hooks.PostStep` (`internal/shedverbs/step.go:126-127`), which batten leaves nil.

So a lifecycle driven the way `ly-drive` drives one — one `lyx batten step <slug>` at a time — never reports that session shutdown had to abandon a session rather than end it cleanly, even though that is precisely the value `TeardownDeps.Shutdown` exists to return and the receiver field exists to carry.
It is logged at `Warn` (`internal/battenshed/teardown.go:85-87`), which the run that produced it has already scrolled past.

Suggested fix: wire `PostStep` to emit the same key, so the two drive modes report the same facts.

### F12 — The sandbox suite and the integration test both claim coverage that does not exist (MEDIUM, docs)

`tools/sandbox/SANDBOX-FABRIC-SUITE.md:543` and `internal/battencli/lifecycle_integration_test.go:1-13`.

F22 instructs the operator to write, in their report:

> "the full driven path -- a completed create, loom run, and teardown -- is covered by the `integration`-tagged end-to-end test instead, so a sandbox operator does not read this scenario's narrower scope as an oversight."

That is false. `lifecycle_integration_test.go` replaces `Env.InnerRun.Spawn` with `func(ctx) error { return nil }` and `Env.InnerRun.ReadStatus` with an in-memory stub (`lifecycle_integration_test.go:82-83`), so the loom run is exactly the part it does not drive.

The test file's own header compounds it: it says the stubbing leaves "the real poll logic (`Env.InnerRun.ResolveStatus`, the persisted-state branching) ... exercised rather than bypassed" — but the seam that actually broke in production (F0: the real `ReadStatus` over the child's real ephemeral lock path) is the very one replaced.

This is not a cosmetic docs nit: a sandbox operator following F22 signs off on a coverage claim that is the direct reason F0 and F1 survived to this round.

Suggested fix: correct both statements to say what is really covered, and say plainly that no automated test drives a real child bootstrap.

### F13 — `lyx board upsert --help` omits `type`, the one Board field batten reads (NIT, docs)

`internal/boardcli/cli.go:103-118`

The help text enumerates "Optional fields" and lists seven, then says "Unknown keys are rejected".
`type` is accepted (`internal/boardengine/store.go:211-224`) and is the single field `Seed-Child` consults to choose the child's recipe (`internal/battencli/wire.go:148-158`), but an operator reading the help has no way to learn it exists — and the "unknown keys are rejected" sentence actively suggests it would be refused.
`short_name` is missing from the list for the same reason.

Suggested fix: list both in the optional-field block.

### F14 — Crash between `Worktree-Create` and its persist: correct, and worth pinning (no defect; recorded as verified)

Re-driving `Worktree-Create` against a pair it already created does **not** double-create: `fabricengine`'s pre-existing-branch refusal fires, the row returns `Stuck`, and the run halts `blocked` with a remedy in the stuck file (see What-was-tested).
That is the right behaviour. It is recorded here because the round prompt's focus item 7 asks for it explicitly and because nothing pins it today — the regression test is added as part of F0's fix batch.

## Docs & operability findings

- **F12** (above) — `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s F22 and `lifecycle_integration_test.go`'s header both claim coverage of the driven path that does not exist.
- **F13** (above) — `lyx board upsert --help` omits `type`.
- **F2 / F10 / F9** (above) are operability findings as much as code ones: in all three, batten knows the real reason and reports something else.
- `docs/overview.md`'s batten entry (~line 369) describes the module accurately but says nothing about the two child-driver flags (`--driver`, `--child-driver`) or about the dead-strand residual an operator watching a 12-hour run needs to know about. Folded into this round's doc fixes rather than raised as a separate finding.
- Not a finding, recorded for the next round: `lyx board list` returns `BriefTask` (`internal/boardengine/store.go:22-32`), which has no `Type` field, so the Board's recipe choice is invisible in the listing an operator actually reads. `lyx board get` does show it. Left alone as boardcli scope.

## What was tested

### Hermetic baseline (before any edit)

- `go build ./...` → clean.
- `go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...` → clean.
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...` → all `ok`
  (`battenshed` 0.075s, `battencli` 0.358s, `battenrecipe` 0.040s, `cmd/lyx` 6.782s).

### Live driving — fixture hub

Built by hand, disposable, outside both the loomyard tree and `$HOME/Code`, under this session's scratch directory:

- `git init --bare` warp (`battenfix.git`, seeded with `go.mod` + `cmd/hello/main.go` + `README.md` on `main`) and an **empty** bare weft (`wft.git`). The first weft attempt carried a commit and `lyx fabric clone` correctly refused it: *"refusing to bootstrap ... as a weft: its history carries neither .lyx-anchor nor an empty tree"*. Both bare repos needed `git symbolic-ref HEAD refs/heads/main` before clone would accept them — clone's own diagnostic named the fix precisely.
- `lyx fabric clone --into <scratch>/hub <weft.git> <warp.git>` → `ok: true`, hub `battenfix-LYXHUB` with `battenfix`, `battenfix-weft`, `_board`.
- `_board` was materialized by `clone` itself; no separate step was needed.
- `lyx board upsert '{"slug":"fixtask",...,"type":"loom"}'` → task created carrying `type: loom` (confirmed via `lyx board get`).
- Dev binary under test: `./deploy-dev` → `.dev-bin/lyx @ fc7f0aebc`. Every live command below ran that binary, never a PATH `lyx`.

### Live scenario 1 — refusal surface from prime, before any run exists

All from `<hub>/battenfix`. Exit code 1 on each (verified directly for `status`):

| Command | Result |
| --- | --- |
| `lyx batten status fixtask` | `battencli: no seed found for run "fixtask"; no run is seeded yet. run "lyx batten run <slug>" first` |
| `lyx batten pause fixtask` | same missing-seed refusal |
| `lyx batten run` | `battencli: no slug given; batten addresses a task worktree by slug ... pass the slug` |
| `lyx batten run self` | `battencli: slug "self" is reserved for addressing prime's own run and cannot name a task` |
| `lyx batten run fixtask --driver llm` | `battencli: batten has no bootstrap verb, so it cannot be driven by an LLM` |
| `lyx batten run fixtask --child-driver bogus` | `shedrun: unknown driver "bogus"; must be "go" or "llm"` |
| `lyx batten step fixtask --driver bogus` | same |

After all seven, `_lyx/shed` and `.lyx/shed` did not exist under prime at all — the read-only verbs created no state, as `EnsureStatusLockDir: false` intends. **OK.**

### Live scenario 2 — Batten Bookend Invariant (focus item 1)

After `Worktree-Create` materialized `<hub>/fixtask`, every batten verb was driven **from inside that worktree**:

```
$ cd <hub>/fixtask && lyx batten run|step|status|pause fixtask
{"error":"battencli: this verb runs from the hub's prime worktree only; \"fixtask\" is not the
 prime worktree (\"battenfix\" is) -- re-run it from there","ok":false}   exit=1
```

All four refuse, naming both worktree names and the remedy. **OK.**

### Live scenario 3 — the driven path, one row at a time (focus items 3, 4, 5)

```
$ lyx batten step fixtask --child-driver llm
  → producer Worktree-Create, outcome done, next Seed-Child        ✅ pair created
$ lyx batten step fixtask
  → producer Seed-Child, outcome done, next Run-Shed               ✅ child seed written+committed
  child seed on disk: {"recipe":"loom","driver":"llm"}             ← no params (F1's cause)
  prime seed on disk: {"recipe":"batten","driver":"go",
                       "params":{"child_driver":"llm","slug":"fixtask"}}   ✅ correct
$ lyx batten step fixtask
  → {"error":"battenshed: Run-Shed: read status: acquire read lock: ...
     /fixtask/.lyx/shed/self/status.json.lock: no such file or directory","kind":"producer"}   ❌ F0
```

Board-type → recipe plumbing (focus item 5) is **half** verified: the child's seed really does carry `recipe: loom` from the Board task's `type` and `driver: llm` from prime's `params.child_driver`, neither silently defaulted. The second half — the nested `ly-drive` loop spawning a real provider off that value — is unreachable behind F0/F1 and is driven after the fixes land.

After hand-creating the directory F0 never creates:

```
$ mkdir -p <hub>/fixtask/.lyx/shed/self && lyx batten step fixtask
  → {"error":"battenshed: Run-Shed: spawn inner shed run: exit status 1","kind":"producer"}   ❌ F1 + F2
$ cd <hub>/fixtask && lyx loom start --no-attach
  → {"error":"shedrun: run \"self\" is already seeded with {Recipe:loom Driver:llm Params:map[]};
     refusing to overwrite with disagreeing seed {Recipe:loom Driver:llm Params:map[parent:main]}"}
```

**Focus item 3 — CONFIRMED GOOD.** The `Run-Shed` hard error left `state: "failed"` at `current_producer: "Run-Shed"`, and `<hub>/fixtask` + `<hub>/fixtask-weft` were both still on disk afterwards. The run did not route to `Worktree-Teardown`; the destructive row stayed structurally unreachable from the failure path, exactly as the recipe header claims.

**Focus item 4 — ANSWERED.** `Spawn` runs `lyx loom start --no-attach`, which returns after a bounded bootstrap handshake, never for the campaign's duration: `driver: go` waits at most 300 × 100 ms = 30 s for the run-lock handshake while the runner itself is `cmd.Start()` + `proc.Detach` (never waited); `driver: llm` waits at most 50 × 100 ms = 5 s on the pane-liveness probe, `Runner.Start` itself returning a handle without blocking. `lyx batten step`'s first advancing call therefore cannot hang for a whole loom campaign. Traced through the real wiring, and consistent with the observed sub-second return of the failing spawn above.

### Live scenario 4 — hand-written seed ahead of batten's auto-seed (focus item 11)

```
$ lyx shed seed handseed --recipe loom --param parent=main
  {"driver":"go","ok":true,"recipe":"loom","run_id":"handseed"}
$ lyx batten step handseed
  → producer Worktree-Create, outcome done, next Seed-Child        ❌ F3: silently adopted
$ cat <prime>/_lyx/shed/handseed/seed.json
  {"recipe":"loom","driver":"go","params":{"parent":"main"}}       ← untouched (good half)
$ lyx shed status handseed
  {... "parent":"", "slug":"", "interrupt_policy":"" ...}          ← loom's envelope shape, armed
                                                                      against prime's batten status
```

Half the property holds (the hand-written seed is authoritative and is not overwritten); the other half does not (a genuinely disagreeing one is not refused). The same run-id is now driven by batten under `lyx batten` and by loom under `lyx shed`, reading the same status file through two different recipes' lenses. **F3.**

### Live scenario 5 — `--child-driver` on a re-run (F4)

`lyx batten step fixtask --child-driver go` against the already-seeded `fixtask` produced no message about the flag and left the child's seed at `driver: llm`. Silent no-op. **F4.**

### Live scenario 6 — cold machine / mid-operation orphans (focus item 7)

**(a) Crash between `Worktree-Create` succeeding and its status persist** — emulated by rewinding prime's status to `Worktree-Create`/`running` after the row really created the pair, then re-driving:

```
$ lyx batten step crashcreate
  WARN battenshed: producer stuck  producer=Worktree-Create
       reason="branch \"crashcreate\" already exists; switch a pair onto it with
               \"lyx fabric checkout crashcreate\", or delete it first with
               \"git branch -D crashcreate\" if it is a leftover from a removed pair"
  → outcome stuck, state blocked, continue false
```

**No double-create.** The run halts `blocked` with a real remedy in `.lyx/shed/crashcreate/Worktree-Create-stuck.md`. **OK (F14).**
But `lyx batten status crashcreate` reported only `"error":"stuck with no OnStuck target"` — the remedy is invisible through the status verb. **F10.**

**(b) Crash between `Seed-Child`'s commit and its push** — covered by construction rather than by driving: `shedrun.WriteSeed` is idempotent against a byte-identical seed and `PushSeed`'s failure only warns (`seamchild.go:104-106`), so a resumed `Seed-Child` re-writes the same bytes, re-commits a no-op, and re-pushes. No double-seed is structurally possible. Not separately driven.

**(c) Cold machine** — emulated by `lyx fabric remove handseed --force`, leaving prime's durable batten status pointing past `Worktree-Create`:

```
$ lyx batten step handseed
{"error":"battenshed: Seed-Child: write seed: not a git repository:
  chdir .../handseed: no such file or directory","kind":"producer","ok":false}
```

No self-heal, and no honest report of what state the run is in. **F9.**

### What could NOT be verified before the fixes

These scenarios are structurally unreachable behind F0/F1 and are driven after the fixes land (results appended to this file as a post-fix addendum):

- The full real end-to-end drive with a real provider spawned inside the child's reed session (`--child-driver llm`), through to a terminal state and teardown.
- Focus item 2 — interrupting a real teardown mid-flight, and a worktree whose session shutdown failed never being removed.
- Focus item 6 — `PrimeRunLock` scope: two different slugs' create/teardown serializing against each other while one slug's long `Run-Shed` watch blocks neither.
- Focus items 8 and 9 — the two sabotage scenarios (a raw git commit landed on weft mid-run; a stray untracked file under `_lyx` mid-run).
- Focus item 10's residual (a) in motion — a driver strand dying mid-watch.

### Explicitly not touched

- **Windows path behaviour** anywhere in this stack. Unreachable from this Linux host; not exercised, and not newly flagged as missing.
- **fabric's own clone/merge correctness** beyond the pair create/remove batten's two bookend rows actually drive.
- **loom's own internal phase-machine correctness**, except where it corrupts or mis-reports through the status/seed boundary batten depends on — which F1 does, and is why F1 is recorded here rather than deferred to loom.
- **No `N×`-concurrent smoke-suite amplifier** was attempted, per this round's own cost declaration.

---

# Post-fix addendum

Written after the fixes landed, per the round prompt's instruction to re-run every live scenario.
Everything below was driven on the same disposable fixture hub, against the redeployed binary.

## F15 — prime's own run seed is never committed (MEDIUM, CONFIRMED live, found post-fix)

`internal/battencli/commitstatus.go:56` (as it stood).

Surfaced while running the sabotage scenarios: `git status` on prime's weft showed `?? _lyx/shed/<slug>/seed.json` for every slug batten had ever run, while `_lyx/shed/<slug>/status.json` was tracked.

The CommitStatus seam committed `shedrun.StatusRel(runID)` and nothing else, so prime's own seed stayed untracked forever.

That contradicts the whole reason the design doc put batten's run state in the durable tree:

> "a worktree recreated on another machine (the mill-resume pattern) must still know what it is running"

A resumed machine gets the status and not the seed, so it can read how far the run came but not what it is running.
Worse, it is not merely a read failure: `armSeed` finds no seed, takes the auto-seed arm, and re-seeds from flag defaults — so a run seeded `--child-driver llm` comes back as `child_driver: go`, exactly the silent divergence between "what a run was seeded as" and "what it is actually doing" that the Driver Choice Single-Site Invariant exists to prevent.

Fixed by committing the seed alongside the status on every transition, included only when present (a pathspec matching no file is a hard `git add` error, and a Shed driven outside batten's own CLI verbs has no seed).
Verified live afterwards: `_lyx/shed/goldrun/seed.json` is tracked at HEAD on prime's weft.

## Live re-drive — results

### The path that never worked, now walked end to end

```
$ lyx batten step goldrun --child-driver llm   → Worktree-Create done
$ lyx batten step goldrun                      → Seed-Child     done
  child seed: {"recipe":"loom","driver":"llm","params":{"parent":"main"}}
  recorded origin: {"parent_branch":"main"}                     ← agrees, so the child bootstrap accepts it
$ lyx batten step goldrun                       → Run-Shed stuck/running, self-bounced
```

`Run-Shed` advanced past its spawn for the first time.
Inside the child, `lyx reed status` reported a live session with two live strands:

```
{"session":"goldrun","strands":[{"name":"loom-status","live":true},{"name":"loom-driver","live":true}]}
```

**Focus item 5 fully confirmed.** The Board task's `type: loom` reached the child's own `seed.json` as `recipe: loom`, prime's `params.child_driver: llm` reached it as `driver: llm`, neither silently defaulted, and the nested `ly-drive` loop spawned a real `claude` provider off that value in the child's own reed session.

One environment note, not a batten defect: the provider first stopped at Claude Code's folder-trust prompt, because the disposable hub lives in a directory it has never seen. Answered by hand for this fixture, after which the driver ran for real.

### Focus item 3 — a genuinely blocked child, with a real child

The child later reached `Preflight blocked` (loom's own precondition row — out of batten's scope).
`Run-Shed` then returned a hard error, not `Stuck`:

```
{"error":"battenshed: Run-Shed: inner shed run reached state \"blocked\": error=\"stuck with no OnStuck target\" current_producer=\"Preflight\"","kind":"producer"}
```

and both `goldrun` and `goldrun-weft` were still on disk afterwards. The destructive row stayed unreachable from the failure path.

### Focus item 6 — `PrimeRunLock` scope, both halves

- With `goldrun` mid-`Run-Shed`, a second slug's `Worktree-Create` completed in 0s. A long watch holds no prime lock.
- Holding `.lyx/shed/run.lock` externally, a third slug's `Worktree-Create` returned `stuck`/`blocked`, its reason naming the lock path, and **no pair was created**. Released, the same create succeeded.

### Focus item 2 — teardown ordering

- With the child's `reed.yaml` made unparseable so `Shutdown` fails: `Worktree-Teardown` returned `stuck`/`blocked`, the reason named session shutdown as the failed half, and **both pair worktrees were still present** — `Remove` was never called.
- Ordinary teardown: `done`, `continue: false`, and both worktrees gone.

### Focus items 8 and 9 — sabotage, both PASS

- **A raw git commit landed directly on weft outside fabric's seams, mid-run.** batten's tracking was not corrupted: subsequent status commits stacked cleanly on top of the foreign commit, and the run kept advancing.
- **A stray untracked file dropped under `_lyx` mid-run.** No stage-all lottery: `_lyx/STRAY.txt` was neither swept into a commit nor removed. batten's commits are positive-only pathspecs per the Fabric Git Invariant, so the stray stayed untracked on disk and visible in `git status` — the honest outcome, without needing a report of its own.

### F10 verified live on a genuinely blocked run

```
$ lyx batten status crashcreate
state:        blocked
error:        stuck with no OnStuck target
stuck_reason: branch "crashcreate" already exists; switch a pair onto it with "lyx fabric checkout crashcreate" ...
```

### Focus item 10 residuals — re-verified, both still accepted

Unchanged from the scope assessment above. The dead-strand residual is now written down in `battenshed`'s own package doc, which is the part an operator was paying for.

## What could NOT be verified, and why

- **A loom campaign driven to a completed, successful terminal state, followed by an automatic `Run-Shed → Done → Worktree-Teardown`.** The child blocked at loom's own `Preflight` row, which this round's scope places outside batten. The teardown row itself was driven directly instead, both its success and its failure path (focus item 2 above), and `Run-Shed`'s `Done` arm is covered by the integration suite.
- **Windows path behaviour** anywhere in this stack — unreachable from this Linux host. Not exercised, and not flagged as missing.
- **The `N×`-concurrent smoke-suite amplifier** — excluded by this round's own cost declaration, never attempted.

## Teardown discipline

- Child reed session shut down, then its tmux server killed, then its `reed watchdog` process killed.
- `ls /tmp/tmux-1000/` is empty — zero tmux sockets, no default server.
- `pgrep -af fixhub` returns nothing. The two remaining `claude` processes on the host are the pre-existing ones present before this round began.
- The disposable fixture hub is fully deleted. Only the small driver scripts remain in this session's scratchpad.
- `/home/hanf/Code/lyx-test-LYXHUB` does not exist on this host, so the operator's standing bench was never touched. All work happened inside the scratchpad fixture.
