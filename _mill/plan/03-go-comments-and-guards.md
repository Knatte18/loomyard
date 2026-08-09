# Batch: go-comments-and-guards

```yaml
task: 'plan-format: drop the v3 suffix and sweep every reference by script'
batch: go-comments-and-guards
number: 3
cards: 4
verify: go test ./internal/planparser/... ./internal/webstercli/... ./internal/websterengine/...
depends-on: [1]
```

## Batch Scope

This batch erases the Go-side residue the sweep cannot reach: the four surviving bare-`v3` format labels in `internal/planparser` comments, every plan-format-**v2** reference in Go comments across `internal/planparser`, `internal/websterengine`, and `internal/webstercli`, and the two v2-named test guards whose names and messages describe a version rather than the concepts they assert.
It is one batch because all three packages carry the same single edit shape — a comment or identifier that names a dead version number gets rewritten to name the thing it is actually about — and because the two test renames must land together with the message and comment changes that reference them.
It depends on batch 1 because the sweep already rewrote the path-bearing half of several of these same comment blocks;
it runs in parallel with batch 2, which owns the markdown half of the same erasure and shares no file with this batch.

Batch-local decision beyond `## Shared Decisions`: `v2-guards-keep-assertions-lose-the-label`.
Both v2-named guards keep every concept assertion they make — `oversized`, `chain`, `## Scope`, `out_of_scope:`, `tests: green`.
Those assertions guard concrete banned constructs in LLM-authored prompt templates and cobra help strings, files agents edit, and they stand on their own with no reference to a version number.
Only the names, comments, and the one genuinely meaningless `"v2"` string literal change.
Deleting the guards outright was rejected: they are the only machine check stopping batch-era language from creeping back into CLI help and the embedded templates.

## Cards

### Card 9: Rewrite planparser's bare-v3 labels and its one v2 comment

- **Context:** none
- **Edits:**
  - `internal/planparser/validate.go`
  - `internal/planparser/validate_test.go`
  - `internal/planparser/parse_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite each comment below so it names the format's actual property instead of a version number.
  Locate each by its quoted text.
  These are comments only — no exported or unexported Go identifier in this package contains `V3`, so there is no code rename to perform here.

  1. `validate.go`, in the package-level file comment on `Findings are keyed by card`:

     ```
// (flat `N-<slug>`), not batch — v3 has no batch concept — and there is no ValidateCaps: the
// oversized-batch cap dies with batch itself.
     ```

     This one site carries both a bare `v3` label and a v2-era contrast, so rewrite it once covering both: findings are keyed by card, the format has no batch concept, and there is no `ValidateCaps` because there is no oversized-batch cap to configure.
     State it as a property of the format, not as a delta from anything.
  2. `validate_test.go`, in `TestValidate_MoveRedundant`'s doc comment:

     ```
// card) and a Creates: target (on another) anywhere in the plan — the plan-wide scope v3 uses since
// batch is gone.
     ```

     Rewrite the trailing clause to name the scope directly: the check's scope is plan-wide because there is no batch to scope it to.
  3. `parse_test.go`, `minimalOverview`'s doc comment — "a syntactically complete v3 overview with a single Card Index entry" → drop the version word and describe the fixture as a syntactically complete overview.
  4. `parse_test.go`, `minimalCardFile`'s doc comment — "a syntactically complete v3 card file body: all five typed file-op fields plus Depends-on…" → same treatment.
  5. `parse_test.go`, inside the test body:

     ```
	// Unlike the frozen v2 parser, a missing format:/approved: key is not a
	// ParsePlan failure — format-unrecognized/plan-unapproved are Validate's
	// checks, not the parser's; a plan simply parses with the zero value.
     ```

     Drop the "Unlike the frozen v2 parser," opener and state the rule directly.

  Do **not** touch the `format: 3` literal in `minimalOverview`'s frontmatter or anywhere else — that is the schema's own version field, not the document's name (`schema-version-field-is-not-the-doc-name`).
  Do **not** touch any `gopkg.in/yaml.v3` import line.
  Change no assertion, no fixture content, and no identifier;
  this card is comment text only.
- **Commit:** `refactor(planparser): name the plan format's properties instead of a version`

### Card 10: Erase v2 from websterengine's comments and rename the template guard

- **Context:** none
- **Edits:**
  - `internal/websterengine/classify.go`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the following, locating each by its quoted text.

  1. `classify.go`, in the package-level file comment:

     ```
// The v2 Scope field is dropped entirely — the flat plan format carries no `## Scope`, so there is
// nothing for a changed file to be judged against beyond the fork's own informational deviation
// list.
     ```

     Keep the second half only: the flat plan format carries no `## Scope`, so there is nothing for a changed file to be judged against beyond the fork's own informational deviation list.
     Drop the "The v2 Scope field is dropped entirely" opener.
  2. `template_test.go`, `requireNotContains`'s doc comment — "used to pin the absence of every dropped v2 concept (oversized, chain, ## Scope) and every concept the fork-context-hygiene Shared Decision moved out of the thin fork prompt" → replace "every dropped v2 concept" with a description of what the three named constructs are: dropped batch-era concepts. Keep the parenthetical list and the second half of the sentence intact.
  3. `template_test.go`, `TestForkTemplate_PinsReportSchemaKeys`'s doc comment — "(status, head_sha, deviations — never the v2 report's tests/stuck_reason/out_of_scope grammar)" → drop the version word; the superseded report grammar is what is being excluded, and naming its fields is what makes the sentence useful.
  4. `template_test.go`, the inline comment above the two `requireNotContains` calls — "The v2 report grammar (done/stuck/tests/out_of_scope) must be gone — the report is deliberately minimal under the flat card-list model." → same treatment: name the superseded grammar without the version word.
  5. `template_test.go`, rename the test function `TestTemplates_NoV2TokensRemain` to `TestTemplates_NoDroppedBatchConceptsRemain` and rewrite its doc comment, which currently reads "asserts neither embedded template carries any of the three dropped v2 concepts — oversized batches, deferred-verify chains, and the per-batch "## Scope" section — anywhere in its bytes", so it names the three concepts without calling them v2's.
     Grep for the old name across the repo first and retarget any other reference;
     the rename is only complete when nothing cites the old identifier.

  Keep every assertion in the file exactly as it is.
  `TestTemplates_NoV2TokensRemain` never asserts on the string `"v2"` at all — it asserts `oversized`, `chain`, and `## Scope` — and `TestForkTemplate_PinsReportSchemaKeys` asserts `out_of_scope:` and `tests: green`.
  Those are concrete banned constructs in agent-edited prompt templates and they stand alone;
  only the naming changes (`v2-guards-keep-assertions-lose-the-label`).
- **Commit:** `refactor(websterengine): name dropped batch concepts instead of the v2 version`

### Card 11: Drop the meaningless v2 token from the CLI help guard and rename it

- **Context:**
  - `cmd/lyx/helptree_test.go`
- **Edits:**
  - `internal/webstercli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three changes in `TestCommand_LongStringsHaveNoStaleV2Language`:

  1. Rename the function to `TestCommand_LongStringsHaveNoStaleBatchLanguage`. Grep for the old name across the repo first and retarget any other reference.
  2. Remove the `"v2"` entry from the `forbidden` slice, leaving `[]string{"--restart-chain", "restart-chain", "chain", "oversized"}`. This is the one genuinely asserted `"v2"` token in the tree, and it guards against reintroducing a label whose referent no longer exists — exactly the stale guard this task's rationale rejects. The four surviving entries all name real banned constructs and stay.
  3. Rewrite the `t.Errorf` message, which currently reads `"command %q Long string contains stale v2/chain/oversized language %q:\n%s"`, so it names the batch-era constructs rather than the version.

  Change nothing else in the test: the cobra walk, the `strings.ToLower` normalization, and the recursion over subcommands all stay.
  The CLI/Cobra Invariant applies — every command keeps its `Short`, and `cmd/lyx/helptree_test.go` must stay green.
- **Commit:** `refactor(webstercli): guard against batch-era help language, not the v2 label`

### Card 12: Confirm the Go-side erasure gates

- **Context:**
  - `internal/state/state_test.go`
  - `internal/gitrepo/reset_test.go`
  - `internal/yamlengine/reconcile_test.go`
  - `internal/shuttleengine/claudeengine/command.go`
  - `internal/burlerengine/doc.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Zero-diff verification card for this batch's half of the erasure.

  1. Acceptance gate 5 — `grep -rni 'v3' internal/planparser/` returns **only** `gopkg.in/yaml.v3` import lines. All four bare labels are gone.
  2. Acceptance gate 3, for this batch's share — `grep -rniE '\bv2\b' --include='*.go' internal/` returns only the deliberately-untouched sites listed below, and nothing in `internal/planparser`, `internal/websterengine`, or `internal/webstercli`.
  3. The batch `verify:` command is green, and `go build ./...` is clean.

  The deliberately-untouched `v2` sites, none of which are plan-format references — do not edit them and do not file them as findings: `internal/state/state_test.go`, `internal/gitrepo/reset_test.go`, `internal/yamlengine/reconcile_test.go`, `internal/shuttleengine/claudeengine/command.go`, and `internal/burlerengine/doc.go`.
  Every `v1`/`V1` in the tree is likewise out of scope in full (`v1-is-out-of-scope-entirely`).
- **Commit:** none

## Batch Tests

`verify:` runs `go test` over exactly the three packages this batch edits — `internal/planparser`, `internal/webstercli`, `internal/websterengine` — which is per-batch scoping, not the unbounded suite.
The whole scoped run completes in well under a second.

What each covers against this batch's specific risk, which is a rename or comment edit that accidentally changes behaviour:

- `internal/websterengine/template_test.go` — the renamed `TestTemplates_NoDroppedBatchConceptsRemain` and `TestForkTemplate_PinsReportSchemaKeys` are themselves in this package, so a broken rename or a dropped assertion shows up as a compile failure or a missing test rather than silently passing.
- `internal/webstercli/cli_test.go` — same for the renamed CLI guard;
  it also still walks every cobra `Long` string, so removing `"v2"` from `forbidden` cannot mask a reintroduced `chain` or `oversized`.
- `internal/planparser` — full parse/validate coverage proves the comment-only edits in `validate.go`, `validate_test.go`, and `parse_test.go` changed no fixture and no assertion.

`cmd/lyx/helptree_test.go` is not in this batch's `verify:` scope because this batch edits no cobra `Long` string — batch 1 owned those, and its own `verify:` covered the help tree.
The overview's module-wide `verify: go build ./...` runs at the batch boundary, and batch 4's gate re-runs the full suite.

No new test (`no-new-tests`).
