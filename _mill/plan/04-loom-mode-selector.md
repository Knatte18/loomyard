# Batch: loom-mode-selector

```yaml
task: 'loom: interactive Discussion-Write'
batch: 'loom-mode-selector'
number: 4
cards: 3
verify: go test ./internal/loomengine/ ./internal/loomcli/
depends-on: [1]
```

## Batch Scope

This batch replaces the hardcoded `true` at `internal/loomcli/wiring.go`'s `DiscussionSpec` closure with a real mode selector, threaded from a new `loom.yaml` key through `loomengine.Config` into `wire()`, and makes `DiscussionSpec` set batch 1's `AwaitOperator` alongside the `Interactive` it already sets.

It is one batch because the three edits are one thread: the config key is useless without the struct field, the struct field is useless without the `wire()` expression, and the `wire()` expression is useless without `DiscussionSpec` setting `AwaitOperator`.
It depends on batch 1 only — it needs `Spec.AwaitOperator` to exist and nothing from `Attach`.
It is deliberately parallel with batch 3: neither touches a file the other does.

Batch-local decisions:

- The key is `discussion_interactive`, default `false`, and it is the **only** spelling.
  No `--interactive` flag on `lyx loom run` is added, per `mode-selector-lives-in-loom-yaml`: `lyx loom run` spawns a **detached** `lyx loom drive` child and then hands the terminal to tmux, so a flag on `run` cannot reach the process that builds the wiring without being persisted somewhere the child reads, and `loom.yaml` is already loaded by both paths through the same `resolvePersistentPreRun`.
  The **CLI / Cobra Invariant** is therefore not engaged: no new verb, no new flag.
- The mode is **not** load-bearing for resume, per `mode-is-not-load-bearing-for-resume`.
  It is read fresh on every `wire()`, nothing compares the current mode against the mode a live run was started with, and flipping it between a crash and a resume is permitted and benign.
  No card here may add such a comparison, and no field is added to `loomengine.Status` or `contracts/specs/loom-status-spec.md`.
- `PlanSpec` is untouched. It hardcodes `Interactive: false` internally by design, and `wiring_test.go`'s existing `PlanSpec` assertion is the regression guard that this change did not leak into the plan producer.

## Cards

### Card 10: `discussion_interactive` in `loom.yaml` and `loomengine.Config`

- **Context:**
  - `_mill/discussion.md`
  - `internal/loomengine/configtemplate.go`
  - `internal/configengine/config.go`
  - `internal/loomengine/discussion.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomengine/template.yaml`
  - `internal/loomengine/config.go`
  - `internal/loomengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a seventh key to `internal/loomengine/template.yaml`: `discussion_interactive: false`, with an inline comment stating that `true` runs the discussion-phase agent as an interview in its pane and requires an operator to be present, and that `false` — the default — is today's autonomous behaviour, because a task spawned unattended must not silently wait for a human.
  Place it immediately after the existing `discussion_timeout_min` line so the discussion role's three knobs sit together.

  Add the matching field to `loomengine.Config` in `config.go`: `DiscussionInteractive bool` with yaml tag `discussion_interactive`, placed after `DiscussionTimeoutMin`.
  The **Config Strictness Invariant** requires both edits in one change: `configengine.Load` hard-errors on unknown and missing keys, so a struct field with no template line is a silent hole and a template line with no struct field is a load failure.
  `LoadConfig` needs no new validation for this key — it is a plain bool with no grammar to check, unlike the three model-spec keys.

  Record the migration obligation in the commit message body, per the discussion's "Migration obligation for existing worktrees" note: adding a key to a strict-side template breaks every worktree whose `loom.yaml` predates it, and the remedy is **`lyx config reconcile --apply`**, not the bare `lyx config reconcile` the `configengine` error text names — the bare verb is a dry run that reports added and removed keys and writes nothing, so an operator who follows the error message literally reconciles nothing and fails the next `lyx loom run` identically.
  The commit message must say this applies to every in-flight worktree, not only new ones.
  Do not change `internal/configengine`'s own error text — whether it should gain `--apply` is a separate repo-wide question this task does not settle.

  Extend `config_test.go`: assert `TestLoadConfig_WellFormed` sees `cfg.DiscussionInteractive == false` from the template's own default, and add a case seeding a hand-written `loom.yaml` with `discussion_interactive: true` that round-trips to `true`.
  Every existing hand-written yaml literal in that file's malformed-spec tests must gain the new key, or `configengine.Load`'s missing-key check fails them.
  `TestConfigTemplate_ContainsEveryConfigYAMLTag` needs no edit — it walks `Config` reflectively and will cover the new tag automatically.
  Do not re-test `configengine`'s own missing-key and unknown-key behaviour here; it is already covered in that package.
- **Commit:** `feat(loom): add the discussion_interactive config key`

### Card 11: `DiscussionSpec` sets `AwaitOperator` alongside `Interactive`

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/spec.go`
  - `internal/loomengine/prompt.go`
  - `internal/loomengine/config.go`
  - `internal/loomengine/prompt_test.go`
  - `contracts/stencils/loom/loom-template-discussion.md`
- **Edits:**
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/discussion_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `DiscussionSpec`, set `AwaitOperator: !autonomous` on the returned `shuttleengine.Spec`, immediately beside the existing `Interactive: !autonomous`.
  Add a comment stating why both are set from the same expression yet remain two fields: `Interactive` means "an operator is present" and governs launch flags and the `AskUserQuestion` recording hook, while `AwaitOperator` means "wait for the operator rather than reporting back" and governs the wait loop — and that loom's interactive `Discussion-Write` deliberately wants both, dropping the real-time asking signal, since the whole point here is to wait rather than report back.
  Make no other change to `DiscussionSpec`: the prompt side is already complete, `modeRules(autonomous)` already returns two finished blocks, and no stencil edit is expected.
  If one turns out to be needed, stop rather than proceeding — the `Stencil Ownership Invariant` and the `Producer Pointer-Rule Invariant` both bind and that is a separate decision.

  Extend `discussion_test.go`'s existing two-case table (`Interactive` / `Autonomous`) with a `wantAwaitOperator` column, asserting `autonomous=false` produces `Interactive == true` **and** `AwaitOperator == true`, and `autonomous=true` produces both `false`.
  `prompt_test.go` needs no change — its mode-rules coverage already pins that the two renderings differ, that the autonomous one says "best-judgment", that the interactive one says "operator", and that neither mentions a `--auto` flag.
- **Commit:** `feat(loom): set AwaitOperator from DiscussionSpec's autonomous argument`

### Card 12: `wire()` reads the mode instead of hardcoding `true`

- **Context:**
  - `_mill/discussion.md`
  - `internal/loomengine/config.go`
  - `internal/loomengine/discussion.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/loomengine/configtemplate.go`
  - `internal/loomengine/config_test.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `wire()`, change the `DiscussionSpec` closure's last argument from the literal `true` to `!loomCfg.DiscussionInteractive`.
  `loomCfg` is already in scope at that point, so no other plumbing changes.

  Rewrite — do not merely edit around — the comment two lines above the closure, which currently ends "autonomous is the literal true, unconditionally, per the autonomous-only Shared Decision."
  The replacement must keep the first half (the closure is evaluated per `Call`, not resolved here, so the stencil is read at call time — what the `Stencil Ownership Invariant` requires) and say instead that `autonomous` is now `!loomCfg.DiscussionInteractive`, read fresh on every `wire()`, and that nothing compares it against the mode a live run was started with: the resume decision is made purely on live-agent evidence, so flipping the key between a crash and a resume is permitted and benign and means only that the next spawn is interviewed differently.
  Leave the sibling `PlanSpec` closure's comment untouched — its contrast with `DiscussionSpec` ("Unlike `DiscussionSpec` beside it, `PlanSpec` takes no autonomous argument") stays accurate.

  In `wiring_test.go`, replace `TestWire_DiscussionSpecEvaluatesToExpectedShape`'s single `if spec.Interactive { ... "autonomous-only" }` assertion with a two-case sub-test driving `wire()` with the mode false and true and asserting the resulting `DiscussionSpec()` closure's `Interactive` and `AwaitOperator` in each.
  Both must be `false` for `discussion_interactive: false` and both `true` for `discussion_interactive: true`.
  Every other assertion in that test — `Role`, `Timeout`, `Model`, `OutputFiles`, `Prompt` — stays and must hold in both cases.
  The file's `hubLocation` helper calls `seedLoomConfig`, which writes the embedded template verbatim, so add a sibling seeding helper that writes a full seven-key `loom.yaml` with a caller-chosen `discussion_interactive` value and have the interactive case call it after `hubLocation` returns.
  Write all seven keys explicitly rather than string-substituting the template, since `configengine.Load` is strict on missing keys and an explicit literal is what `internal/loomengine/config_test.go` already does.
  `TestWire_PlanSpecEvaluatesToExpectedShape`'s `spec.Interactive` assertion at the same file must stay green untouched — it is the regression guard that this change did not leak into the plan producer.
- **Commit:** `feat(loom): wire discussion_interactive into the DiscussionSpec closure`

## Batch Tests

`verify: go test ./internal/loomengine/ ./internal/loomcli/` is scoped to the two packages this batch edits.

`internal/loomengine` covers `config_test.go` (the new key's default and its `true` round-trip, plus the reflective `TestConfigTemplate_ContainsEveryConfigYAMLTag` guard that struct and template agree), `discussion_test.go` (the `AwaitOperator`/`Interactive` pair in both modes), and `prompt_test.go` as an untouched regression guard on the two mode-rules renderings.

`internal/loomcli` covers `wiring_test.go`'s new two-case discussion assertion and its untouched `PlanSpec` counterpart, plus every other `wire()` assertion, which must survive the added config key — `seedLoomConfig` writes the embedded template, so a template line without a matching struct field would fail all of them at once.

Both packages are untagged Tier 1 and drive hand-built `*lyxcwd.Location` values over temp dirs with no git, tmux, or network involved.

The overview's module-wide `verify: go vet ./...` catches any other caller of `loomengine.Config` or `DiscussionSpec` broken at this boundary.
