# Batch: docs — spec rewrite, stencils, sandbox suite, stale figures, roadmap

```yaml
task: Migrate planparser.Card to Edits/Uses fields
batch: docs — spec rewrite, stencils, sandbox suite, stale figures, roadmap
number: 4
cards: 10
verify: go test ./internal/loomengine/... ./internal/webstercli/... ./internal/planparser/...
depends-on: [3]
```

## Batch Scope

This batch makes every document that describes the plan card format describe the format that now exists.
It rewrites `contracts/specs/loom-plan-spec.md` in full as `internal/planparser`'s as-built contract, reduces both stencils to their legitimate share of that contract, retargets the stencil-pinning tests in `internal/loomengine`, rewrites the agent-facing sandbox suite instruction, corrects two now-false comment passages and every remaining stale check-count figure, closes out the design doc's status banner and open items, and moves the roadmap item to Done.

It lands last because a spec is only an as-built contract once the build matches it, and it is one batch because the Documentation Lifecycle in `CLAUDE.md` requires the docs to land with the change rather than trail it.

The batch closes with a verification-only card that re-runs all three sweeps from `_mill/discussion.md` rather than trusting any hand list — including the sandbox suite, which is an agent-facing markdown instruction file that the whole-tree Go gate structurally cannot check.

Batch-local decisions beyond `## Shared Decisions`:

- **The stencil rewrites reduce duplication rather than translate it.** A mechanical field-name find-and-replace would preserve today's duplication in new vocabulary and re-commit the Producer Pointer-Rule violation.
  `loom-template-plan.md` keeps only its pinned LLM-facing subset;
  `webster-body-implementer.md` reduces to a pointer plus the deviation-union rule.
- **The two stale design docs are corrected, not reconciled.** `manifest/designs/scout-plan-symbol-fields.md` gets its check-count figure fixed as one site of a mechanical sweep and nothing else.
  Correcting a number is not reconciling a document, and skipping one site out of a sweep would be arbitrary.
- **`CONSTRAINTS.md` gains no entry.** The Planparser Sole-Parser Invariant is about who parses the format and who declares its path;
  it says nothing about the field set, so a field-set change leaves it untouched.
  This task introduces no new cross-cutting structural rule.

## Cards

### Card 17: rewrite loom-plan-spec.md

- **Context:**
  - `manifest/designs/plan-card-format.md`
  - `internal/planparser/plan.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/classify.go`
  - `internal/planparser/normalize.go`
  - `internal/planparser/testdata/goodplan/00-overview.md`
  - `internal/planparser/testdata/goodplan/01-json-row-type.md`
  - `internal/planparser/testdata/goodplan/02-json-flag.md`
  - `internal/planparser/testdata/goodplan/03-json-emission.md`
  - `internal/planparser/testdata/goodplan/04-legacy-rows-delete.md`
  - `internal/planparser/testdata/goodplan/05-rowmapper-rename.md`
  - `internal/planparser/testdata/goodplan/06-helppins-move.md`
  - `internal/planparser/testdata/goodplan/07-json-docs.md`
- **Edits:**
  - `contracts/specs/loom-plan-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the spec in full as format 4's pinned as-built contract, keeping its existing top-level structure and its status-banner convention.
  Correct the banner's "the fourteen checks below are already implemented" to sixteen, and keep the banner's existing statement that the LLM-facing subset lives separately in the producer's own stencil — that split survives this task unchanged.
  Keep the `## What a card is`, `## Batch is gone / the card is the unit`, `## Plan vs. schedule`, and `## On-disk layout` sections, editing only what the field-set change forces: the `format:` key's value becomes 4, and the sentence claiming a card's typed file-op fields are its declared footprint must instead say a card's own type-label target list is its declared footprint.
  Replace the `## Card fields and order` section with the format-4 card: a title heading, exactly one bold type label from the set `Create`, `Edit`, `Delete`, `Rename`, `Move`, `Prosa`, `Custom` whose own indented backtick-wrapped sub-bullets are the card's targets, an optional `**Uses:**` list of what the card reads but does not change, a required multi-line `**Intent:**`, an `**ImpactSummary:**` required for `Edit` and `Delete` only and taking its value inline on the label line, and the optional `**Commit:**` and `**Verify:**`.
  State that a field with no content is omitted and that no `none` sentinel is admitted on any field.
  State that the type name is the key and that there is no separate `Type:` field.
  Replace the `## Depends-on` section with a short section stating that dependency edges are derived by intersecting a card's `Uses:` against every other card's target list, never authored, and that no `DependsOn` or `Produces` field exists.
  Add a card-type table mirroring the design doc's, and state unambiguously that `ImpactSummary` is required for `Edit` and `Delete` only — the design doc's table row marking it required for `Create` as well is a drafting slip this spec resolves in favour of the doc's own prose, because a `Create` card has no existing callers to have a blast radius over.
  Add a section on the shape classifier stating the three-rule order — a separator makes it a path, otherwise an all-lowercase-alphanumeric final dot-segment makes it a path, otherwise it is a symbol — and stating the deliberate deviation from the design doc: the doc's "resolvable against ground truth (`go doc` for a symbol, file existence for a path)" clause is taken in its shape half only, because a process spawn is barred from tier1 by the Test Tier Purity Invariant and would stop the parser being a leaf, while the file-existence half survives as the `path-missing` check at validation time.
  Document the known limitation in that same section: an unexported symbol reference misclassifies as a path and surfaces as a loud `path-missing` finding, and the author resolves it by writing the exported name or a `//`-escaped path.
  Keep the `## Card path resolution: root: and //` section, adding that normalization applies to path-shaped entries only and that a symbol entry passes through verbatim regardless of `root:`.
  Replace the `## Moves and the Rename mechanic` section with a `Rename` and `Move` section: a `Rename` card's bullets are `` `old` -> `new` `` pairs on the ASCII arrow grammar and both endpoints project into the card's target list in pair order;
  a `Move` card states its destination in `Intent` prose rather than in its target list;
  the plan-level `## Rename mechanic` section is required when any card is type `Rename`, and its canonical text must be reproduced verbatim in the same rewritten form the golden fixture carries, with its third step naming a separate `Create` card rather than the removed `Creates:` field.
  Document the accepted false positive this creates: because a `Move` destination lives in prose there is no third union to build, so a later card naming a file an earlier `Move` relocated into place produces a false-positive `path-missing` finding, which the author resolves by reordering or by naming the pre-move path.
  Keep `## Numbering and commit subject` unchanged.
  Rewrite `## verify model` to record the three-tier model as designed-not-implemented, exactly as the design doc has it, and to state that the per-card `**Verify:**` field stays the optional, verbatim, rare escape hatch it is today while tier1's automatic package-scoped run is specified only.
  Rewrite `## Validation checks` as a numbered list carrying exactly sixteen rows, one row per distinct `Check:` ID, in the fixed order `Validate` runs them, with `format-unrecognized` and `plan-unapproved` unbundled into their own rows.
  Add a sentence stating that the figure counts distinct IDs rather than presentation rows, and that this resolves the row-count-versus-ID-count divergence the repo's former "14" carried.
  State that `Custom` is exempt from `path-missing` on its own targets and from the `Prosa` target-shape rule, and from nothing else — it remains bound by every card-generic check.
  Replace the `## Worked example` section with the seven-card format-4 worked example, byte-consistent with the golden fixture the tests parse, and update the closing paragraph that explains which entries the plan's `root:` resolves and which the `//` escape keeps worktree-root-relative.
  In `## Deferred / forward-compat`, replace the `changes-files` union definition with the deviation union's new definition — every path-shaped target entry plus the files holding every symbol-shaped target, with `Uses:` excluded because it is read rather than written.
  Leave every relative link in the file resolvable, per the Markdown Link Integrity invariant, and follow the repo's semantic-line-break rule throughout.
- **Commit:** `docs(spec): rewrite loom-plan-spec.md for the format-4 card model`

### Card 18: rewrite the Plan-Write stencil

- **Context:**
  - `contracts/specs/loom-plan-spec.md`
  - `contracts/stencils/stencils.go`
  - `manifest/designs/plan-card-format.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `contracts/stencils/loom/loom-template-plan.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite this stencil's mechanical field-name, grammar, and validator content for format 4, and leave its prompt architecture alone — the `Plan-Write` producer's prompt redesign belongs to the Wave 3 roadmap item, not to this task.
  Keep the leading HTML comment, the four `{{.X}}` markers and their exact spellings, the four numbered Steps and their headings, and the closing `## Never use AskUserQuestion` section unchanged.
  Keep `### What a card is` as it stands — its three criteria are format-independent.
  In `### 00-overview.md`, change the frontmatter block to `format: 4`, keep `approved: false` and the `root:` key with its existing explanation, and rewrite the body-sections sentence so `## Rename mechanic` is required when any card is type `Rename` rather than when any card has a non-empty `Moves:`.
  Rewrite `### Each NN-<card-slug>.md` for the format-4 card: the heading, exactly one bold type label from the seven-name set with its own backtick-wrapped bullets as the card's targets, an optional `**Uses:**`, a required `**Intent:**`, an `**ImpactSummary:**` on `Edit` and `Delete` cards taking its value inline on the label line, and the optional `**Commit:**` and `**Verify:**`.
  State that a field with no content is omitted and that no `none` sentinel is written.
  State that `Uses:` names what the card reads but does not change, and that an entry appearing in both a card's target list and its own `Uses:` is a `card-field-overlap` contradiction.
  Delete the `Depends-on:` paragraph outright — the field no longer exists and edges are derived rather than authored.
  Keep the runnable-verify paragraph as it stands.
  Rewrite the `### ## Rename mechanic` section: its prose must state the `Rename` pair grammar and that a genuinely new file with no predecessor belongs in a separate `Create` card, and its fenced canonical block must match the spec's rewritten canonical text verbatim.
  Rewrite the `### Minimal skeleton` fenced examples to format 4.
  Apply the Producer Pointer-Rule Invariant while rewriting: this stencil legitimately carries the LLM-facing subset — what the producer must write — but any validator rule it currently spells out that the spec also spells out is a pointer candidate rather than something to re-paraphrase in the new vocabulary.
  The rewrite must reduce duplication rather than translate it, and it must not widen the subset beyond what the producer needs in order to write a plan.
- **Commit:** `docs(stencils): rewrite the Plan-Write stencil for the format-4 card model`

### Card 19: rewrite the webster implementer stencil

- **Context:**
  - `contracts/specs/loom-plan-spec.md`
  - `contracts/stencils/stencils.go`
  - `internal/websterengine/render.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `contracts/stencils/webster/webster-body-implementer.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite this consumer stencil's two references to the format-3 field set and reduce its duplication, changing nothing else about the implementer's job.
  In the FRESH-READ rule, replace the phrase naming a card's `Context:` and file-op fields with a phrase naming the card's own target list and its `Uses:` list.
  In step 2 of the per-card procedure, replace the enumeration of `Context:`, `Edits:`, `Creates:`, `Deletes:`, and `Moves:` with a single sentence saying a card names its targets under its own type label and what it reads under `Uses:`, plus a pointer at the spec for the grammar — this stencil is a consumer prompt and must restate less than the producer's, per the Producer Pointer-Rule Invariant.
  In step 1, keep the fallback to the Card Index one-liner but name the `**Intent:**` field rather than `**What:**`.
  In the final batch-report section, rewrite the `deviations` definition to the new deviation union: every path-shaped target entry across the batch's cards, plus the files holding every symbol-shaped target, which the implementer resolves itself since it is already in the worktree and resolving a package-qualified symbol to its file is one read.
  State explicitly that `Uses:` stays out of the union because it is read rather than written, and keep the existing rule that `deviations` is always informational and never makes `status` `FAILED` on its own.
  Keep the `{{.prev_digest}}`, `{{.card_pointers}}`, `{{.worktree_root}}`, `{{.self_fix_cap}}`, and `{{.report_path}}` markers and their exact spellings unchanged, and keep the report's three-field YAML block unchanged in shape.
- **Commit:** `docs(stencils): point the webster implementer stencil at the format-4 card model`

### Card 20: retarget the stencil-pinning tests

- **Context:**
  - `contracts/stencils/loom/loom-template-plan.md`
  - `internal/loomengine/plan.go`
- **Edits:**
  - `internal/loomengine/plan_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget the seven `TestPlanSpec_PromptStates*` tests to the rewritten stencil, keeping the file's `renderedPlanPrompt` helper and its substring-assertion style.
  Delete `TestPlanSpec_PromptStatesMoveRedundantRule`, `TestPlanSpec_PromptStatesMovedFileNotInEdits`, and `TestPlanSpec_PromptStatesDependsOnCriterion` outright — all three pin rules format 4 removes, and adapting a deleted rule's test onto a new rule produces a test that documents the wrong thing.
  Keep `TestPlanSpec_PromptStatesCardCriteria`, `TestPlanSpec_PromptStatesRootResolution`, and `TestPlanSpec_PromptStatesVerifyIsRunnable`, adjusting their expected substrings only where the rewritten stencil's wording moved.
  Retarget `TestPlanSpec_PromptStatesContextSemantics` to the `Uses:` semantics: it must assert the stencil states that `Uses:` names what a card reads but does not change, and that an entry in both a card's target list and its own `Uses:` is a contradiction.
  Add two tests: one asserting the stencil states the type-label grammar — that a card carries exactly one bold type label from the seven-name set and that the label's own bullets are the card's targets — and one asserting the stencil states the `ImpactSummary` requirement on `Edit` and `Delete` cards.
  Every expected substring must be a byte-exact excerpt of the stencil this batch's card 18 wrote, so a later stencil reword fails loudly here rather than drifting silently.
- **Commit:** `test(loomengine): retarget the stencil-pinning tests to the format-4 prompt`

### Card 21: rewrite the sandbox webster suite

- **Context:**
  - `contracts/specs/loom-plan-spec.md`
  - `internal/planparser/validate.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the paragraph describing the plan artifact the sandbox agent must construct per scenario.
  It currently instructs the agent to write frontmatter `format: 3` plus per-card `**What:**`, the five typed file-op fields with `none` when empty, and `**Depends-on:**`.
  Replace that with format 4: frontmatter `format: 4` with `approved: true` and a `## Card Index`, plus one card file per card carrying exactly one bold type label from the seven-name set with its own backtick-wrapped target bullets, an optional `**Uses:**`, a required `**Intent:**`, and an `**ImpactSummary:**` on `Edit` and `Delete` cards.
  State that a field with no content is omitted rather than carrying a `none` sentinel.
  Keep the paragraph's existing closing instruction that the agent reasons the shape out from the validator's own error messages, which name every violated check.
  This file is an agent-facing instruction file, not a Go fixture: the whole-tree `go build` and `go test` gate structurally cannot check it, which is why it is its own card rather than a trailing edit on another one.
  Leave the rest of the suite — its scenarios, goals, and pass criteria — untouched.
- **Commit:** `docs(sandbox): describe the format-4 plan artifact in the webster suite`

### Card 22: correct the websterengine doc comments

- **Context:**
  - `internal/planparser/plan.go`
  - `contracts/specs/loom-plan-spec.md`
  - `manifest/roadmap.md`
- **Edits:**
  - `internal/websterengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Correct two now-false passages, changing no code and no behavior.
  In the `# Declared order now, a dead DAG seam for later` section, the sentence saying the sketched `HasSymbolFields()` call is unreachable in v0 because plan-format cards carry no symbol fields yet becomes false the moment card targets can be symbols.
  Rewrite it so the reason the seam is inactive is that the Wave 3 roadmap item activates it, not that cards carry no symbol fields.
  The seam itself stays dead: do not add a scheduler, a graph, or a topological sort to this package.
  In the `# Fork-return contract` section, rewrite the deviation-union sentence to the new definition — every path-shaped target entry plus the files holding every symbol-shaped target, with `Uses:` excluded — matching the definition card 19 wrote into the implementer stencil.
  Keep the surrounding statement that the deviation list is always informational and never a failure condition.
  This is a comment-only card: no exported identifier, no function body, and no test changes.
- **Commit:** `docs(websterengine): correct the dead-seam and deviation-union comments`

### Card 23: correct the webstercli check-count figure

- **Context:**
  - `contracts/specs/loom-plan-spec.md`
  - `internal/planparser/validate.go`
- **Edits:**
  - `internal/webstercli/validate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `validate` subcommand's `Long` help text, change "the 14 checks" to "the 16 checks" and rewrite the clause enumerating what those checks cover, which currently names the `Moves:` grammar/redundancy/mechanic checks and the depends-on-order gate.
  Replace that enumeration with the format-4 grouping: format and approval, Card Index to card-file consistency, card type presence and retired-label detection, card path well-formedness, the `Rename` pair grammar and its plan-level mechanic section, the per-card structural and field-presence checks, the card-numbering heading cross-check, and the existence-dependent path and commit-subject checks.
  Keep the surrounding sentences unchanged, including the clean-plan envelope shape and the statement that this is the same gate `lyx webster run` runs before forking an implementer.
  Because `Short` and the command tree are untouched, the help-tree pinning tests must keep passing;
  a `Long`-text-only edit is deliberately the whole card.
- **Commit:** `docs(webstercli): update the validate help text to the sixteen-check set`

### Card 24: close out the design docs

- **Context:**
  - `contracts/specs/loom-plan-spec.md`
  - `internal/planparser/validate.go`
  - `manifest/roadmap.md`
- **Edits:**
  - `manifest/designs/plan-card-format.md`
  - `manifest/designs/scout-plan-symbol-fields.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the design doc's status banner clause by clause rather than only its opening phrase.
  Its "designed, not implemented" status becomes implemented, naming this task.
  Its clause saying it supersedes the spec's Card fields and the Plan-Write stencil, neither of which is rewritten yet, becomes a plain pointer at the spec — both are rewritten by this task.
  Its clause instructing the reader to reconcile or delete the two stale predecessor design docs is repointed at the roadmap items that own them: the group-level reconcile instruction and the Someday item that owns `webster-parallel-execution.md`, and the standalone roadmap item that owns `scout-plan-symbol-fields.md`.
  Do not assign either doc to the Wave 3 DAG item, which names neither of them;
  leaving the clause as an unactioned instruction would make the doc read as instructing work this task deliberately declines.
  Keep the clause about the discussion stencil's own scoped supersession claim unchanged.
  Rewrite the `## Open, not decided here` section to record that this task closes all three of its items: `Custom` needs no type-specific mechanical check, which is a principled closure in the affirmative rather than an oversight;
  `ImpactSummary` on a `Delete` card stays one line of prose, identical in shape to an `Edit` card's, because a structured shape would need a caller enumeration the parser cannot produce without the symbol lookup this task excludes;
  and the reconciliation with the spec's validator checks is the disposition table this task implemented, landing on sixteen distinct check IDs.
  Correct that section's own stale "14 validator checks" figure to 16 as part of the same rewrite.
  In `manifest/designs/scout-plan-symbol-fields.md`, correct its stale "14 checks" figure to 16 and change nothing else.
  That document as a whole is stale and its substantive reconciliation stays out of this task's scope — correcting a number is not reconciling a document, and skipping one site out of a mechanical sweep would be arbitrary.
  Follow the repo's semantic-line-break rule in both files and leave every relative link resolvable.
- **Commit:** `docs(designs): close out the plan-card-format doc and fix the stale check counts`

### Card 25: move the roadmap item to Done

- **Context:**
  - `contracts/specs/loom-plan-spec.md`
  - `manifest/designs/plan-card-format.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Move the Wave 2 item `planparser: Card-format migration to Edits/Uses` out of the Wave 2 list and into the Done section, following the shape every existing Done entry uses — a bolded item name, a summary of what actually shipped, and a closing pointer at the package documentation and the pinned spec.
  The summary must record what the task in fact did rather than what the Wave 2 entry predicted, and must correct the entry's own framing: the entry said to update every direct consumer in `internal/websterengine` for rendering, report parsing, and deviation-checking, but no non-test code there read a card file-op field at all — the only non-test consumer change was one field read in the Card Index renderer.
  Record that the check set moved to sixteen distinct IDs, that the format version is now 4 with no dual-reader for format 3, and that webster's execution order is unchanged.
  Renumber the remaining Wave 2 list so it stays consistent, and leave the Wave 3 items' stated dependency on this item intact by rewording their dependency phrase to name the now-Done item.
  Change nothing else in the file: this task moves exactly one planned item, and a roadmap entry moves only on completing or adding a planned item.
- **Commit:** `docs(roadmap): move the planparser card-format migration to Done`

### Card 26: re-run the migration sweeps

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Re-run all three sweeps from the discussion's Technical context and confirm each is clean, rather than trusting any hand list.
  Sweep 1 greps recursively for the literal `**What:**` label or the string `format: 3` across the `internal/`, `tools/`, `contracts/`, and `cmd/` trees;
  every remaining hit must be either an intentional historical mention inside a Done roadmap entry or a deliberate format-3 test case pinning that `format-unrecognized` fires, and any other hit is a missed carrier that must be fixed before this card is committed.
  Sweep 2 greps the `internal/` and `cmd/` trees for the Go-level identifiers `MovePair`, `ContextFiles`, `EditsFiles`, `CreatesFiles`, `DeletesFiles`, and `HasWhat`;
  only `MovePair` may still appear, because it survives as the `Rename` pair type, and the unrelated board-layer `DependsOn` field in `internal/boardengine` is a known false positive of that sweep's pattern rather than a carrier.
  Sweep 3 greps every Go and markdown file for the phrases "14 checks", "fourteen checks", "14 validation checks", and "14 validator checks", excluding the repository's own git directory and the `_mill/` task-state tree;
  it must return zero hits, since every site becomes 16.
  Run `go build ./...` and `go test ./...` from the repository root as the closing confirmation that the tree is green.
  This is a verification-only card: it changes no file, and its whole job is proving the mechanical sweeps are clean before the task hands off.
  If a sweep surfaces a missed carrier, fix it under the card whose scope it belongs to rather than here.
- **Commit:** none

## Batch Tests

`verify:` covers the two packages this batch's Go edits touch plus the package whose spec it rewrites.
`internal/loomengine` is where card 20's retargeted stencil-pinning tests live, and they are the mechanical proof that card 18's stencil rewrite actually says what it claims — every expected substring is a byte-exact excerpt of the rewritten stencil, so a drift between the two fails here.
`internal/webstercli` covers card 23's help-text edit against the repository's help-tree pinning tests, which assert the command tree and its `Short` strings.
`internal/planparser` is included because card 17's rewritten worked example must stay byte-consistent with the golden fixture batch 2 built, and the fixture's own parse and zero-findings tests are what enforce that.

Three of this batch's ten cards touch files no Go test covers at all — `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`, `contracts/stencils/webster/webster-body-implementer.md`, and the two design docs.
Card 26 is their verification: it re-runs all three of the discussion's sweeps and the whole-tree build and test gate, which is the only mechanical check those markdown carriers ever get.

`pipeline.done_gate` runs `go test ./...` plus the integration-tagged suite from the repository root before the task is marked done, catching anything outside every batch's own scoped verify.
