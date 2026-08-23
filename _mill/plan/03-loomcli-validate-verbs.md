# Batch: loom CLI validate verbs

```yaml
task: 'loom: self-checkable mechanical gates'
batch: 'loom CLI validate verbs'
number: 3
cards: 3
verify: go test ./internal/loomcli/... ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

This batch adds the two zero-argument verbs the whole task exists to ship — `lyx loom validate-discussion` and `lyx loom validate-plan` — registers them on the existing `loom` cobra subtree, covers them with tier-1 tests, and turns the package's registered-verb assertion into a real exact-set guard.
It is one batch because the three cards share one surface: the verbs, their tests, and the registration guards that keep them discoverable are meaningless apart, and every file touched is either in `internal/loomcli` or is `cmd/lyx/helptree_test.go`'s single `loom` entry.

It depends on batch 1 only — `validate-discussion` calls `discussionparser.Validate` and `validate-plan` calls `planparser` directly, so neither verb needs batch 2's `loomshed` rewrite.
The external interface batch 4 consumes is the two unexported constructors `(*loomCLI).validateDiscussionCmd` and `(*loomCLI).validatePlanCmd`, driven against a hand-populated receiver.

Batch-local decision: both verbs live in one new file, `internal/loomcli/validate.go`, rather than one file each.
They are two ten-to-twenty-line `RunE` bodies over the same receiver fields with the same envelope contract, and splitting them would put the shared envelope-shaping helper in a third file for no gain.

## Cards

### Card 5: the validate-discussion and validate-plan verbs

- **Context:**
  - `internal/loomcli/status.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/discussionparser/validate.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
  - `internal/output/output.go`
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/loomcli/cli.go`
- **Creates:**
  - `internal/loomcli/validate.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/loomcli/validate.go` in `package loomcli`, holding `(*loomCLI).validateDiscussionCmd() *cobra.Command` and `(*loomCLI).validatePlanCmd() *cobra.Command`, both modelled on `internal/loomcli/status.go`'s `statusCmd` shape: a `*cobra.Command` with `Use`, a non-empty `Short`, a `Long` carrying a concrete example line, and a `RunE` whose **first** statement is `if clihelp.ShouldAbort(cmd.Context()) { return nil }`, followed by `out := cmd.OutOrStdout()`.
  Neither command declares any flag, and both set `Args: cobra.NoArgs` — per the discussion's `zero-argument-verbs` decision, a path override is the one mechanism by which the self-check and the gate could be pointed at different files.

  `validateDiscussionCmd` has `Use: "validate-discussion"`.
  Its `RunE` calls `discussionparser.Validate(c.env.DecisionRecordPath, c.env.SupportLogPath)` once — the same function `loomshed.discussionValidate.Call` calls, per the `shared-implementation-is-the-whole-point` Shared Decision — and maps the result per the `envelope-and-exit-contract` Shared Decision: a non-nil error emits `output.Err` with a message naming the failure and the decision-record path;
  a non-empty findings slice emits `output.ErrFields` with a summary message and a `findings` key;
  zero findings emits `output.Ok` with a `decision_record` field carrying `c.env.DecisionRecordPath` and a `support_log` field carrying `c.env.SupportLogPath`.
  Every emission goes through `clihelp.SetExit(cmd.Context(), ...)`, exactly as `statusCmd` does, and `RunE` returns nil in all cases.

  `validatePlanCmd` has `Use: "validate-plan"`.
  Its `RunE` makes the same three calls, in the same order, that `loomshed.planValidate.Call` makes: `planparser.PlanDir(c.env.AnchorPath)`, then `planparser.ParsePlan(planDir)`, then `planparser.Validate(plan, c.env.WorktreeRoot)`.
  A `ParsePlan` error emits `output.Err` and carries no `findings` key;
  a non-empty `[]planparser.ValidationError` emits `output.ErrFields` with the `findings` key;
  an empty slice emits `output.Ok` with a `plan_dir` field carrying the resolved plan directory.
  Do not extract a shared wrapper from `internal/loomshed` for this — `internal/planparser` is already the one shared implementation, and what differs between the two call sites is only the outcome mapping.

  Add one unexported helper in the same file, used by both `RunE` bodies, that turns a slice of values carrying an `Error() string` method into the `[]string` payload placed under the `findings` key, so both verbs render their findings identically per the `findings-render-as-error-strings` Shared Decision.
  A generic function constrained to `interface{ Error() string }` is the natural shape;
  two small type-specific helpers are acceptable if that reads better in review.
  The `findings` key must be present on every findings failure and absent on every I/O-fault failure — batch 4's parity tests key their three-way comparison off exactly that structural difference, so it is never a matter of message wording.

  In `internal/loomcli/cli.go`, extend the existing `parent.AddCommand(...)` call to `parent.AddCommand(c.runCmd(), c.driveCmd(), c.statusCmd(), c.pauseCmd(), c.validateDiscussionCmd(), c.validatePlanCmd())`, and extend the parent's `Long`: add a sentence to the prose describing the two verbs as the standalone form of the `Discussion-Validate` and `Plan-Validate` mechanical gates, callable by the writer agent before handoff, and add `lyx loom validate-discussion` and `lyx loom validate-plan` to the trailing `Example:` block.
  Help accuracy is a review obligation under the CLI / Cobra Invariant, so both halves are required, not optional.
  Do not change `resolvePersistentPreRun`, `wire`, `RunCLI`, or `RunCLIIn` — the verbs deliberately reuse the existing wiring unchanged, and the pre-run's `cmd.Name() == "loom"` guard already gives both new verbs a wired `c.env` for free.
- **Commit:** `feat(loomcli): add validate-discussion and validate-plan self-check verbs`

### Card 6: tier-1 tests for both verbs

- **Context:**
  - `internal/loomcli/cli_test.go`
  - `internal/loomcli/validate.go`
  - `internal/loomcli/wiring.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/discussionparser/validate.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/testdata/goodplan/00-overview.md`
  - `internal/planparser/testdata/goodplan/01-json-flag.md`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/validate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/loomcli/validate_test.go` in `package loomcli`, with no build tag, driving both verbs through the hand-populated-receiver mechanism `TestVerbRefusals` already establishes in `internal/loomcli/cli_test.go`: build a `c := &loomCLI{env: shedrecipe.Env{...}}` whose `DecisionRecordPath`, `SupportLogPath`, `AnchorPath`, and `WorktreeRoot` point into a `t.TempDir()`, then run `clihelp.Execute(c.validateDiscussionCmd(), &out, nil)` or `clihelp.Execute(c.validatePlanCmd(), &out, nil)`.
  Do not call `RunCLIIn` and do not call `wire`, per the `cli-tests-never-go-through-runcliin` Shared Decision.

  Every case asserts three things: the exit code, that the captured output is **exactly one** JSON line (split on newline, discard a single trailing empty element, require length one — this is what protects the one-envelope-per-invocation rule the smoke suites' single-object unmarshal depends on), and the decoded envelope's `ok` value.
  Decode with `encoding/json` into a `map[string]any` so the presence or absence of the `findings` key can be asserted directly rather than by substring.

  Cases for `validate-discussion`:

  - clean — a decision record carrying all seven required headings plus a support log, both written under the temp dir: exit 0, `ok: true`, no `findings` key.
  - findings — the support log absent: exit 1, `ok: false`, a `findings` key present whose rendered entries name the missing support log.
  - findings — a required heading removed from an otherwise valid decision record: exit 1, `ok: false`, a `findings` key naming that heading specifically, proving the payload carries *which* heading failed rather than a bare "failed".
  - I/O fault — the decision-record path created as a directory while the support log exists: exit 1, `ok: false`, and **no** `findings` key.

  Cases for `validate-plan`:

  - clean — a minimal but genuinely valid plan written under `<AnchorPath>/_lyx/plan/`, so `planparser.PlanDir` finds it: a `00-overview.md` with `format: 3`, `approved: true`, an empty `root:`, a one-entry Card Index, and one `01-<slug>.md` card file, with every path the card names materialized under `WorktreeRoot` so the existence-dependent checks pass.
    Copy the field shape from `internal/planparser/testdata/goodplan/00-overview.md` and `internal/planparser/testdata/goodplan/01-json-flag.md` rather than inventing it, and mind that the `Commit:` line must match the card-number/slug form those fixtures use or `commit-subject-mismatch` fires.
    Assert exit 0, `ok: true`, no `findings` key.
  - findings — the same fixture with `approved: false` in the overview frontmatter, which trips `plan-unapproved`: exit 1, `ok: false`, a `findings` key present.
  - parse fault — no `_lyx/plan/` directory at all under `AnchorPath`, so `ParsePlan` errors: exit 1, `ok: false`, and **no** `findings` key.

  Write a small fixture builder per verb, used by every case in that verb's table, so no two cases can drift apart on the fixture shape.
  Keep both builders exported nowhere — they are test-local helpers in this package, and batch 4's parity tests reuse them by name rather than defining a second set.
  Every case is tier 1: no `exec.Command`, no `gitexec`, no `hubforge.NewHub`, no `time.Sleep`.
- **Commit:** `test(loomcli): cover both validate verbs' envelope, exit code, and findings payload`

### Card 7: exact-set registration guard and help-tree pin

- **Context:**
  - `internal/loomcli/cli.go`
- **Edits:**
  - `internal/loomcli/cli_test.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/loomcli/cli_test.go`, replace `TestCommand_AllFourVerbsRegistered` with `TestCommand_RegisteredVerbs_ExactSet`, a genuine exact-set assertion rather than the current subset check.
  Today's body builds `want := map[string]bool{"run": false, "drive": false, "status": false, "pause": false}`, marks only names already in that map, and asserts those four are present — so two extra verbs pass it unchanged, and a stray seventh verb would pass it too.
  The replacement collects `sub.Name()` for every entry in `parent.Commands()` into a sorted `[]string`, and compares it against the sorted literal `[]string{"drive", "pause", "run", "status", "validate-discussion", "validate-plan"}`, failing with both the got and want slices on any difference.
  Report a missing verb and an unexpected extra verb as distinct failures so the message says which kind of drift happened.
  Update the test's doc comment and the file's header comment to describe the exact-set guarantee.

  Note for the implementer, so the assertion is not written against the wrong set: cobra auto-adds a `help` command lazily, on `Execute`/help generation, not at `AddCommand` time, so `Command().Commands()` on a freshly built tree returns only the six explicitly registered verbs.
  If the built tree turns out to include a `completion` or `help` entry in this repo's cobra version, filter those two names out before comparing rather than pinning them — they are cobra's, not loom's, and pinning them would make the guard brittle across a cobra upgrade.

  In `cmd/lyx/helptree_test.go`, extend the `loom` entry's `wantSubs` from `[]string{"run", "drive", "status", "pause"}` to also carry `"validate-discussion"` and `"validate-plan"`.
  These assertions are documented superset checks, so the test would not fail without the addition — this is deliberate coverage, not a forced update, and the same is true of the `cli_test.go` tightening above.
  Do not touch `cmd/lyx/drift_test.go`, `cmd/lyx/longlist_test.go`, `cmd/lyx/registration_test.go`, or `cmd/lyx/seamsignature_test.go`: the first three derive their expectations from the live tree and need no per-verb edit, and the fourth pins module seam signatures, which this task does not change.
- **Commit:** `test(loomcli): tighten the registered-verb guard into an exact set and pin the new verbs in the help tree`

## Batch Tests

`verify: go test ./internal/loomcli/... ./cmd/lyx/...` covers `internal/loomcli/validate_test.go` (both verbs' clean, findings, and fault cases plus the one-envelope assertion), `internal/loomcli/cli_test.go` (the new exact-set registration guard and the untouched `TestCommand_EveryCommandHasShort`, which now walks the two new commands and would fail on an empty `Short`), and `internal/loomcli/status_test.go`, `bootstrap_test.go`, `seedinput_test.go`, and `wiring_test.go`, which share the package and must keep passing.
`./cmd/lyx/...` covers `helptree_test.go`'s extended `loom` entry — which exercises the real registered tree, so it also catches a verb added to `validate.go` but never passed to `AddCommand` — and `tierpurity_test.go`, whose `TestTierPurity_UntaggedTestsSpawnNothing` scans the new untagged test file for banned spawn tokens.
The `//go:build smoke` file in `internal/loomcli` is not built by this command and is deliberately not extended: a genuine end-to-end run of either verb through `RunCLIIn` against a `hubforge.NewHub` fixture is explicitly optional per the discussion, and `wiring_test.go`'s `TestWire_PathFieldsMatchLoomengineAccessors` already pins separately that `c.env`'s four path fields come from the same `loomengine` accessors the producer rows use.
