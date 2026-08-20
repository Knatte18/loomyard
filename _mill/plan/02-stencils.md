# Batch: stencils

```yaml
task: 'Bouncer: the generic review-gate producer'
batch: 'stencils'
number: 2
cards: 1
verify: go test ./contracts/stencils/...
depends-on: [1]
```

## Batch Scope

This batch ships the two generic Bouncer prompt templates and registers them, so `stencilstore.Reconcile` seeds them and `stencilstore.Read` can find them at call time.
It is one card rather than three because `contracts/stencils/registry_test.go`'s `TestRegistry_MatchesOnDiskTree` fails in both directions: a `.md` on disk with no registry row fails, and a registry row with no `.md` fails.
Creating the files and registering them in separate commits would leave the suite red in between.
The external interface batch 3 consumes: the registered stencil names `bouncer-template-seed` and `bouncer-template-judge`, and each template's exact marker set.
No batch-local decisions differ from `## Shared Decisions` in the overview.

## Cards

### Card 4: the two Bouncer templates, registered

- **Context:**
  - `contracts/stencils/treadle/treadle-template-judge-circling.md`
  - `contracts/stencils/burler/burler-step-2-review.md`
  - `internal/stencil/stencil.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/shedadapters/bouncerfiles.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `contracts/stencils/stencils.go`
  - `contracts/stencils/registry_test.go`
- **Creates:**
  - `contracts/stencils/bouncer/bouncer-template-seed.md`
  - `contracts/stencils/bouncer/bouncer-template-judge.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `contracts/stencils/bouncer/bouncer-template-seed.md`, the prompt for the seed call's focus-setting pass.
  Open it with an HTML comment header in the shape `contracts/stencils/treadle/treadle-template-judge-circling.md` already uses: name which Go call site fills it, state that every marker is a top-level `{{.X}}` substitution, and state that there are no `{{if}}` or `{{range}}` conditionals anywhere in the file.
  Its marker set is exactly four, and no more: `{{.rubric}}`, `{{.artifacts}}`, `{{.round}}`, `{{.focus_path}}`.
  The body must: place the rubric via `{{.rubric}}` as the judging criteria; tell the agent that `{{.artifacts}}` is a newline-separated list of absolute paths to the artifacts under review and that it must read each one; state that no round has been reviewed yet and that this call's only job is to set the initial focus for round `{{.round}}`; and require exactly one output file, `{{.focus_path}}`.
  State the focus-file format in full: `---`-delimited YAML frontmatter carrying `round` (a positive integer, here `{{.round}}`), `exclude_lenses` (a list of strings, possibly empty), and `focus` (a list of strings, possibly empty), over optional prose rationale below the closing delimiter.
  State that both list keys are always present even when empty, that the format is enforced by the parser in `internal/shedadapters/bouncerfiles.go`, and that a file the parser rejects is discarded and replaced with an empty-lists fallback.
  Instruct the agent to write only that one file.

  Create `contracts/stencils/bouncer/bouncer-template-judge.md`, the prompt for a judge call on round `{{.round}}`, with the same header-comment shape.
  Its marker set is exactly eight, and no more: `{{.rubric}}`, `{{.artifacts}}`, `{{.round}}`, `{{.report_path}}`, `{{.previous_ledger}}`, `{{.verdict_path}}`, `{{.ledger_path}}`, `{{.focus_path}}`.
  The body must: place the rubric via `{{.rubric}}`; tell the agent to read each absolute path in `{{.artifacts}}`, then the round's report at `{{.report_path}}`, then the previous ledger at `{{.previous_ledger}}`; and state that the literal value `(none)` in `{{.previous_ledger}}` means this is the first round and there is no prior ledger to read.

  The judge template must require **exactly three** output files on **every** call, and must say so as a blocking rule with its reason: `{{.verdict_path}}`, `{{.ledger_path}}`, and `{{.focus_path}}`.
  State that an `APPROVED` verdict still writes `{{.focus_path}}`, with an empty `exclude_lenses` and an empty `focus`, because the run is classified complete only when every declared output file exists — a judge that writes two of three files has its approval discarded.

  State each of the three formats in full.
  Verdict file: `---`-delimited YAML frontmatter carrying `verdict`, exactly `APPROVED` or `BLOCKING` case-sensitively, and `rationale`, a non-empty double-quoted single-line YAML string, over unconstrained human-facing prose.
  Require the double-quoted single-line rationale explicitly and say why, in the same shape `contracts/stencils/treadle/treadle-template-judge-circling.md` already does: an unquoted rationale containing a colon-space is invalid YAML, so the whole verdict file is rejected and the verdict is discarded.
  Ledger file: frontmatter carrying `round` (a positive integer, here `{{.round}}`) and `ledger`, a list of entries each with a non-empty `key`, a non-empty `rounds` list of positive integers, and a `status` of exactly `open` or `resolved`, over a distilled cross-round prose narrative.
  State the carry-forward rule prominently as a blocking rule: every entry present in the previous ledger reappears in this one, as `open` or as `resolved`, never dropped, because losing a recurring finding breaks the cross-round record for every later call.
  Focus file: the same format section the seed template carries, in byte-identical wording.
  Name `internal/shedadapters/bouncerfiles.go`'s parsers as the authority all three formats are enforced by.

  Register both in `contracts/stencils/stencils.go`: add a `BouncerTemplateSeed []byte` var with a doc comment and a `//go:embed bouncer/bouncer-template-seed.md` directive, and a `BouncerTemplateJudge []byte` var the same way, placed after the burler vars and before the treadle vars so the file's family grouping stays alphabetical-by-family as it reads today.
  Add the two matching rows to `entries` in the same relative position: `{"bouncer-template-seed", &BouncerTemplateSeed}` and `{"bouncer-template-judge", &BouncerTemplateJudge}`.
  The `bouncer-` name prefix is load-bearing: `stencilstore.RelPath` derives the family subfolder from the name's first token, so a name whose first token is not `bouncer` would resolve to a path that does not exist.

  Add `TestRegistry_IncludesBouncerStencils` to `contracts/stencils/registry_test.go`: assert that `Registry().Names()` contains both `bouncer-template-seed` and `bouncer-template-judge`, and that `Registry().Default` returns non-empty bytes and `true` for each.
  The two existing tests already cover the pair generically; this named case is what makes a later accidental removal fail with a message naming the Bouncer rather than as a diff in a generic list.
- **Commit:** `feat(stencils): add and register the two generic Bouncer prompt templates`

## Batch Tests

`verify: go test ./contracts/stencils/...` runs `registry_test.go`, whose two existing tests pin the registry against the on-disk tree in both directions and check that every registered name's `stencilstore.RelPath` resolves to a file that exists.
That is exactly the coverage a newly-registered stencil needs, and the new `TestRegistry_IncludesBouncerStencils` adds the named assertion.
The templates' marker sets are not verified here — they are pinned in batches 3 and 4 by the marker-completeness tests, which fill each template through `stencil.Fill` and assert the Go call site supplies every marker the template declares.
