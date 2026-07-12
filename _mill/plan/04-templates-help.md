# Batch: templates-help

```yaml
task: "Add typed file-ops to lyx's plan-format"
batch: "templates-help"
number: 4
cards: 3
verify: go test ./internal/builderengine/... ./internal/buildercli/...
depends-on: [1, 2, 3]
```

## Rename mechanic

Not applicable — this batch has no `Moves:` entries.

## Batch Scope

This batch retargets every prompt template and CLI help text from plan-format v1 to
v2: the embedded implementer prompt (typed fields, `NN.C` card headings, pinned
`Commit:` fallback, read-the-overview rule, rename-mechanic compliance), the
orchestrator prompt's version references, and `lyx builder`'s Cobra `Short`/`Long`
strings (help accuracy is a review obligation per the CLI/Cobra Invariant). Depends
only on batch 1 (field names and heading grammar); it does not touch `validate.go`
or `plan.go`, so it cannot collide with batches 2-3.

Batch-local decision: help text describes the check set count-neutrally ("the
plan-format machine checks") so batch 3's additions never stale it again.

## Cards

### Card 13: implementer-template.md rewritten for v2

- **Context:**
  - `_mill/discussion.md`
  - `internal/builderengine/spawn.go`
  - `internal/builderengine/plan.go`
- **Edits:**
  - `internal/builderengine/implementer-template.md`
  - `internal/builderengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Rewrite the prose only — the five stencil markers (`{{.batch_file}}`,
    `{{.batch_name}}`, `{{.worktree_root}}`, `{{.self_fix_cap}}`,
    `{{.report_path}}`) and the no-conditionals rule are unchanged (`spawn.go`'s
    `SpawnBatch` fill contract).
  - "plan-format v1" -> v2 throughout, including the banner comment.
  - Replace the "read it, and only it" section: the implementer now reads its
    batch file AND `00-overview.md` (task framing, Batch Index, `## Shared
    Decisions`) — but still never another batch's file (discussion
    `implementer-reads-overview`).
  - Replace the What/Where card instructions with the v2 field semantics: work
    card by card in `NN.C` order; `Context:` is the advisory read-list; make
    exactly the changes `What:` describes in the files `Edits:`/`Creates:`/
    `Deletes:`/`Moves:` declare; `//`-prefixed and `root:`-relative paths per the
    batch's frontmatter (state the three-case rule in one sentence).
  - Rename-mechanic compliance: when a card has `Moves:` pairs, run
    `git mv <old> <new>` FIRST, then only surgical edits; never rewrite-and-
    recreate (the batch's `## Rename mechanic` section is binding).
  - Commit-subject rule: use the card's `**Commit:**` value verbatim when present;
    otherwise derive `NN.C: <short what>` exactly as v1 describes. The heading
    `### Card NN.C — <title>` matches the commit subject 1:1.
  - Scope/out-of-scope reporting language updates from "Scope and Where lines" to
    "Scope and the card's declared files"; the batch-report contract section is
    unchanged.
  - Update `template_test.go`'s implementer-template assertions (e.g.
    `TestImplementerTemplate_StatesBatchDiscipline`) to pin the NEW prose facts:
    reads-overview-but-not-other-batches, the five field names, the `git mv`-first
    rule, the `Commit:`-verbatim rule, and the surviving markers. Follow the
    file's existing assertion style; drop pins that asserted v1-only wording
    (never-read-overview, `**Where:**`).
- **Commit:** `04.1: implementer template speaks plan-format v2`

### Card 14: orchestrator-template.md version references

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/builderengine/orchestrator-template.md`
  - `internal/builderengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Update "plan-format v1" wording (line 13 banner region and any other
    occurrence) to v2. The orchestrator drives verbs, not card fields — beyond the
    version wording, change only sentences that concretely describe v1-only card
    shape, if any exist on read-through.
  - Adjust any `template_test.go` orchestrator assertions only if they pin the
    changed wording (read the three `TestOrchestratorTemplate_*` tests; touch
    nothing that still holds).
- **Commit:** `04.2: orchestrator template v2 wording`

### Card 15: buildercli help text v2

- **Context:**
  - `_mill/discussion.md`
  - `internal/builderengine/validate.go`
  - `internal/buildercli/cli_test.go`
- **Edits:**
  - `internal/buildercli/cli.go`
  - `internal/buildercli/validate.go`
  - `internal/buildercli/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `cli.go` line ~95 `Long`: "pinned plan-format v1 plan" -> v2.
  - `buildercli/validate.go`: `Short` "lint the plan against the six plan-format
    machine checks..." -> count-neutral "lint the plan against the plan-format
    machine checks without running anything"; `Long` and the file banner comment
    likewise drop "v1" and "six" in favor of "plan-format v2 machine checks"
    (batch-local decision: count-neutral wording). The doc comment naming
    "Scope/Where file" resolution updates to the typed-fields wording.
  - Re-read every remaining `Short`/`Long` in `internal/buildercli` for v1/Where
    staleness and fix in place (help accuracy is a review obligation —
    CONSTRAINTS.md CLI/Cobra Invariant). `cli_test.go` is Context because grep
    shows no pinned help strings — confirm on read and leave it untouched if so.
  - `validate_test.go` moved from Context to Edits: batch 2's
    `move-source-missing`/`move-target-collision` on-disk checks (commit
    `71b6242`) resolve every card file-op path against `worktreeRoot`, but
    `seedPlanFixture` here only copies the `plan-valid`/`plan-unapproved`
    fixture files into the seeded hub's `_lyx/plan` dir, never into the hub
    root that `c.layout.Cwd` (this package's `worktreeRoot`) resolves against —
    unlike `builderengine`'s own tests, which pass the fixture directory
    itself as `worktreeRoot` per the `fixture-self-reference` decision. This
    regressed `TestRunCLI_Validate_CleanPlan` and every other `plan-valid`-
    seeded buildercli test the moment batch 2 landed
    `move-source-missing` (confirmed via `git worktree` bisection: green at
    `f94dd01`/`c3e844a`, red starting at `71b6242`) — a same-task regression
    in this batch's own `verify:` dependency chain, not a pre-existing
    failure, so it is fixed here rather than reported as pre-existing.
    `seedPlanFixture` additionally copies the same fixture entries to `hub`
    itself (the worktree root), alongside the existing `_lyx/plan` copy, so
    on-disk card-path resolution succeeds the same way it does for
    `builderengine`'s own fixture-self-reference tests.
- **Commit:** `04.3: buildercli help text for plan-format v2`

## Batch Tests

`verify:` runs `go test ./internal/builderengine/... ./internal/buildercli/...` —
`template_test.go` re-pins the rewritten implementer/orchestrator prose (card 13/14),
and the buildercli suite proves the help-text edits break no envelope/JSON test.
Two packages, both directly touched — still narrowly scoped per
`package-scoped-verify` (no `go test ./...`).
