# Batch: shedrecipe-wiring

```yaml
task: 'Producer gates: mechanical gates before session release'
batch: 'shedrecipe-wiring'
number: 4
cards: 6
verify: go test ./internal/shedrecipe/... ./internal/loomrecipe/... && go test -tags smoke -run '^$' ./internal/loomcli/...
depends-on: [2, 3]
```

## Batch Scope

This batch is where the four gated rows become gated: two new row-config keys, one shared resolver, three entry constructors reading them, and the four recipe rows declaring them.
After it, `Discussion-Write`, `Discussion-Burler`, `Plan-Write`, and `Plan-Burler` each hold a live gate, while the three standalone validate rows are still in the graph and still pass — a gated write row that produces a valid artifact reaches `Done`, and the row below it validates the same artifact a second time and also passes.
That redundancy is deliberate and lasts exactly one batch: it keeps the tree green and every sequence expectation stable while the wiring lands, and batch 5 removes the rows.

The gate closure and its `gate_attempts` budget are packed into one `GateSpec` here, at the one point where both are known — the entry constructor, which reads the row's `config:` block and `Env` in the same place — and travel as a unit from there, so a gate and its budget cannot be separated at any hop.

Batch-local decision: `gate_attempts: 3` is written explicitly on all four rows rather than left absent to take the package default.
The recipe is where an operator tunes a row, the four sites have genuinely different re-prompt costs, and an explicit value is what makes that tunable without reading Go.
The absent-means-default branch stays covered by the `shedrecipe` unit tests.

## Cards

### Card 22: The shared gate resolver for every gated entry

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/loomshed/gates.go`
  - `internal/shedrecipe/config.go`
  - `internal/shedrecipe/paths.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_gate.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func resolveGateSpec(entry string, cfg Config, env Env) (shuttleengine.GateSpec, error)`, the single place the two new row keys are read and turned into a `shuttleengine.GateSpec`.
  It reads the optional string key `gate` via `configString` and the optional int key `gate_attempts` via `configInt`, and resolves `gate`'s closed two-value vocabulary against `Env`: `discussion` requires `Env.DecisionRecordPath` and `Env.SupportLogPath` to pass `requireAbsRoot` and returns `loomshed.NewDiscussionGate` over them; `plan` requires `Env.AnchorPath` and `Env.WorktreeRoot` and returns `loomshed.NewPlanGate` over them; any other non-empty value is an error naming the key and both legal values.
  An absent `gate` returns the zero `GateSpec`, which is what every ungated row carries by saying nothing.
  A `gate_attempts` present with no `gate` is an error naming both keys, never a silently-ignored key: it is unambiguously an author mistake, and this package's constructors already fail loud on malformed config rather than defaulting.
  Every error is qualified with `entry` so a recipe author with a typo is told which row spoke, matching the qualification `resolveUnderRoot` already applies.
  The resolver reads the keys but never rejects unknown ones — each caller keeps owning its own `configRejectUnknown` call, which is where the Config Strictness Invariant's strictness lives.
  The file's doc comment records that the selector is a declared string resolved against `Env` exactly as `bouncerEntry` already resolves `commit_seam`/`approve_seam`, and that selecting the validator by switching on the row **name** was rejected because it would make row names load-bearing in a second place beyond resume identity, where a renamed row would silently lose its gate rather than failing to build.
  This package's import allowlist needs no edit: it already admits both `loomshed` and `shuttleengine`, and under this placement it never names `planglyph` or `discussionparser` at all.
- **Commit:** `feat(shedrecipe): add the shared gate/gate_attempts config resolver`

### Card 23: The two writer entries build gated producers

- **Context:**
  - `internal/shedrecipe/entries_gate.go`
  - `internal/shedrecipe/config.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/loomshed/planwrite.go`
- **Edits:**
  - `internal/shedrecipe/entries_discussionwrite.go`
  - `internal/shedrecipe/entries_planwrite.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In both entries, call `resolveGateSpec` with the entry's own name, widen the `configRejectUnknown` call from its currently empty permitted set to `"gate", "gate_attempts"`, and build the inner producer through `shedadapters.NewSingleLLMProducerGated`, passing the resolved spec as the trailing argument and leaving every other argument exactly as it is today.
  Both rows carry a `gate:` key even though their dedicated constructors could imply the validator, so that one key means one thing at all four gated sites and a reader of the recipe can see which validator guards each row without opening Go; the entries must therefore not hard-code a validator choice of their own.
  Correct `planWriteEntry`'s doc comment sentence stating that the row carries no Config keys of its own — it now carries exactly these two — and extend both doc comments with the gate the row builds.
  `Env.DecisionRecordPath`, `Env.SupportLogPath`, `Env.AnchorPath`, and `Env.WorktreeRoot` are already populated for these rows and are validated inside the resolver, so neither entry gains a `requireAbsRoot` call of its own beyond the ones it already makes.
- **Commit:** `feat(shedrecipe): build Discussion-Write and Plan-Write as gated producers`

### Card 24: The burler entry selects its round's validator

- **Context:**
  - `internal/shedrecipe/entries_gate.go`
  - `internal/shedrecipe/config.go`
  - `internal/burlerengine/profile.go`
- **Edits:**
  - `internal/shedrecipe/entries_burler.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Call `resolveGateSpec` in `burlerRoundEntry` and assign the result to the `Gate` field of the `burlerengine.RunOpts` it already builds.
  Add `"gate"` and `"gate_attempts"` to the **row-level** `configRejectUnknown` call — the one listing `run_subdir`, `profile`, `model`, `effort`, `timeout_s` — and never to `burlerRoundProfile`'s nested `profile:` sub-map allowlist, which is a different list governing a different map.
  All three burler rows share this one constructor, which is why the validator is selected by the `gate:` key rather than implied by the constructor; a `gate:` on the Webster round fails loud through the resolver's own closed vocabulary, because no third validator exists to name.
  Add a test-visible note to the entry's doc comment recording that the Webster round is deliberately ungated: there is no mechanical validator over a committed diff, so its `RunOpts.Gate` stays the zero `GateSpec`.
- **Commit:** `feat(shedrecipe): select a burler round's validator from its gate config key`

### Card 25: The four gated rows declare their gate

- **Context:**
  - `internal/shedrecipe/entries_gate.go`
  - `internal/shedrecipe/entries_burler.go`
  - `contracts/recipes/recipes.go`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomrecipe/resume_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Give the `Discussion-Write` row its first `config:` block, carrying `gate: discussion` and `gate_attempts: 3`, and the `Plan-Write` row one carrying `gate: plan` and `gate_attempts: 3`.
  Add the same two keys at row level to the `Discussion-Burler` and `Plan-Burler` rows' existing `config:` blocks, as siblings of `run_subdir` and `profile` and never inside `profile:`, with `gate: discussion` and `gate: plan` respectively.
  Leave the Webster round's block untouched and add a comment there stating that its absent `gate:` key is deliberate, matching the style the row's existing absent-key comments already set.
  Each new block carries a comment naming what the key buys: the gate holds the row's handoff until the artifact is mechanically valid, re-prompting the live session up to `gate_attempts` times, so a failure never reaches the row below.
  Do not touch any `on_done`/`on_stuck` edge, any `max_bounces`, or the three standalone validate rows in this card — the graph is unchanged here and batch 5 owns its rewiring.
  Discussion-Write's new gate makes `internal/loomrecipe/resume_test.go`'s `TestBounceRouting_StuckContinuesAtDeclaredTarget` and `TestBounceRouting_BudgetExhaustionBlocks` fall out of date: both assumed Discussion-Write always reports Done regardless of its artifact's content, which the gate makes false, so both are adjusted in this card to drive their same generic shed-routing assertions (single-bounce continuation, and per-producer bounce-budget exhaustion) through a producer pair the new gate does not touch -- Discussion-Validate (planted directly via `resetCurrentProducer`, skipping Discussion-Write's own now-load-bearing gate) for the first, and Discussion-Bouncer/Discussion-Burler (whose mutual bounce this task's Burler-round gating never reaches inside this package's own fakes) for the second.
- **Commit:** `feat(recipe): declare gate and gate_attempts on the four gated rows`

### Card 26: shedrecipe gate-key tests

- **Context:**
  - `internal/shedrecipe/entries_gate.go`
  - `internal/shedrecipe/entries_discussionwrite.go`
  - `internal/shedrecipe/entries_planwrite.go`
  - `internal/shedrecipe/entries_burler.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedrecipe/seam_enforcement_test.go`
- **Edits:**
  - `internal/shedrecipe/entries_discussionwrite_test.go`
  - `internal/shedrecipe/entries_planwrite_test.go`
  - `internal/shedrecipe/entries_burler_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Cover, across the three entry test files: `gate: discussion` and `gate: plan` each build a producer whose resolved gate is the matching closure, asserted by driving the constructed gate and observing which validator's findings come back rather than by comparing func values, which Go cannot compare; an unrecognised `gate:` value fails loud with a message naming the key and both legal values; a `gate_attempts` with no `gate:` fails loud with a message naming both keys; a row carrying neither key builds an ungated producer whose `GateSpec` is the zero value; and an absent `gate_attempts` alongside a present `gate:` yields the package default rather than zero.
  For the burler entry specifically, cover that `gate_attempts` is accepted at the row level and **rejected** inside the `profile:` sub-map, which is the guard against the two allowlists being confused.
  The existing `seam_enforcement_test.go` allowlist assertion is the standing guard that the closures did not drift into this package and needs no edit; state that in the test file's own comment so a later reader does not add one.
- **Commit:** `test(shedrecipe): cover the gate and gate_attempts config keys at all three entries`

### Card 27: The live-substrate gate smoke test

- **Context:**
  - `internal/loomcli/smoke_attachprobe_test.go`
  - `internal/shuttleengine/gate.go`
  - `internal/shuttleengine/run.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/loomshed/gates.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/smoke_gate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add one `//go:build smoke` end-to-end test, modelled on the harness the neighbouring attach-probe smoke test already builds, proving the one thing no untagged test can: that a real agent, re-prompted through a real pane after a deliberately-invalid first artifact, fixes it and the row reports `Done`.
  It is the only test in the task that exercises real `Send` delivery against a real pane, which is the single mechanism every unit test necessarily fakes.
  Assert that the run reaches `shedengine.Done`, that the gate's reported `Attempts` is at least 1, and that the artifact on disk passes the same gate closure afterwards.
  The file is smoke-tagged, so it is outside this batch's untagged run and outside the repo-wide done gate; the batch's chained tagged invocation, whose `-run` pattern matches nothing, is what type-checks it without executing it, and the test file's own doc comment must state that this compile gate is its only automatic guard and that an operator runs the test itself by hand against a real substrate.
- **Commit:** `test(loomcli): add the live-substrate gate re-prompt smoke test`

## Batch Tests

`verify: go test ./internal/shedrecipe/... ./internal/loomrecipe/... && go test -tags smoke -run '^$' ./internal/loomcli/...` covers the package carrying the work, the package that builds the whole recipe from it, and a compile check for the one smoke-tagged file this batch adds.
`internal/loomrecipe` is in scope rather than deferred because card 25 edits the embedded recipe every one of its guards builds from — the coverage guard, the shape test, the sequence test, and the routing-graph test all rebuild the real seventeen-row list, and a malformed `config:` block on any of the four rows fails there rather than in `shedrecipe`.
The sequence test is the load-bearing one in this batch: its nineteen-entry expectation must still hold unchanged, which is what proves that turning four rows gated left the graph's behaviour identical while the standalone validate rows are still in it.
The gates are genuinely exercised in that run rather than stubbed out, because batch 2 gave `fakeLoomShuttle` a `RunGated` that invokes the closure.
The smoke file is not run: it needs a real agent and a real pane, and the chained tagged invocation is deliberately a compile gate and nothing more.
