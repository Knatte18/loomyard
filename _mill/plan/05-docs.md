# Batch: docs

```yaml
task: "Add typed file-ops to lyx's plan-format"
batch: "docs"
number: 5
cards: 3
verify: null
depends-on: [3]
```

## Rename mechanic

Not applicable — this batch has no `Moves:` entries.

## Batch Scope

This batch rewrites `docs/modules/plan-format.md` as the pinned **plan-format v2**
contract — the task's primary deliverable — and updates the two docs that reference
v1 (`docs/modules/builder.md`, `docs/overview.md`). It depends on batch 3 because the
doc's "Validation checks" list documents the final check set by name.
`docs/roadmap.md` is NOT touched: no planned milestone covers this task (verified —
the roadmap only cites plan-format.md as a contract reference), and CLAUDE.md forbids
roadmap notes for delivered work that is not a planned milestone.

Batch-local decision: the doc keeps v1's structure (same section order) so diffs read
as an evolution, with a short "Changes from v1" note in the status banner instead of
retaining any v1 body text.

## Cards

### Card 16: plan-format.md v2 — format specification

- **Context:**
  - `_mill/discussion.md`
  - `internal/builderengine/plan.go`
  - `internal/builderengine/validate.go`
- **Edits:**
  - `docs/modules/plan-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Retitle to "Plan format v2 — Builder's input contract"; status banner notes v2
    supersedes v1 (format bump, v1 plans are refused by `format-unrecognized`) and
    keeps the pinned-contract framing.
  - Overview frontmatter section: `format: 2`.
  - Batch Index section: the v2 entry shape
    `- NN — <batch-slug> (C cards) — <one-line intent>` with the mandatory card
    count, cross-checked by `card-count-mismatch`.
  - New "Shared Decisions" subsection in the `00-overview.md` section: optional
    `## Shared Decisions` with `### Decision: <short-name>` /
    `Decision:`/`Rationale:`/`Applies to:` shape; prose-only, unparsed, no
    validation check; read by the implementer (discussion
    `shared-decisions-section`). State explicitly that "All Files Touched" was
    evaluated and deliberately NOT adopted (one sentence, with the
    derivable-from-cards rationale).
  - Batch file section: optional `root:` frontmatter key with the three-case path
    rule (`root:`-relative by default, `//` always worktree-root-relative, Scope
    unaffected), verbatim per the `normalize-at-parse` Shared Decision.
  - Card section rewritten to the v2 grammar (copy the `v2-card-grammar` Shared
    Decision's shape exactly): `### Card NN.C — <title>` headings matching the
    commit-subject convention 1:1; `What:` prose; the five required file-op fields
    with `none` sentinels and backtick-bullet path grammar; advisory `Context:`
    semantics spelled out (read-list, not a cage — cite the existing Scope
    philosophy); optional `Commit:` (verbatim-use rule + `NN.C: ` prefix
    validation) and optional per-card `verify:`; per-card mutual exclusivity with
    the legitimate cross-card Creates-then-Edits pattern named.
  - New "Moves and the Rename mechanic" section: the `` `old` -> `new` `` pair
    grammar, rename-plus-extraction pattern, and the CANONICAL `## Rename
    mechanic` text (the four numbered rules from discussion
    `moves-grammar-and-rename-mechanic`: git mv first; surgical edits only;
    Creates only for genuinely new files; never rewrite-and-delete). State that
    every batch with a non-empty `Moves:` field MUST carry the section
    (`move-mechanic-missing`).
  - "Validation checks" section: the complete renumbered v2 list — the six
    existing checks (format/approval, index-file consistency, verify presence,
    chain-end soundness, batch-oversized with the typed-fields estimate, scope
    well-formedness now covering card paths) plus the new named checks:
    `move-format`, `move-redundant`, `move-source-missing`,
    `move-target-collision`, `move-mechanic-missing`, `card-missing-field`,
    `card-field-overlap`, `card-numbering`, `card-count-mismatch`, `path-missing`,
    `card-outside-scope`, `commit-subject-mismatch` — one line each, semantics as
    implemented in batches 2-3.
  - Restate the verify-scope guardrail as a design constraint with rationale:
    per-batch `verify:` stays package-scoped, never a full-suite run; no
    module-wide per-batch verify (a future module-wide gate must be
    baseline-aware and boundary-gated) — discussion `verify-scope-guardrail`.
  - Unchanged sections (sizing principle, Scope semantics, oversized/chains,
    batch-report, roles) are kept, re-read for stale `Where`/card wording, and
    updated only where they name v1 card shape.
- **Commit:** `05.1: plan-format.md v2 specification`

### Card 17: plan-format.md v2 — worked example

- **Context:**
  - `_mill/discussion.md`
  - `internal/builderengine/testdata/plan-valid/00-overview.md`
  - `internal/builderengine/testdata/plan-valid/01-json-flag.md`
  - `internal/builderengine/testdata/plan-valid/02-list-tests.md`
- **Edits:**
  - `docs/modules/plan-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Rewrite the worked example (same fictional `--json` task, byte-consistent
    across index <-> filenames <-> report as v1 demanded) so it demonstrates, in
    two to three batch files: all five file-op fields with at least one `none`
    sentinel; `### Card NN.C — <title>` headings; the `(C cards)` Batch Index
    segments; a `## Shared Decisions` entry in the overview; one batch using
    `root:` with a `//`-escaped path; one card with a `**Commit:**` field; and one
    `Moves:` card with its `## Rename mechanic` section (extend the fictional task
    with a small rename, e.g. `rows.go` -> `rowsjson.go`, so the example stays
    plausible).
  - The example must be internally consistent with every v2 validation check
    (as the fixture is — mirror `testdata/plan-valid`'s patterns where they fit;
    exact byte-sharing with the fixture is NOT required, self-consistency is).
  - Keep the batch-report example section as-is (schema unchanged).
- **Commit:** `05.2: plan-format.md v2 worked example`

### Card 18: builder.md + overview.md version references

- **Context:**
  - `_mill/discussion.md`
  - `docs/modules/plan-format.md`
- **Edits:**
  - `docs/modules/builder.md`
  - `docs/overview.md`
  - `docs/reference/model-spec.md`
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `docs/modules/builder.md`: line ~13 "pinned plan-format v1 plan" -> v2; the
    `validate` verb row ("the six plan-format v1 machine checks") -> count-neutral
    "the plan-format machine checks" matching the batch-4 help text; scan the rest
    of the file for `Where`/v1 card wording and update in place.
  - `docs/overview.md`: line ~264 "pinned plan-format v1 plan" -> v2; no module
    table changes (no module added or renamed).
  - `docs/reference/model-spec.md`: line ~5 "pinned alongside plan-format v1" ->
    v2 (the doc it links to is now v2; missed in the original touched-file scan).
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`: Scenario S9's hand-written example plan
    (format frontmatter, Batch Index entry, card heading, and fields) rewritten to
    valid plan-format v2 so its documented `{"ok":true,"valid":true,"batches":1}`
    walkthrough still holds under the new mandatory grammar; added to scope here
    per holistic review round 2 (file wasn't in the original touched-file scan).
- **Commit:** `05.3: builder.md, overview.md, model-spec.md, and sandbox suite reference plan-format v2`

## Batch Tests

`verify: null` — this batch edits markdown documentation only; there is no runnable
surface (the doc's worked example is validated by consistency with the batch-1
fixtures and by plan review, not by a test). The Go-touching batches (1-4) each carry
their own package-scoped `verify:`.
