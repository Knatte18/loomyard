# batten — independent review, round `fable-high-r2`

> Clean-room review of `internal/battenshed`, `internal/battenrecipe`, `internal/battencli`, `contracts/recipes/batten-recipe.yaml` against the recovered SPEC (`git show 8ac857ce1~1:manifest/designs/seeded-shed.md`).
> Written incrementally per the round prompt's "log as you go" rule; the executive summary is filled last.
> Reviewer: Fable 5.1, high effort. Worktree `/home/hanf/Code/loomyard/wts/crucible-batten-end-to-end`, branch `crucible-batten-end-to-end`, starting HEAD `fe161633d`.

## Executive summary

_(filled at the end of Job 1)_

## Merge bar

Correctness in the NORMAL single-instance flow: one slug driven start to finish from prime, including the SUCCESS terminal state (real child `StateDone` → `Run-Shed` `Done` → `Worktree-Teardown` fires and tears down).
Item 6 (two different slugs interleaved) is an in-scope concurrency check, not the N×-concurrent amplifier gate, which does not apply here.

## Scope assessment (plan vs shipped)

_(filled after the code pass)_

## Code findings (severity-ranked; provisional until the end of Job 1)

### Provisional findings from the code pass

- **P-A** `internal/battencli/arm.go:376-379,443-446` — the done-slug refusal names the WRONG directory as its remedy. `"%q has already completed; delete %s to run it again"` interpolates `BattenDir(c.location, c.slug)` = `shedrun.ScratchDir` = the EPHEMERAL `.lyx/shed/<slug>/`, but the status file the refusal is gating on is the DURABLE `_lyx/shed/<slug>/status.json`. An operator following the remedy verbatim deletes the scratch dir and is refused again identically. CONFIRMED by reading; to be reproduced live after the full drive reaches `done`. Severity: MEDIUM (an operator-facing remedy that does not work). Fix: name the durable run directory (`shedrun.RunDir`) and say that removing it is a weft change.
- **P-B** `internal/battencli/wire.go:212` — `Spawn` hardcodes `exe loom start --no-attach` for every child, while `WriteSeed` (`wire.go:267`) accepts ANY registered recipe name for the child (`shedrun.ValidateRecipe`), so a Board task with `type: batten` seeds the child `recipe: batten`, commits and pushes that seed onto the child's branch, and then `Run-Shed` runs `lyx loom start` in it — whose own `seedAndCommitBootstrap` finds a disagreeing seed (`batten` vs `loom`) and refuses. Result: `Run-Shed` hard-errors → `StateFailed`, with a message about a seed disagreement inside the child rather than "batten cannot bootstrap a `batten` child". The SPEC says Run-Shed spawns "per the child's seed"; as built it can only bootstrap `loom`. Severity: LOW (loud, not silent; but late — after a worktree exists and a seed is already committed+pushed — and misattributed). CONFIRMED by reading; to be driven live (cheap, no LLM). Fix: refuse at Seed-Child, before any commit, wrapping `ErrUnknownRecipe` so the row lands `Stuck` with a reason naming the recipe and that only a recipe with a bootstrap verb (`loom`) can be a batten child.
- **P-C** `internal/battenshed/deps.go:63-65`, `innerrun.go:50-52` — `InnerRunDeps.Now` is declared, defaulted to `time.Now` in `NewInnerRun`, and documented as "a test holds the clock still by setting it", yet no production code path reads it (`grep -n "deps.Now" internal/battenshed/*.go` finds only the nil-default). Dead seam with a misleading contract. Severity: NIT. CONFIRMED.
- **P-D** `internal/battenshed/doc.go:1-3`, `ctx.go:1`, `stuck.go:1` — package doc says "the three task-worktree batten producers: creating the task worktree, running the loom session inside it, and tearing it down"; `ctx.go`/`stuck.go` headers say "this package's three producers" / "all three producers". There are four (Seed-Child is omitted everywhere). Severity: NIT. CONFIRMED.
- **P-E** `internal/battencli/cli.go:150-162` — the `batten` group `Long` lists three verbs ("run" / "status" / "pause") and says "All three verbs run from the hub's prime worktree only"; `step` is the fourth and is missing from both sentences. Severity: NIT. CONFIRMED.
- **P-F** `internal/battencli/cli.go:94-98` — `status`'s `Long` says "A slug that has never been run on this machine is reported as a determined answer on the success envelope, not as an error", but `armSeed` (`arm.go:180-185`) refuses `status` with the missing-seed listing BEFORE the verb ever runs when no seed exists; the `found: false` envelope is reachable only in the narrow seed-present/status-absent window. Severity: NIT/LOW (docs vs behaviour). PLAUSIBLE — to verify live.
- **P-G** `internal/battencli/arm.go:203-205` — the auto-seed writes `params.slug: <runID>`, but nothing anywhere reads `params["slug"]` (`grep` over `battencli`, `battenshed`, `shedcli`: zero readers). Write-only param; a hand-written `lyx shed seed <slug> --recipe batten` (F22's own scenario) carries no such param and behaves identically, which proves it decorative. Severity: NIT. CONFIRMED.

### Open questions to settle live

- **Q1** Teardown never forces (`top.Remove(..., force=false, ...)`), and fabric's dirtiness probe is `scopeAll` (untracked files included) on BOTH warp and weft. Does a real child that finished a loom campaign under `driver: llm` leave anything untracked behind (the ly-drive skill writes `.scratch/ly-drive/step-<n>.json` relative to the task worktree, and fabric excludes only `_lyx`/`.lyx`, never `.scratch`)? If so, `Worktree-Teardown` blocks on every real llm-driven success. Verify on the primary drive.
- **Q2** Focus 4: how long does the first advancing `InnerRun.Call` block? By reading (`loomcli/start.go`), `loom start --no-attach` returns after the go arm's run-lock handshake or the llm arm's pane-liveness probe — a bootstrap, not the campaign. Time it live.
- **Q3** Focus 6: two slugs' Run-Shed polls both commit prime's weft only on real transitions (the no-op marker skip), so the index.lock collision window is narrow; drive interleaved anyway.
- **Q4** Focus 7/8/9: mid-operation kill points and the two sabotage scenarios.
- **Q5** `reedengine.Down()` on a child whose reed session never existed (a `go`-driven child, or one whose driver already exited and whose server is gone) — is it idempotent (Stuck-free) at teardown time?

## Docs & operability findings

_(filled as found)_

## What was tested (exact commands + observed results, appended as each returns)

### Hermetic baseline (before any change)

```
go build ./...
go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...
go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...
go test -tags integration -count=1 ./internal/battencli/...
```
All green at HEAD `fe161633d` (`ok` for battenshed, battencli, battenrecipe, cmd/lyx; integration battencli `ok 3.461s`).

### Host baseline

- `pgrep -fl "tmux|claude"` before any live driving: two pre-existing `claude` processes (pids 1506, 47275 — the orchestrator session and this one), NO tmux server. These two pids are the baseline the end-of-round stray-process check compares against.
- The operator's standing `lyx-test-LYXHUB` bench is not under `~/Code` on this host and was never touched.
- Dev binary under test: `/home/hanf/Code/loomyard/wts/crucible-batten-end-to-end/.dev-bin/lyx` (deployed via `./deploy-dev`); every live command below invokes it by absolute path.

## Deferred items from the prior round (re-evaluated after my own pass)

_(filled after the clean-room pass; prior-round material is read only then)_

## What I could NOT verify and why

_(filled at the end)_

### A1 — refusal surface on prime (no worktree created)

From prime (`<hub>/warp`), dev binary by absolute path:
- `lyx batten status nope` → `{"error":"battencli: no seed found for run \"nope\"; no run is seeded yet. run \"lyx batten run <slug>\" first"}` exit 1. Same for `pause nope`. Nothing written under `_lyx/shed`.
- `lyx batten run` (no slug) → "no slug given; ... pass the slug" exit 1. `lyx batten step self` → "slug \"self\" is reserved for addressing prime's own run" exit 1.
All four correct and honest.

### A2 — focus 11: hand-seeded prime run (`lyx shed seed <slug> --recipe batten`)

- `lyx shed seed pre-seeded --recipe batten` → writes `_lyx/shed/pre-seeded/seed.json` `{recipe: batten, driver: go}` (no `params.slug`, unlike batten's own auto-seed — see P-G).
- `lyx batten run pre-seeded --driver llm` → refused: "already seeded with driver \"go\"; --driver \"llm\" cannot change a seeded run's recorded driver". `--child-driver llm` → refused analogously. Seed byte-identical afterwards. Hand seed treated as authoritative; a disagreeing flag refused rather than adopted. CORRECT.
- `lyx shed seed wrong-recipe --recipe loom` then `lyx batten run wrong-recipe` → refused: "already seeded with recipe \"loom\", not \"batten\"; batten refuses to drive it -- drive it with \"lyx shed run wrong-recipe\"". CORRECT.
- **DEFECT (F1)** `lyx batten status pre-seeded` (seed present, status absent — the ONLY state in which the documented `found: false` "determined answer" is reachable, and exactly F22's own hand-seed scenario) → `{"error":"battencli: decode status file .../warp/_lyx/shed/pre-seeded/status.json: acquire read lock: acquire read lock: open .../warp/.lyx/shed/pre-seeded/status.json.lock: no such file or directory"}` exit 1. `lyx shed status pre-seeded` → identical. Root cause: `specFor` sets `EnsureStatusLockDir: false` (`arm.go:293`) so the generic status body never creates the ephemeral lock directory, and `state.ReadJSONStrict` needs the lock file's parent to exist even to report "absent". This also breaks the cold-machine design: a durable status synced to another machine has NO `.lyx/shed/<slug>/` there, so `lyx batten status <slug>` errors on the fresh clone instead of reading the very status file the SPEC made durable for that purpose (to be re-proven below by deleting the scratch dir after a status exists). `pause` shares the flag and the failure.

### A3 — P-B confirmed live: Board `type: batten`

Board task `bad-type` with `"type":"batten"`. `lyx batten step bad-type` ×3 from prime:
1. `Worktree-Create` → done (0.12 s real). Pair `bad-type`/`bad-type-weft` created.
2. `Seed-Child` → done. Child `_lyx/shed/self/seed.json` = `{recipe: batten, driver: go}`, committed (`6fe2e36 batten: seed child bad-type`) and pushed onto the child's weft branch.
3. `Run-Shed` → `{"error":"battenshed: Run-Shed: spawn inner shed run: exit status 1: {\"error\":\"shedrun: run \\\"self\\\" is already seeded with {Recipe:batten Driver:go Params:map[]}; refusing to overwrite with disagreeing seed {Recipe:loom Driver:go Params:map[parent:main]}\",\"ok\":false}","kind":"producer"}`; prime status `state: failed` at `Run-Shed`.
Outcome as predicted: loud but late (a pair exists and a wrong seed is already committed+pushed) and misattributed (a `shedrun` disagreement raised by `lyx loom start` inside the child, not "batten can only bootstrap a `loom` child"). The `kind: producer` classification also means an ly-drive-style supervisor would retry this once for nothing. **F2 = P-B, severity LOW → raised to MEDIUM** on the strength of the committed-and-pushed wrong seed: recovery needs `lyx fabric remove bad-type`, deleting the branch, and hand-removing prime's durable `_lyx/shed/bad-type/`, none of which the envelope names.
