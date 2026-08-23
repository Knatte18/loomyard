# Batch: gate parity tests

```yaml
task: 'loom: self-checkable mechanical gates'
batch: 'gate parity tests'
number: 4
cards: 1
verify: go test ./internal/loomcli/... ./internal/lyxcwd/...
depends-on: [2, 3]
```

## Batch Scope

This batch is the task's own guard: one parity test per mechanical gate, each driving the producer path and the CLI path over the same fixture set and asserting the two verdicts agree three-way, plus the `CONSTRAINTS.md` section that makes the rule binding on every mechanical gate added later.
It is one batch and one card because the test and the invariant it encodes are the same claim written twice — once for the machine, once for the reviewer — and `CLAUDE.md` requires a new cross-cutting invariant to land in the same commit as the change that introduces it.

It depends on batch 2 and batch 3 together: the producer half needs `loomshed.NewDiscussionValidate` already rewritten over the shared implementation, and the CLI half needs the verbs and their fixture builders.
Nothing consumes this batch's output — batch 5 documents the shipped surface.

Batch-local decision: both parity tests live in `internal/loomcli`, in one new `parity_test.go`, because that package may import `internal/loomshed`'s exported constructors while `loomshed` may not import a `<module>cli` package.
The direction of the dependency is what fixes the location;
it is not a preference.

## Cards

### Card 8: three-way parity tests and the Gate Self-Check Parity Invariant

- **Context:**
  - `internal/loomcli/validate.go`
  - `internal/loomcli/validate_test.go`
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/discussionparser/validate.go`
  - `internal/planparser/validate.go`
  - `internal/clihelp/exec.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:**
  - `internal/loomcli/parity_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/loomcli/parity_test.go` in `package loomcli`, with no build tag, holding `TestGateParity_DiscussionValidate` and `TestGateParity_PlanValidate`.

  Each test walks one fixture table.
  For each fixture it runs both halves over the **same** paths: the producer half constructs the producer via `loomshed.NewDiscussionValidate(name, decisionRecordPath, supportLogPath)` or `loomshed.NewPlanValidate(name, anchorPath, worktreeRoot)` and calls `Call(context.Background())`;
  the CLI half builds a hand-populated `&loomCLI{env: shedrecipe.Env{...}}` carrying those identical path values and runs `clihelp.Execute(c.validateDiscussionCmd(), &out, nil)` or `clihelp.Execute(c.validatePlanCmd(), &out, nil)`.
  Reuse the fixture builders card 6 wrote in `internal/loomcli/validate_test.go` rather than writing a second set — two fixture sets could drift and hide exactly the divergence these tests exist to catch.

  The comparison is **three-way, not binary**, per the discussion's `parity-tests-per-gate` decision.
  The producer has three outcomes and the verb has two exit codes, so map both sides onto one three-valued verdict before comparing.
  Producer side: `shedengine.Done` with a nil error is `verdictDone`;
  `shedengine.Stuck` with a nil error is `verdictStuck`;
  a non-nil returned error is `verdictError`.
  CLI side: decode the single JSON line into a `map[string]any`, then `ok == true` is `verdictDone`;
  `ok == false` **with** a `findings` key is `verdictStuck`;
  `ok == false` **without** a `findings` key is `verdictError`.
  Declare the three values as an unexported string-typed constant set in this file so a mismatch failure message names them readably.
  A binary pass/fail comparison would collapse `Stuck` and error onto the same side, which is precisely the distinction the `short-circuit-order-is-load-bearing` Shared Decision exists to protect — a parity test blind to it would pass while the two paths disagreed on the one case that motivated pinning the order.

  Each fixture table must contain at least one instance of **each** of the three verdicts, not merely a passing and a failing document.
  For the discussion gate: a clean fixture, a missing-support-log fixture and a missing-heading fixture, and a decision-record-is-a-directory fixture.
  For the plan gate: the valid minimal plan, the same plan with `approved: false`, and an absent `_lyx/plan/` directory.
  Assert the two mapped verdicts are equal, and on inequality report the fixture name, the producer's raw outcome and error, and the CLI's exit code and raw output line.

  Both halves take told paths and construct no repository, so both stay tier 1: no `exec.Command`, no `gitexec`, no `hubforge.NewHub`, no `RunCLIIn`.

  In `CONSTRAINTS.md`, add a `## Gate Self-Check Parity Invariant` section of its own — not a bullet inside the Discussionparser section card 2 added, because this rule binds both gates including the `planparser`-backed one, and binds every mechanical gate added later;
  a future gate author would have no reason to look for it under a `Discussionparser`-named heading.
  Place it immediately after the Discussionparser Sole-Parser Invariant so the mirrored Planparser/Discussionparser pair stays adjacent and symmetric.
  State the rule: a mechanical gate's `ShedProducer` row and its CLI self-check verb call the same package function, and neither re-implements the other's check.
  Bullets: name today's two instances and the shared function each pair calls;
  state that the verb's envelope distinguishes a findings failure from an I/O fault **structurally**, by the presence of the `findings` key, never by message wording, because that is what the three-way comparison keys off;
  and state that adding a mechanical gate means adding its verb and its parity test in the same task.
  Close with an **Enforced by** bullet naming `internal/loomcli/parity_test.go` (`TestGateParity_DiscussionValidate`, `TestGateParity_PlanValidate`), in the phrasing the neighbouring sections use.
  Use semantic line breaks, per the `markdown-semantic-line-breaks` Shared Decision.
- **Commit:** `test(loomcli): assert three-way gate/verb parity and record the Gate Self-Check Parity Invariant`

## Batch Tests

`verify: go test ./internal/loomcli/... ./internal/lyxcwd/...` covers `internal/loomcli/parity_test.go`'s two tests plus every other test in the package, which must keep passing — in particular `validate_test.go`, whose fixture builders this card reuses, so a builder changed here breaks both suites at once rather than silently serving one.
`./internal/lyxcwd/...` is included because this card edits `CONSTRAINTS.md`, and `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks` resolves `#anchor` targets into that file from every link under `manifest/` and `docs/`, so a new section heading changes the anchor set those links resolve against.
The parity tests are the task's own correctness guard rather than incidental coverage: nothing else in the repo asserts that the agent's self-check and the mechanical gate reach the same verdict, and a later refactor forking one of the two shared functions is exactly what they exist to fail on.
