# Batch: webstercli-rewire

```yaml
task: 'webster: rewrite for flat card list'
batch: webstercli-rewire
number: 9
cards: 4
verify: go test -tags integration ./internal/webstercli/...
depends-on: [2, 3, 7, 8]
```

## Batch Scope

Rewire `internal/webstercli` off `internal/builderengine` and onto `internal/planparser`,
`internal/batcher`, and the retargeted `internal/websterengine` — the last import edge to cut.
The eight CLI verbs (`validate`/`run`/`status`/`pause`/`begin-batch`/`await-batch`/`record-batch`/
`recover-batch`) and their `Short` strings stay UNCHANGED so the help-tree pins hold (only the
now-meaningless `--restart-chain` flag on `begin-batch` is removed). The `PersistentPreRunE`
resolution chain is preserved; the one addition is load-time batcher selection/validation via
`batcher.Select(cfg.Batcher)`. On completion the whole `internal/webster*` surface imports zero
`builderengine` symbols.

## Cards

### Card 39: cli.go seam + batcher config wiring

- **Context:**
  - `internal/batcher/registry.go`
  - `internal/websterengine/strand.go`
  - `internal/websterengine/config.go`
  - `internal/shuttleengine/mux.go`
  - `internal/shuttleengine/engine.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/webstercli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cli.go` retarget the `websterCLI` seam field types and helpers off `builderengine`: `starter builderengine.Starter` → `websterengine.Starter`; the `builderengine.OrchestratorStarter`/`StrandLive`/`TurnEnded` references (cli.go:14/53/59/65 and comments) → their `websterengine` equivalents (the `runnerMasterStarter` adapter and the `c.starter = runner` / `c.injector = runner` / `c.masterStarter = ...` assignments still hold because `*shuttleengine.Runner` structurally satisfies the webster-local `Starter`/`OrchestratorStarter`). In the `PersistentPreRunE` closure, after `websterengine.LoadConfig(layout.Cwd, "webster")` returns `websterCfg`, resolve and validate the active batcher via `batcher.Select(websterCfg.Batcher)` — surface an unknown-name error here (fail-fast, before any verb runs) as an `output.Err` envelope; store the resolved batcher (or the validated name) on `websterCLI` for the verbs/run to use. Keep the rest of the resolution chain (`hubgeometry.Resolve` → shuttle/mux/webster cfg → `ResolveRoles` → engines → `shuttleengine.NewRunner` → the four `hubgeometry` dirs) intact. Re-audit the parent `webster` command's `Use`/`Long` (its embedded `Verbs:` help block): the eight verb names are unchanged, but strip any v2 language (chain/oversized/`--restart-chain`) from the parent `Long` prose if present, per the CLI/Cobra Invariant's help-accuracy obligation.
- **Commit:** `refactor(webstercli): retarget cli seam to websterengine, wire batcher selection`

### Card 40: verb parse/validate/digest retarget + drop --restart-chain

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/validate.go`
  - `internal/websterengine/digest.go`
  - `internal/websterengine/report.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/webstercli/awaitbatch.go`
  - `internal/webstercli/beginbatch.go`
  - `internal/webstercli/recordbatch.go`
  - `internal/webstercli/recoverbatch.go`
  - `internal/webstercli/validate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget every `builderengine.ParsePlan(c.planDir)` in `awaitbatch.go`/`beginbatch.go`/`recordbatch.go`/`recoverbatch.go`/`validate.go` to `planparser.ParsePlan(c.planDir)` (with `c.planDir` still supplied from `hubgeometry.PlanDir(layout.Cwd)`). In `validate.go` retarget `builderengine.Validate(plan, c.layout.Cwd, caps)` → `planparser.Validate(plan, c.layout.Cwd)` (DROP the `builderengine.ValidateCaps{...}` construction — no oversized) and the `findingsEnvelope(..., []builderengine.ValidationError)` signature → `[]planparser.ValidationError`. **Pin the changed observable JSON keys deterministically** (card 42's tests assert them): the ok-envelope count field emits `len(plan.Cards)` (was `len(plan.Batches)` — `planparser.Plan` has `Cards`, no `Batches`; rename the JSON key from `batches` to `cards`), and each finding entry emits the finding's `card` identifier from `f.Card` (was `batch`/`f.Batch` — `planparser.ValidationError` has `Card`, no `Batch`; rename the JSON key from `batch` to `card`). In `recordbatch.go` retarget `digestFields(d builderengine.Digest)` → `websterengine.Digest`. In `beginbatch.go` REMOVE the `--restart-chain` flag registration and update the `websterengine.BeginBatch(...)` call to the new signature without the `restartChain` argument (chain machinery deleted in batch 7). **Rewrite the affected verb `Long` help strings for the flat card-list model** (CLI/Cobra Invariant — stale help is a review-blocking defect): drop every mention of `--restart-chain`, the deferred-verify chain, oversized batches/`oversized:` frontmatter, and "plan-format v2"/"Batch Index"/"chain-end soundness" from `begin-batch`'s and `validate.go`'s `Long`; describe the flat card-list validation (the 14 planparser checks) and the batchifier-derived batch model instead. Re-audit ALL eight verbs' `Long` strings (and the parent `webster` command's `Long` `Verbs:` block in `cli.go`, edited in card 39) so none describe removed v2 behavior. Preserve each verb's `Use`/`Short` string exactly (help-tree pins) and the JSON `output.Ok`/`output.Err` envelope.
- **Commit:** `refactor(webstercli): retarget verbs to planparser/websterengine, remove --restart-chain`

### Card 41: pause/status/weft retarget

- **Context:**
  - `internal/websterengine/pause.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/webstercli/pause.go`
  - `internal/webstercli/status.go`
  - `internal/webstercli/weft.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget the pause-surface references off `builderengine`: `pause.go`'s `builderengine.RequestPause(c.websterDir)` → `websterengine.RequestPause`; `status.go`'s `builderengine.PauseRequested(c.websterDir)` → `websterengine.PauseRequested`; `weft.go`'s `builderengine.PauseFlagName` (the `:(exclude)` pathspec that keeps the pause flag out of the weft commit) → `websterengine.PauseFlagName`. Keep the verb `Short` strings and behavior unchanged.
- **Commit:** `refactor(webstercli): retarget pause/status/weft to websterengine pause helpers`

### Card 42: cli tests + help-tree green

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/batcher/registry.go`
  - `internal/websterengine/digest.go`
  - `internal/websterengine/report.go`
- **Edits:**
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/webstercli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget the webstercli tests to the new types and fakes: `cli_test.go` and `verbs_test.go` inject fakes into `websterCLI` fields directly (existing pattern) — update the seam field types (`websterengine.Starter`, etc.), the `planparser.ParsePlan`/`planparser.Validate` call expectations, the `websterengine.Digest` digest-fields assertions, and drop any `--restart-chain`/`ValidateCaps`/oversized/chain test cases. Assert the pinned `validate` JSON contract from card 40: the ok-envelope count key is `cards` = `len(plan.Cards)` and each finding entry carries a `card` key from `f.Card` (not the old `batches`/`batch` keys). Add a test that an unknown `batcher:` config name fails fast at `PersistentPreRunE` with an `output.Err` envelope, and that the default/`identity` name resolves. Assert the rewritten verb `Long` strings contain no `--restart-chain`/chain/oversized/v2 language. Ensure the help-tree stays green: the eight verb names and their `Short` strings are unchanged, so `helptree`/`drift`/`registration`/`longlist` pins in `cmd/lyx` still pass; if `smoke_test.go` drives the plan surface, point its fixtures at a flat card-list plan (reuse a planparser-style fixture) and remove batch/scope/oversized assumptions. Tag any git-spawning cli tests `//go:build integration` and confirm the package `TestMain` (`testmain_test.go`) calls `lyxtest.HermeticGitEnv()`.
- **Commit:** `test(webstercli): retarget verb/seam tests to planparser/websterengine, keep help-tree green`

## Batch Tests

`verify: go test -tags integration ./internal/webstercli/...` runs the verb/seam behavior tests
(fake injection) plus any integration-tagged smoke tests. The help-tree/drift/registration pins
in `cmd/lyx` are exercised by the overview `go build ./...` gate and the full-suite done check;
this batch keeps the eight verb names and `Short` strings unchanged so those pins hold. The
existing `TestMain` neutralizes the operator gitconfig for any real-git smoke path.
