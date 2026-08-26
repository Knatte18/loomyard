# Batch: loomcli-wiring-and-flag

```yaml
task: 'loom: Plan-Write/Plan-Validate approval deadlock (F7)'
batch: 'loomcli-wiring-and-flag'
number: 5
cards: 4
verify: go test ./internal/loomcli/... ./cmd/lyx/...
depends-on: [3, 4]
```

## Batch Scope

This batch fills the seam batch 4 declared and mirrors the two-mode gate batch 3 created into the CLI: `wire()` supplies `Env.ApprovePlan` as a closure over `planparser.SetApproved`, and `lyx loom validate-plan` gains a `--require-approved` flag choosing between the two `planparser` entry points.

The flag exists because of the Gate Self-Check Parity Invariant, not for its own sake: two rows now share one engine in two modes, so a single-mode verb would leave one of them with no self-check.
The default is `false` because the verb's documented user is the writer agent calling it before handoff, which is pre-review — and that default is exactly what makes the plan stencil's Step 5 "re-run it until it exits 0" instruction satisfiable for the first time, over the `approved: false` plan the writer is ordered to produce.

Batch-local decision: the mode is a flag on the existing verb, not a second verb.
Two verbs for one gate contradicts the invariant's one-verb-per-gate shape.

## Cards

### Card 17: Fill Env.ApprovePlan in wire()

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/planparser/approve.go`
  - `internal/planparser/parse.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Fill `ApprovePlan` in the `shedrecipe.Env` literal `wire()` builds, placed immediately after the existing `CommitPlan` field so the plan-side seams stay adjacent, as a closure returning `planparser.SetApproved(planparser.PlanDir(location.AnchorPath()))`.
  Take the anchor from the already-resolved `location` exactly as the neighbouring `CommitPlan` closure does, per the Cwd Resolution Invariant, and reach the plan directory through `planparser.PlanDir` rather than any hand-built join naming the `_lyx` literal, per the Lyxdirs Single-Declarer Invariant.
  The file already imports `planparser`, so add no import.
  Give the field a comment stating that this closure is what flips `approved: true` on the `Plan-Bouncer` row's approved settle, that it runs before that row's commit seam so the flag lands inside the commit rather than as working-tree dirt afterwards, and that it is idempotent — a second run over an already-approved plan is a successful no-op — which is what makes the failed-settle resume path converge.
- **Commit:** `17: loomcli: wire Env.ApprovePlan to planparser.SetApproved`

### Card 18: Give validate-plan its --require-approved flag

- **Context:**
  - `internal/planparser/validate.go`
  - `internal/loomshed/planvalidate.go`
- **Edits:**
  - `internal/loomcli/validate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a bool flag named `require-approved`, defaulting to `false`, to the `validate-plan` subcommand, bound to a local variable the command's `RunE` reads.
  With the flag absent, call `planparser.ValidateFormat`;
  with it set, call `planparser.Validate`.
  Change nothing else about the command's behaviour: `Args: cobra.NoArgs` stays, the `clihelp.ShouldAbort` check stays first in `RunE` per the CLI/Cobra Invariant, the parse-failure path still emits `output.Err`, the findings path still emits `output.ErrFields` with the `findings` key, and the success path still emits `output.Ok` with `plan_dir` — the envelope's structural discrimination by the presence of the `findings` key is what the parity comparison keys off and must not move.
  Rewrite the whole `Long` paragraph rather than only its no-flags sentence: it currently claims the verb runs `planparser.Validate` and that it takes no flags, and both halves become false.
  The new text names both modes, says the default mode matches the `Plan-Validate` row while `--require-approved` matches the `Plan-Revalidate` row, and states that the default is the mode the plan writer calls before handoff.
  Keep the `Example:` section and extend it with a second line showing the flag.
  The command already carries a non-empty `Short`, which the CLI/Cobra Invariant requires;
  leave it as it is unless the new modes make it inaccurate.
- **Commit:** `18: loomcli: add --require-approved to validate-plan`

### Card 19: Re-key the plan gate-parity test by mode

- **Context:**
  - `internal/loomcli/validate.go`
  - `internal/loomshed/planvalidate.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomcli/parity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Turn `TestGateParity_PlanValidate` into a two-dimensional table: the four fixtures {clean approved, clean unapproved, format-invalid, absent plan directory} crossed with the two modes {flag absent, `--require-approved`}.
  Each cell constructs the producer with the matching `requireApproved` argument and drives the verb with the matching argument slice — `nil` for the flag-absent mode, a slice carrying the flag string for the other — then asserts the producer's mapped verdict equals the verb's mapped verdict and equals the cell's expected verdict.
  The existing `Stuck_Unapproved` case is re-keyed rather than deleted: unapproved maps to `stuck` in the `--require-approved` mode and to `done` in the flag-absent mode.
  That flag-absent-times-unapproved cell is the one that proves the deadlock is gone and must expect `done`;
  say so in the test's own doc comment.
  The format-invalid fixture is `stuck` in both modes and the absent-plan-directory fixture is `error` in both.
  Keep the existing `planParityCase` shape, the existing `producerVerdict`/`cliVerdict`/`decodeSingleEnvelope` helpers, and the envelope's structural `findings`-key discrimination unchanged.
  Leave `TestGateParity_DiscussionValidate` in the same file untouched.
- **Commit:** `19: loomcli: drive plan gate parity across both modes`

### Card 20: Cover the flag's registration and its two modes at the verb

- **Context:**
  - `internal/loomcli/validate.go`
  - `internal/loomcli/parity_test.go`
- **Edits:**
  - `internal/loomcli/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `TestValidatePlanCmd` so its existing fixtures run in both modes, keeping this file's own local `planFixture(t, anchorPath, worktreeRoot, approved bool)` helper and its `approved` parameter as they are.
  The unapproved fixture now expects a success envelope in the default mode and a `findings` envelope naming `plan-unapproved` in the `--require-approved` mode;
  the clean approved fixture succeeds in both;
  the format-invalid and absent-plan-directory fixtures behave identically in both.
  Add one assertion that the flag is registered on the command under the exact name `require-approved` with a `false` default, since the repo's help-tree tests assert subcommand names rather than flags and would not catch a flag that failed to register.
  No positional-argument case is needed — `cobra.NoArgs` already covers that and is unchanged by this batch.
- **Commit:** `20: loomcli: test validate-plan in both modes`

## Batch Tests

`verify: go test ./internal/loomcli/... ./cmd/lyx/...` covers both the batch's own edits and the one place outside the package that observes them.

`internal/loomcli` is the edit surface: cards 19 and 20 are the direct tests, and the package's existing `wiring_test.go` is the regression surface for card 17's new `Env` field.
`cmd/lyx` is included because card 18 changes an observable CLI surface, and that package holds the help-tree, registration, `Short`-drift, and seam-signature tests the CLI/Cobra Invariant is enforced by;
it also holds the tier-purity and hermetic-git-environment guards, which a new test in `internal/loomcli` could otherwise trip without any signal inside `internal/loomcli` itself.
