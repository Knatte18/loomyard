# batten — independent review (round 4, tag `opus-medium-r4`)

> Clean-room round: findings below were formed from the SPEC (`git show 8ac857ce1~1:manifest/designs/seeded-shed.md`), the module code, the docs, and live driving of a disposable fixture hub — before any prior round's review material was opened.

**Status: in progress.** Sections are appended as each scenario returns.

## Executive summary

_(written last)_

## Scope assessment — plan vs shipped

_(written last)_

## Findings

_(appended as they are found)_

## Docs & operability findings

_(appended as they are found)_

## What was tested

### Hermetic (all green, before any change)

- `go build ./...` — clean.
- `go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...` — clean.
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...` — all `ok`.
- `go test -tags integration -count=1 ./internal/battencli/...` — `ok`, 2.504s.

### Fixture hub

Disposable hub built outside both the loomyard tree and `$HOME/Code`, under this session's scratchpad:
`.../scratchpad/hub` holding `remotes/bfix.git` + `remotes/bfix-weft.git` (bare, hand-seeded minimal Go project: `go.mod`, `main.go`, `main_test.go`, `README.md`) and the cloned hub `bfix-LYXHUB` (`bfix` warp prime on `main`, `bfix-weft` on `main-weft`, `_board`).

- `lyx fabric clone --into <scratch> <weft.git> <warp.git>` — `ok: true`, `partial: false`, 22 mutations, warp binding recorded.
- **GitHub self-report hazard, discharged first:** `selfreport: false` committed and pushed onto `main-weft` (`f4807bd fixture: selfreport off`) BEFORE any Board task existed and before any `Worktree-Create`.

### Live scenario A — argument, driver and seed refusals (all from prime)

| Command | Observed |
| --- | --- |
| `lyx batten run` | `battencli: no slug given; ... pass the slug: "lyx batten run <slug>"` |
| `lyx batten run self` | `slug "self" is reserved for addressing prime's own run` |
| `lyx batten status add-farewell` (unseeded) | `no seed found for run "add-farewell"; no run is seeded yet. run "lyx batten run <slug>" first` |
| `lyx batten pause add-farewell` (unseeded) | same refusal |
| `lyx batten run add-farewell --driver llm` | `batten has no bootstrap verb, so it cannot be driven by an LLM` |
| `lyx batten run add-farewell --driver bogus` | `shedrun: unknown driver "bogus"; must be "go" or "llm"` |
| `lyx batten run add-farewell --child-driver bogus` | same shedrun refusal |

After all seven refusals, `_lyx/shed/` did not exist in prime: **no refused invocation leaves a seed behind.**

### Live scenario B — the Batten Bookend Invariant (focus item 1) — CONFIRMED NON-DEFECT

Drove all four verbs (`run`/`step`/`status`/`pause`), each against both its own slug and a different slug, from
(a) the freshly-created task worktree `bad-type` and (b) its weft sibling `bad-type-weft`. 16/16 refused.

- From the task worktree: `battencli: this verb runs from the hub's prime worktree only; "bad-type" is not the prime worktree ("bfix" is) -- re-run it from there` — both worktree names present, remedy stated.
- From the weft sibling: `RequireWarpWorktree` fires first — `... is the weft sibling of a pair, not a warp worktree; run lyx from the paired warp worktree instead`.
- From `_board`: `... is the hub's _board checkout, not a warp worktree`.
- From a task-worktree SUBDIRECTORY (`bad-type/sub`): refused by the anchor gate before batten is reached.
- Bare `lyx batten` still lists subcommands from a task worktree (the deliberate `cmd.Name() == "batten"` short-circuit) — a listing, not a drive.

### Live scenario C — Seed-Child recipe refusals (focus item 5, negative half)

Board task `bad-type` (`type: batten`) and `unknown-type` (`type: nonsense`), each `lyx batten step`ed twice:

- step 1: `Worktree-Create → done` (0.09–0.11s, no poll sleep on this row).
- step 2: `Seed-Child → stuck`, `state: blocked`, and `lyx batten status` carried the producer-supplied `stuck_reason`:
  - `bad-type`: `Board task type "batten" cannot be the task worktree's own run: ... a Board task's type must be "loom" or empty; got "batten"`
  - `unknown-type`: `unknown recipe name "nonsense": ... available recipes: [batten loom]`
- **Both task worktree pairs were left intact** — the failure path never routes to the destructive teardown row (focus item 3's structural half).
- No child seed was written in either worktree (refusal is ahead of the write).

### Live scenario D — hand-written seed ahead of the auto-seed (focus item 11) — CONFIRMED NON-DEFECT

1. `lyx shed seed add-shout --recipe loom` → `lyx batten run/status add-shout` refuses: *"already seeded with recipe "loom", not "batten"; batten refuses to drive it -- drive it with "lyx shed run add-shout", or delete its seed to re-seed it as a batten run"*. The hand-seed was **not** overwritten.
2. `lyx shed seed add-shout --recipe batten --driver go` → `lyx batten status add-shout` answers `{"found":false,"ok":true,...}` (a determined answer, not an error); `lyx batten run add-shout --child-driver llm` refuses (`already seeded with child driver "go"`); `lyx batten run add-shout --driver llm` refuses (`already seeded with driver "go"`). Seed bytes unchanged after every refusal.
3. `lyx batten step add-shout` with no flag typed **adopts** the hand-seed and advances — authoritative, not overwritten, not silently contradicted.

### Live scenario E — PrimeRunLock contention (focus item 6, half A) — CONFIRMED NON-DEFECT

Held `bfix/.lyx/shed/run.lock` with `flock -x ... sleep 20` (the same advisory lock `internal/lock` takes), then:

- A **different** slug's `lyx batten step add-shout` (its `Worktree-Create` row) → `Worktree-Create → stuck`, `state: blocked`, `stuck_reason: prime lock "..." is already held; another batten producer is creating or tearing down a task worktree`. Deterministic, not raced.
- Read-only `lyx batten status bad-type` was **unaffected** by the held lock, as it must be.
- After the holder exited, the identical `lyx batten step add-shout` advanced: `Worktree-Create → done`.

_(more live scenarios appended below as they run)_
