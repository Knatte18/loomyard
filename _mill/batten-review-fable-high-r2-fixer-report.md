# batten — fixer report, round fable-high-r2

Companion to `_mill/batten-review-fable-high-r2.md`. One commit per fix, on `crucible-batten-followup`, nothing pushed.

## Implemented

| Finding | Severity | Commit | What changed | Regression test (fails without the fix) |
|---|---|---|---|---|
| F1 — teardown reports `done` over a half-torn pair | MEDIUM | `37e4b4788` | `fabricengine.PairSiblingRemnant` (vocabulary-neutral: "is the pair's other side still on disk, and where"); `battencli/wire.go` `Teardown.Remove` refuses when the task worktree is gone but the sibling remains, naming the leftover path and `lyx fabric prune --apply`; Shutdown still skips (no worktree to resolve reed's config from). `docs/overview.md` teardown sentence now speaks of the pair. F22 gained the half-torn scenario. | `TestWire_TeardownRefusesAHalfTornPair` (`wire_test.go`): failed at "Remove() = nil; want a refusal" before the fix. Live: blocked with the remedy, `lyx fabric prune --apply` removed the orphans, re-step → `done`. |
| F2 — create row's verbatim fabric remedy switches prime | MEDIUM | `85bf53198` | `fabricengine.Add`'s re-add refusal is a typed `*ErrBranchExists` (same text); `battencli` `createRefusal` recognises it and words the abandon path — delete the branch locally and on the remote, `lyx fabric cleanup --apply --remote` for the orphaned sibling branch, never `lyx fabric checkout` from here. Overview gained the sentence; F22 gained the re-run scenario. `battenshed`'s verbatim-pass-through test still holds: the producer passes through whatever the closure returns. | `TestCreateRefusal_LeftoverBranchRemedyNeverNamesCheckout` (`wire_test.go`): sabotaged to a passthrough → fails on the missing remote-delete and cleanup wording. Live: the new `stuck_reason`, then following it (branch -D, push --delete, cleanup --apply --remote) let `Worktree-Create` succeed. |
| F4 — done-slug remedy incomplete | NIT | `d79829eb6` | One `doneSlugRefusal` for both pre-run hooks, naming the run directory and the torn-down pair's branches, local and remote; `run`'s Long text and F22's done-remedy line updated. | `TestBattenPreStep_DoneSlug_KindBootstrap` and `TestRunCmd_StateDoneRefusesNamingTheRunDir` extended: both failed on "want it to contain \"local and remote\"" before the fix. Live: the new text on `lyx batten run fx-untyped`. |
| F3 — `lyx shed seed --recipe batten` accepted from a task worktree | LOW | `eefd0dc1f` | `battencli.RefuseUnlessPrime` (the guard `armAt` already applied, now one exported call); shedcli's table entry carries `RefuseSeedAt`, batten's from that guard, loom's nil; `writeSeed` consults it after fabric's drivable-worktree check. Three Tier 1 seed tests switched to loom seeds, since batten's rule reaches a git worktree listing. Overview's `lyx shed seed` sentence and F22 updated. | `TestWriteSeed_RecipeLocationRuleGatesTheWrite` (Tier 1, predicate) and `TestWriteSeed_BattenSeedIsRefusedOutsidePrime` (integration, real hub): sabotaged the rule to never be consulted → both fail. `TestRecipes_SeedLocationRuleMatchesTheRecipesOwnVerbs` pins which entries carry the rule. Live: refused from `fx-typed` with batten's prime-only wording, no run dir left; loom seed there and batten seed in prime both ok. |
| F5 — teardown can kill loom's post-run friction reflection | LOW | `944d3cfc6` (docs half only, see below) | `internal/battenshed/doc.go`'s residual paragraph states that the whole-worktree teardown ends whatever the driver is still doing after the child's run reached its terminal state, post-run bookkeeping included. | none (docs) |

## Deferred

- **F5's behavioural half — NOT-FIXED-THIS-ROUND (operator decision on a design tradeoff).** Making `Run-Shed` wait for loom's post-run friction reflection (or `Worktree-Teardown` wait for the driver process) contradicts the shipped residual "the row watches the child's persisted status file, never the driver's own liveness", and the reflection's own lock (`loomengine.LoomFrictionLock`) is a loom-internal path batten would have to be told. The cost today: with `friction` on (the shipped default), every batten-driven task loses its Tier 2 report and records an `abandonedSession`. Options for the operator: (a) accept and keep the residual; (b) have loom persist `done` only after its post-run bookkeeping, so batten's status read is already late enough; (c) give batten a told "driver still busy" probe. (b) is the smallest and keeps batten's own contract intact.

## Test commands and results

After every fix, repo-wide: `go vet ./... && go test -count=1 ./...` → exit 0, 92 packages ok (`f1-full.log`, `f2-full.log`, `f4-full.log`, `f3-full.log` in the session scratchpad). After F3 also `go test -tags integration -count=1 ./internal/battencli/... ./internal/shedcli/...` → ok.

Final gates at the round's last fix commit `944d3cfc6` (`final-gates.log`), exit 0:

- `go build ./... && go vet ./... && go test -count=1 ./...` — 92 packages ok
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./internal/shedrun/... ./cmd/lyx/...` — ok
- `go test -tags integration -count=1 ./internal/battencli/... ./internal/shedcli/...` — ok

Sabotage proofs recorded per row above were run by this round itself (fix neutralised, test failed at the intended assertion, fix restored to a byte-identical file) before each commit.

Live re-verification after each redeploy (`./deploy-dev`): recorded per row above; every scenario driven against the fixture hub described in the review report.

## Changed files

- `internal/fabricengine/fabric.go` (+`PairSiblingRemnant`), `internal/fabricengine/add.go` (+`ErrBranchExists`)
- `internal/battencli/wire.go`, `wire_test.go`, `arm.go`, `refusal.go`, `cli.go`, `step_test.go`, `run_test.go`
- `internal/shedcli/table.go`, `seed.go`, `seed_test.go`, `table_test.go`, `seed_integration_test.go` (new)
- `internal/battenshed/doc.go`
- `docs/overview.md`, `tools/sandbox/SANDBOX-FABRIC-SUITE.md` (F22)
- `_mill/batten-review-fable-high-r2.md`, this file

No change to `CONSTRAINTS.md` (no invariant moved) and none to `manifest/roadmap.md`.
