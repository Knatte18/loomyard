# Batch: loomshed-interrupt-policy

```yaml
task: "lyx loom step + external supervisor skill"
batch: "loomshed-interrupt-policy"
number: 2
cards: 2
verify: go test ./internal/loomshed/... ./internal/loomrecipe/...
depends-on: []
```

## Batch Scope

This batch delivers the exported interrupt-policy table and the meta-test that pins it against loom's real producer list.
It is one batch because the table and its guard are a single unit of meaning: an unpinned table is exactly the silent-drift hazard the guard exists to prevent, and the guard cannot live in the same package as the table.
The external interface batches 4 and 5 consume is `loomshed.InterruptPolicyFor(name string) string`.

Batch-local decision: the meta-test lives in `internal/loomrecipe`'s test package rather than beside the table. `loomRowEngines` is an unexported package-level `var` in `internal/loomrecipe/coverage_guard_test.go`, so a test in `internal/loomshed` cannot see it at all, and `loomrecipe`'s test package is the only place that can see both that mapping and the exported `loomshed` table. A small, table-only unit test still lives beside the table in `internal/loomshed`, covering the accessor's own contract rather than its agreement with the recipe.

## Cards

### Card 4: the exported interrupt-policy table and its accessor

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/doc.go`
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/shedadapters/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/interruptpolicy.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Declare two exported string constants, `InterruptPolicyReinvoke = "reinvoke"` and `InterruptPolicyHandback = "handback"`, and an exported `var InterruptPolicies = map[string]string{...}` keyed by the seventeen existing `Name*` constants from `loomshed.go` — never by repeated string literals, since those constants are the durable on-disk identities and a literal here would not follow a rename.

  Every row maps to `InterruptPolicyReinvoke` except `NameWebster`, which maps to `InterruptPolicyHandback`. So: `NamePreflight`, `NameLoomPreflight`, `NameDiscussionWrite`, `NameDiscussionValidate`, `NameDiscussionBouncer`, `NameDiscussionBurler`, `NamePlanWrite`, `NamePlanValidate`, `NamePlanBouncer`, `NamePlanBurler`, `NamePlanRevalidate`, `NameBatchifier`, `NameWebsterBouncer`, `NameWebsterBurler`, `NamePublish`, and `NameFinalize` are all `reinvoke`; `NameWebster` alone is `handback`.

  Add an exported `func InterruptPolicyFor(name string) string` returning the table's entry for `name`, and the empty string when `name` is empty or names no row. Document that the empty return is the caller's signal to omit the key rather than a third policy value.

  Document the table's reason to exist and the `Webster` carve-out's reason in the package-level comment on the file. State that on every row but one, re-calling `current_producer` after an interrupted invocation does not double-spawn, because `internal/shedadapters/doc.go`'s "Every spawning adapter probes for a live agent first" section records that `SingleLLMProducer`, `Bouncer`, and `BurlerProducer` all call `shuttleengine`'s `Attach` seam with the step's own `OutputFiles` and wait on a match before any archive — so a re-invocation reattaches to the live agent rather than spawning a second one. State that `WebsterProducer` is the exception because it inherits `websterengine`'s own entry-time reclaim, which stops a leftover Master rather than attaching to it (`reclaimEntryTimeStrands` in `internal/websterengine/runlevel.go`), so re-invoking an interrupted `Webster` step kills the in-flight Master and restarts the batch run from `state.json`. Record that correctness survives that restart — recovery from `state.json` is what the reclaim exists for — but the cost does not, `Webster` being the most expensive row in the list, and that the cost is the operator's to accept rather than the supervisor skill's.

  Add no import beyond the standard library; this file declares data and one lookup, and must not reach for the recipe, the adapters, or any engine.
- **Commit:** `feat(loomshed): add the exported interrupt-policy table for loom's seventeen rows`

### Card 5: the accessor's unit test and the recipe meta-test

- **Context:**
  - `internal/loomshed/interruptpolicy.go`
  - `internal/loomshed/loomshed.go`
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomrecipe/shape_test.go`
  - `internal/loomrecipe/fixture_test.go`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/interruptpolicy_test.go`
  - `internal/loomrecipe/interruptpolicy_meta_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/loomshed/interruptpolicy_test.go`, cover `InterruptPolicyFor`'s own contract: it returns `InterruptPolicyHandback` for `NameWebster`, `InterruptPolicyReinvoke` for at least one row of each other shape (a gate, a writer, a validator, a `Bouncer`, and a `Burler`), the empty string for the empty string, and the empty string for a name that is in no row. Also assert every value in `InterruptPolicies` is one of the two declared constants, so a typo'd third policy word cannot ship.

  In `internal/loomrecipe/interruptpolicy_meta_test.go`, write the meta-test against the **production** authority — the rows `New` actually assembles. Build a real `shedrecipe.Env`/`ShedPaths` pair with the existing `testEnv(t)` helper and call this package's own `New`, exactly as `TestCoverageGuard_EveryLoomRowHasAnEngine` does. `testEnv` is declared in `internal/loomrecipe/shape_test.go`, not in `fixture_test.go`; `fixture_test.go` is listed alongside it because `testEnv` fills its Landing seam via `testLandingDeps`, which is declared there. Assert in both directions: every row `New` assembles has an entry in `loomshed.InterruptPolicies`, and every key in `loomshed.InterruptPolicies` names a row `New` actually has — so the table covers exactly those row names, no more and no fewer.

  Then cross-check against `loomRowEngines`, the unexported package-level `var` already declared in `coverage_guard_test.go`, which this package's test binary can see: assert that every row whose engine is `"Webster"` carries `InterruptPolicyHandback`, and that every row whose engine is anything else carries `InterruptPolicyReinvoke`. Do not redeclare `loomRowEngines`, `testEnv`, or `testLandingDeps` — all three already exist in this package's test binary. Explain in the test's own comment that this is the assertion that keeps a row which changes adapter from silently keeping the wrong policy, and that the engine-name side is what makes the `Webster` exception derivable rather than hand-maintained.
- **Commit:** `test(loomshed): pin the interrupt-policy table against loom's assembled rows`

## Batch Tests

`verify: go test ./internal/loomshed/... ./internal/loomrecipe/...` runs both packages the batch touches, and nothing else. Both are fast unit-test packages with no real fabric, no real reed, and no git spawn.

`internal/loomshed/interruptpolicy_test.go` covers the accessor's own contract and the table's closed value vocabulary. `internal/loomrecipe/interruptpolicy_meta_test.go` covers the agreement between the table and the rows `loomrecipe.New` assembles, plus the engine-identity cross-check. Running `loomrecipe` is required rather than incidental: the meta-test lives there, and it is the only test in the plan that can fail when a row is added to the recipe without a policy.
