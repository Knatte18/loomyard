# Batch: shed-addressing

```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: shed-addressing
number: 7
cards: 5
verify: go build ./... && go test ./internal/shedcli/... ./internal/loomcli/... ./internal/battencli/... ./cmd/lyx/... && go test -tags integration ./internal/shedcli/...
depends-on: [1, 3, 4, 6]
```

## Batch Scope

This batch turns `lyx shed` from a flag-driven surface into a seed-driven one: the `--recipe` persistent flag is deleted, the four generic verbs take an optional run-id positional, the pre-run reads the addressed run's seed to decide which recipe to arm, `Arm` gains a `*lyxcwd.Location`-taking form so the resolution happens once, and a new `lyx shed seed` command writes the first seed.
It is one batch because all of it is one control-flow rewrite of `resolvePersistentPreRun` plus the signature change that rewrite forces on both arming modules, and because the deleted flag and the deleted `entry.Args` field cannot survive separately from the positional that replaces them.

It depends on all three of the preceding wiring batches: `internal/shedrun` for the seed read, and both `loomcli` and `battencli` for the `Arm` signature it changes in place.

Batch-local decision beyond the overview's: the **refusal precedence** on the `lyx shed` path is reordered by this batch and must be pinned rather than discovered.
The new order is (1) `lyxcwd.Resolve`'s own not-a-git-repository sentinel, (2) the seed read's run-id listing refusal, (3) the verb gate, (4) `Arm`'s own refusals, batten's non-prime refusal among them.
The visible consequence: `lyx shed run <some-slug>` typed from a *task* worktree now reads that worktree's own `_lyx/shed/` and refuses with a run-id listing, where today it reaches batten's "re-run this from prime" message.
That is acceptable because the clearer message stays reachable by the command an operator would actually type — `lyx batten run <slug>` arms `battencli` directly, and its own pre-run is unchanged.

## Cards

### Card 27: Arm gains a Location-taking form

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/shedcli/table.go`
  - `internal/shedrun/runid.go`
- **Edits:**
  - `internal/loomcli/arm.go`
  - `internal/battencli/arm.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Give both arming modules an exported `ArmAt(location *lyxcwd.Location, verb string, runID string) (shedverbs.Spec, error)` alongside today's `Arm(cwd string, verb string, args []string)`.
  `ArmAt` takes an already-resolved `*lyxcwd.Location` and an already-resolved run-id and performs no `lyxcwd.Resolve` of its own; `Arm` keeps its signature and becomes a thin wrapper that resolves cwd, derives the run-id from `args`, and delegates.
  Each module's own subtree pre-run keeps calling the cwd-resolving worker unchanged — this card adds an entry point, it does not remove one.
  The `lyx shed` path enters through `ArmAt` with the Location and the run-id already in hand, which is why the form exists: `shedcli`'s pre-run must read a seed before it knows which recipe to arm, a seed read is a path read, a path read needs an anchor, and resolving one twice per invocation to preserve a doc comment would cost a second `git rev-parse` on every call.
  Amend both files' `arm` doc comments where they state that each module's `Arm` owns its own resolution — that rule survives for the module's own subtree and no longer describes the `lyx shed` path.
  Preserve every existing refusal inside `ArmAt`, batten's non-prime refusal and `self`-address refusal among them, and preserve the receiver-sharing property both `Arm` wrappers document: the hooks must close over the same value the caller holds.
- **Commit:** `feat(loomcli,battencli): add a Location-taking ArmAt entry point`

### Card 28: the seed-driven pre-run and the deleted --recipe flag

- **Context:**
  - `internal/shedrun/seed.go`
  - `internal/shedrun/runid.go`
  - `internal/shedverbs/verbs.go`
  - `internal/shedverbs/spec.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/loomcli/arm.go`
  - `internal/battencli/arm.go`
- **Edits:**
  - `internal/shedcli/cli.go`
  - `internal/shedcli/table.go`
  - `internal/shedcli/doc.go`
  - `cmd/lyx/constructoranchoring_test.go`
  - `cmd/lyx/notransients_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `--recipe` persistent flag from the `shed` parent command and the `recipeFlag` variable behind it.
  Delete the `Args` field from `internal/shedcli/table.go`'s `entry` struct and the `argsFor()` closure in `cli.go` that read it, setting `cobra.MaximumNArgs(1)` statically on each of the four verbs instead.
  The closure's whole reason for existing was that `--recipe` was readable at validation time while the table entry was not; with one universal positional there is nothing left for a per-recipe argument contract to vary.
  Rewrite `resolvePersistentPreRun`'s body to the new sequence: short-circuit on `cmd.Name() == "shed"` as today, and additionally on `cmd.Name() == "seed"` for card 29's command; resolve cwd via `lyxcwd.CwdFrom`; resolve it into a `*lyxcwd.Location` via `lyxcwd.Resolve`, passing its error through bare since it is already the self-describing not-a-git-repository sentinel; resolve the run-id from `args`, defaulting to `shedrun.SelfRunID`; call `shedrun.ReadSeed`, refusing via `shedrun.MissingSeedMessage` with `lyx shed seed <run-id> --recipe <name>` as the remedy when `found == false`; look `seed.Recipe` up through the existing `lookup`, whose unknown-name error already names the available recipes; apply the existing verb gate against the entry's `Verbs` set; and call the entry's `ArmAt` with the Location and run-id.
  Keep that order exactly — it is this batch's pinned refusal precedence.
  Change the `entry.Arm` field's type to the `ArmAt` shape and retarget both map values to `loomcli.ArmAt` and `battencli.ArmAt`.
  Add `"step"` to the `"batten"` entry's `Verbs` set, now that batch 6 gave batten a step verb, and rewrite the map's doc comment, which today explains why batten has no step analogue — replace that explanation with a note that the gate itself stays because a future recipe may still exclude a verb, and the gate is what keeps `step`'s refusal-kind vocabulary closed at five values.
  The missing-run refusal must carry **no** `kind` field: a run-id that does not exist must not become a sixth kind.
  Rewrite every `--recipe` mention in the group's and the four verbs' `Short`, `Long` and `Example` text to the run-id positional form, keeping each `Short` non-empty, and update `doc.go` to match.
  This batch's own `verify` runs `go test ./cmd/lyx/...`, which surfaces two pre-existing defects from earlier same-task batches: `cmd/lyx/constructoranchoring_test.go` and `cmd/lyx/notransients_test.go` still reference `loomengine.LoomStatusFile`/`LoomStatusLock`/`LoomRunLock`, deleted by batch 4's `loomDirName-survives-the-status-relocation` decision in favour of `shedrun.StatusFile`/`StatusLock`/`RunLock`, and `notransients_test.go` still classifies `battencli.StatusFile` as transient after batch 3/6 relocated it onto the durable shed run directory.
  Retarget both files' constructor references onto the `shedrun` equivalents (at `shedrun.SelfRunID`) and move `battencli.StatusFile` into `durableSet`, so `go test ./cmd/lyx/...` passes; this is pre-existing breakage from an earlier batch in this same task, not new API surface this batch introduces.
- **Commit:** `feat(shedcli): address runs by run-id and resolve the recipe from the seed`

### Card 29: the lyx shed seed command

- **Context:**
  - `internal/shedrun/seed.go`
  - `internal/shedrun/runid.go`
  - `internal/shedcli/table.go`
  - `internal/shedcli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/shedcli/seed.go`
  - `internal/shedcli/seed_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `lyx shed seed <run-id> --recipe <name> [--driver <name>] [--param k=v]` as a `shedcli`-only cobra command with a non-empty `Short`, registered on the `shed` parent beside the four generic verbs.
  It is **not** a `shedverbs` verb: that package is a leaf that derives no path and knows no recipe table, and seeding needs both.
  `--recipe` survives here as a **local** flag naming the recipe being written rather than the one being armed; it is required.
  `--driver` defaults to `go` and refuses `llm` via `shedrun.ValidateDriver`.
  `--param` is repeatable and parses `k=v` pairs into the seed's `Params` map, refusing an entry with no `=` or an empty key.
  The command resolves cwd for itself, validates the run-id, validates the recipe name through the table's own `lookup` so a seed can only ever name an armable recipe, and calls `shedrun.WriteSeed`, which creates the run directory, is idempotent against a byte-identical existing seed, and refuses a disagreeing one.
  Its `RunE` checks `clihelp.ShouldAbort` first and reports errors as JSON via `internal/output`, per the CLI/Cobra Invariant.
  It is exempt from `resolvePersistentPreRun` by the `cmd.Name() == "seed"` short-circuit card 28 adds, for the reason that pre-run's whole sequence is predicated on a seed already existing while `seed` is by definition the command invoked when one does not — and it belongs to no recipe's `Verbs` set, so the verb gate would refuse it for every recipe.
  In `seed_test.go` cover the pre-run exemption first: `lyx shed seed <run-id> --recipe <name>` succeeds with no seed present and no recipe armed, which is the case the pre-run would otherwise refuse three different ways.
  Then cover an unknown `--recipe` refusing with the table's available names, `--driver llm` refusing with the roadmap pointer, a malformed `--param` refusing, idempotency against an identical seed, and refusal of a disagreeing one.
- **Commit:** `feat(shedcli): add the lyx shed seed command`

### Card 30: rewrite the arming and parity tests

- **Context:**
  - `internal/shedcli/cli.go`
  - `internal/shedcli/seed.go`
  - `internal/shedrun/seed.go`
  - `internal/loomcli/arm.go`
  - `internal/battencli/arm.go`
- **Edits:**
  - `internal/shedcli/table_test.go`
  - `internal/shedcli/cli_test.go`
  - `internal/shedcli/parity_test.go`
  - `internal/shedcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extract `armFromSeed(location *lyxcwd.Location, verb string, args []string) (shedverbs.Spec, error)` out of `resolvePersistentPreRun` in `cli.go`: the resolution-free body (run-id resolution, seed read, recipe lookup, verb gate, `Arm` call), taking an already-resolved Location, so `cli_test.go` can drive the seed-driven arming and refusal-precedence coverage below directly, with no real git repository behind it, staying Tier 1 -- matching `lyxcwd.Resolve` real-git-spawn cases belonging only in integration-tagged files elsewhere in this repo (e.g. `internal/lyxcwd/lyxcwd_test.go`).
  Extend `table_test.go`'s existing arming-coverage shape to the new `entry` struct with no `Args` field, and add the sync meta-test the overview's `shedrun-owns-the-recipe-name-vocabulary` Shared Decision requires: the `recipes` map's key set equals `shedrun.RecipeNames()`, so the vocabulary and the arming table cannot drift apart.
  In `cli_test.go`, cover seed-driven arming: the run-id positional defaulting to `self`; an absent seed refusing with a listing of existing run-ids; a seed naming an unknown recipe refusing with the table's available names; and the verb gate still firing for a verb a recipe's entry excludes.
  Pin the **refusal precedence** as a table — not-a-git-repository, then the run-id listing, then the verb gate, then `Arm`'s own refusals — since that order changed in this batch and a regression in it is invisible until an operator gets the wrong message from the wrong worktree.
  Pin explicitly that a missing-run refusal carries **no** `kind` field, which is the regression the Shed Verb-Set Invariant most invites.
  Delete `TestShed_LoomRunRefusesExtraArgByNoArgs` from `parity_test.go` rather than adapting it: it pins the table giving loom `cobra.NoArgs`, and the universal positional is what replaces that, with its intent — a wrong argument count refuses — carried by the new arity assertions.
  Rewrite `TestParity_PositionalArgs` rather than extending it.
  It exists to prove both paths refuse identically *because* both validate through the same shared `cobra.ExactArgs(1)` value; with the shared value gone it must assert that each path independently carries `MaximumNArgs(1)`.
  Its `TwoSlugs` case stays an arity refusal on both sides; its `NoSlug` case stops being a refusal at all.
- **Commit:** `test(shedcli): rewrite the arming, precedence and parity tests for run addressing`

### Card 31: amend the Shed Verb-Set Invariant's no-resolver bullet

- **Context:**
  - `internal/shedcli/cli.go`
  - `internal/shedverbs/seam_enforcement_test.go`
  - `internal/shedrun/paths.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `cmd/lyx/helptree_test.go`
  - `internal/shedverbs/step.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Amend `CONSTRAINTS.md`'s `## Shed Verb-Set Invariant`, whose final bullet states that neither `shedverbs` nor `shedcli` sits in the Told-Geometry Invariant's bound list because "their identical no-derived-paths obligation is carried by this invariant's own no-resolver clause".
  This batch makes that false of `shedcli`, which now calls `lyxcwd.Resolve` and builds seed paths from the result.
  Split the two: `shedverbs` keeps the no-resolver clause verbatim, enforced by `internal/shedverbs/seam_enforcement_test.go` as today; `shedcli` is carved out by name as the one site that resolves, on the stated ground that it cannot know which recipe to arm without first reading a seed, with the narrower obligation that every path it touches comes from a `shedrun` constructor and none is derived locally.
  In `cmd/lyx/helptree_test.go`, add `seed` to the `shed` module's pinned subcommand set and `step` to `batten`'s, both of which are new commands this task registers and which the CLI/Cobra Invariant's help-tree coverage must name.
  In `internal/shedverbs/step.go`, clarify `KindUnseeded`'s doc comment, whose "the status file could not be seeded" reads ambiguously now that `lyx shed seed` exists and writes a different artefact: say plainly that it means the **status file**, never `seed.json`.
  Its **value does not change** — the five-kind vocabulary is pinned closed by `internal/shedverbs/step_test.go` and by `ly-drive`'s own contract — and neither does `StepKinds`.
- **Commit:** `docs(constraints,shedverbs): carve shedcli out of the no-resolver clause and disambiguate KindUnseeded`

## Batch Tests

`verify: go build ./... && go test ./internal/shedcli/... ./internal/loomcli/... ./internal/battencli/... ./cmd/lyx/...` covers the four trees this batch changes plus a whole-module build.
Both arming modules are in scope because card 27 changes their exported surface and card 28 retargets the table at it; `cmd/lyx/...` is in scope because card 31 moves the help-tree gate and because the deleted `--recipe` flag changes the live command tree `drift_test.go` walks.

`internal/shedcli`'s untagged tests are Tier 1 and stay so.
Its `integration`-tagged pair runs too, via the verify's trailing `go test -tags integration ./internal/shedcli/...`: card 30 rewrites `parity_test.go`, which is `//go:build integration`, and an untagged run would never compile the file whose two rewritten cases are this batch's arity contract — `testmain_integration_test.go` comes along as that suite's `TestMain`.

Two pieces of coverage in this batch are load-bearing beyond their size.
The refusal-precedence table in `cli_test.go` is the only thing pinning an order this batch deliberately reordered, and the wrong-message-from-the-wrong-worktree failure it guards against is silent.
The no-`kind`-field assertion, present in both `cli_test.go` here and `arm_seed_test.go` in the two preceding batches, is what keeps the `step` vocabulary closed at five values across all three surfaces; `internal/shedverbs/step_test.go` pins the vocabulary itself and is reached transitively by the whole-module build rather than by this verify's test list, which is why the per-path assertions exist separately.

`table_test.go`'s new sync meta-test is the mechanism the overview's `shedrun-owns-the-recipe-name-vocabulary` decision rests on: without it the vocabulary and the arming table are two hand-maintained lists that can silently disagree.
