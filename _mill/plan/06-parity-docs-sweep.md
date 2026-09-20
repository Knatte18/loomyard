# Batch: parity-docs-sweep

```yaml
task: 'Producer gates: mechanical gates before session release'
batch: 'parity-docs-sweep'
number: 6
cards: 8
verify: go test ./contracts/stencils/... ./internal/lyxcwd/... ./internal/loomcli/... ./internal/loomrecipe/... && go test -tags smoke -run '^$' ./internal/loomcli/... && go test -tags integration -run '^$' ./internal/websterengine/...
depends-on: [5]
```

## Batch Scope

This batch lands the one cross-cutting invariant the task rewrites and sweeps every remaining mention of the three removed rows out of the repository.
The sweep is a **method, not a hand list**: a repo-wide grep for the five strings hits fifty-three files, and any enumeration written into a plan would be stale before it was implemented, so each card names a class of hit and the final card verifies the whole set.
Card 36 in the preceding batch has already confirmed that every surviving hit at this point is prose, so nothing in this batch is a live identifier.

Every such mention is part of this task rather than a follow-up: a stale pointer to a row that no longer exists is worse than no pointer, and three of the hits are deployed normative stencils that actively mislead a judge or a fixer about where its checks are enforced.

Batch-local decision: this batch promotes nothing into the roadmap's Planned section.
Moving the shipped item to Done empties Planned, and choosing what fills it is the operator's call, not a consequence of this task.

## Cards

### Card 37: Rewrite the Gate Self-Check Parity Invariant

- **Context:**
  - `internal/loomshed/gates.go`
  - `internal/loomcli/parity_test.go`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the invariant around the new shape: a mechanical gate's **closure** and its CLI self-check verb call the same package function for every mode.
  Its pair list becomes two entries rather than three — the two discussion rows paired with `validate-discussion` on `discussionparser.Validate`, and the two plan rows paired with `validate-plan` on `planglyph.ValidateFormat`.
  Drop the retired third pair and record in its place that the verb's `--require-approved` mode, running the full check set, has no recipe counterpart by design: both plan gate sites run strictly before the Plan-Review segment's approve seam writes the flag, so demanding it would fail every fix round, and the flag's guarantee rests on that seam failing loudly instead.
  Keep the invariant's closing rule verbatim — adding a mechanical gate means adding its verb and its parity check in the same task — and state that moving the gate from a row into a producer changed *where* the call sits, never the property, which is why the invariant survives rather than retiring with the rows.
  Make no other edit to this file: the gate adds no new cross-cutting invariant of its own, and every other invariant it binds against (Completion Signal, Told-Geometry, Shuttle Provider-Seam, Config Strictness, the two sole-parser invariants, Shed Recipe Registry, Test Tier Purity, Quarry CGO) is satisfied unchanged rather than amended.
- **Commit:** `docs(constraints): rewrite the Gate Self-Check Parity Invariant around the gate closures`

### Card 38: Mark the design doc shipped and move the roadmap item

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/shuttleengine/wait.go`
  - `internal/loomshed/gates.go`
  - `internal/shuttleengine/claudeengine/settings.go`
- **Edits:**
  - `manifest/designs/producer-gates.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change the design doc's status line from a settled direction ready to be broken into a task to shipped, and record against the doc's own text the three places the implementation deliberately narrowed or diverged from it, each with its reason, so a later reader is not left thinking the doc describes what shipped.
  First, the per-attempt done-signal: the compound quiescence definition the doc sketches — turn-idle together with a process-tree probe and a hook-maintained pending-work ledger — is not built, because the in-process `Agent` tool is denied at every gated site and none of them authorizes fork subagents, so the async-subagent hazard that definition guards cannot arise there; name the two tests that pin both halves of that precondition.
  Second, findings delivery: always a file with a one-line pointer, never the doc's inline-or-file split, because the send path rejects any text containing a newline outright and findings counts are unbounded, which makes the inline branch a latent delivery failure rather than an optimization.
  Third, the gate signature: `func() (GateResult, error)` closing over its own told paths rather than the doc's `func(artifactDir string)`, because the two validators take materially different path shapes and no single directory argument fits either.
  In the roadmap, move the Planned item to the top of the Done section, keeping its wording and adding a pointer to the loom module doc, and leave the Planned section empty with its own intro paragraph intact — promoting a successor is the operator's call, not this task's.
- **Commit:** `docs(manifest): mark producer gates shipped and move the roadmap item to Done`

### Card 39: Correct the deployed specs and the top-level docs

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomshed/gates.go`
  - `internal/loomcli/validate.go`
- **Edits:**
  - `docs/overview.md`
  - `README.md`
  - `contracts/specs/loom-plan-spec.md`
  - `contracts/specs/specs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the overview, correct the two self-check verb descriptions, which name the removed rows as the gates whose checks each verb runs standalone: they now run the same checks the writer and fixer rows' own gates run.
  No module table row and no execution-stack entry changes, since the task adds and removes no module; say nothing in this file about the recipe's row count, which it does not carry.
  In the README, correct the pipeline diagram's two lines, which walk through both removed validate rows and the removed revalidate row, so the walk reads writer straight into its review segment and the plan segment straight into the batchifier.
  In the plan spec, correct the three sentences attributing checks to the removed rows: the validation-checks pointer in the header, the rename-resolution paragraph naming both plan rows, and the consumer-guard paragraph that contrasts the pre-review gate against the post-segment row — that contrast no longer exists, and the paragraph must instead say that both plan gate sites run the format-only set before approval, that the standalone consumers still enforce the flag, and that no row re-checks it.
  In the specs package doc, correct the sentence naming the two plan rows as what parses a written plan against the shipped grammar.
- **Commit:** `docs: correct the deployed specs and top-level docs for the gated rows`

### Card 40: Correct the manifest design docs

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/shuttleengine/gate.go`
  - `internal/shedrecipe/entries_gate.go`
  - `manifest/designs/producer-gates.md`
- **Edits:**
  - `manifest/designs/loom.md`
  - `manifest/designs/shed.md`
  - `manifest/designs/shed-recipe.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The loom module doc carries the heaviest share of the sweep — its producer table, its recipe walkthrough, and its gate prose all name the three removed rows.
  Correct the table and the walkthrough to the fourteen-row shape, and rewrite the gate prose around the new mechanism rather than merely deleting the rows: which four rows are gated, which validator each gets and through which config key, that an exhausted gate on a writer row halts the run for a human while an exhausted gate on a fix round hands back to its segment's Bouncer with an empty pointer, and that the budget is spent in memory inside one wait and resets across an interrupted-and-reinvoked step by design.
  Add to the same section the one thing no removed row ever covered and the whole reason the task exists: a fix round's own output is now checked, so a round can no longer hand back an artifact it just made invalid.
  In the shed design doc, correct the single sentence using the removed plan row as its worked example of a per-producer `OnStuck` config value, re-pointing it at a surviving pair.
  In the shed-recipe design doc, correct the sentence listing the loom-specific engine names as examples of bespoke single-consumer registry entries, dropping the two retired engines and keeping the point, which is unaffected.
  Every markdown link this card touches must still resolve, file part and anchor alike, per the Markdown Link Integrity invariant.
- **Commit:** `docs(manifest): correct the loom, shed and shed-recipe design docs for the gated rows`

### Card 41: Correct the three deployed rubric stencils

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `contracts/specs/loom-plan-spec.md`
  - `internal/stencilstore/stencilstore.go`
- **Edits:**
  - `contracts/stencils/loom/loom-rubric-discussion-review.md`
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
  - `contracts/stencils/rubric_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** These three are the most actively misleading hits in the sweep: they tell judges and fixers that the mechanical checks are already enforced upstream by rows that will not exist, when those checks now run on that very round's own output.
  In the discussion rubric, correct the sentence attributing the section contract to the removed row; in the plan rubric, correct all four hits, including the twenty-seven-upstream-and-one-downstream split, which collapses now that both halves no longer exist as rows — the format-only set runs at the round's own gate and the approval check runs at no row at all; in the webster rubric, correct the out-of-scope bullet naming both removed plan rows.
  Each rewrite says the checks are enforced by the round's own gate over the round's own output, which is a stronger claim than the one it replaces and is why the out-of-scope bullets stay out of scope.
  Update the rubric test's two pinned substrings to match, keeping the assertions' shape.
  Say nothing about stencil distribution in the stencils themselves: an already-seeded worktree picks the corrected text up through the store's own four-state reconcile — an untouched copy is rewritten on the next prod-mode run with no action, a dev build runs the documented force-refresh verb once, and a hand-edited copy keeps its edit and its warning — and that behaviour needs no carve-out to the Stencil Ownership Invariant and no new prose here.
- **Commit:** `docs(stencils): tell each review round its mechanical checks run at its own gate`

### Card 42: Correct the Go doc-comment prose across the tree

- **Context:**
  - `internal/loomshed/gates.go`
  - `internal/shuttleengine/gate.go`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:**
  - `internal/shuttleengine/attach.go`
  - `internal/loomshed/discussionwrite.go`
  - `internal/loomcli/start.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/cli.go`
  - `internal/friction/friction.go`
  - `internal/planglyph/planglyph.go`
  - `internal/planparser/validate.go`
  - `internal/discussionparser/validate.go`
  - `internal/loomcli/validate_test.go`
  - `internal/websterengine/runlevel_test.go`
  - `internal/shedengine/run_routing_test.go`
  - `internal/loomcli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Every hit in these files is a doc comment or a test comment naming a removed row as the thing that performs some behaviour; none is an identifier, and none changes any code.
  Re-point each at what actually performs that behaviour now: the discussion parser's file comment and its bounce-behaviour paragraph name the discussion gate rather than the removed row; the glyph package's whole-plan comment and the plan parser's done-check comment name the plan gate; the friction package's re-entry paragraph is rewritten for the new graph, in which a writer row is re-entered by a resume rather than by a validate row's bounce; the attach file's crash-versus-bounce comment, which currently reasons via a bounce that no longer exists, is rewritten for the resume case it actually guards; the discussion write decorator's comment naming the row that used to judge its output afterwards now names its own gate, which judges it before the handoff; the CLI's package comment, the loom start comment, and the wiring comment each name their own gate.
  The four test comments are corrected the same way.
  Where a comment's *reasoning* rested on the removed row's existence rather than merely naming it — the friction re-entry paragraph and the attach crash-versus-bounce paragraph are both of that kind — rewrite the reasoning rather than substituting a name into a sentence that no longer holds.
- **Commit:** `docs: re-point Go prose at the gates that replaced the three validate rows`

### Card 43: Correct the self-check verbs' own help text

- **Context:**
  - `internal/loomshed/gates.go`
  - `internal/loomcli/parity_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomcli/validate.go`
  - `internal/loomcli/status_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The two verbs' `Short` and `Long` strings are operator-facing text naming the removed rows as the gates whose checks each verb runs standalone, and the `--require-approved` flag's usage string names the removed row it used to match.
  Re-point each at the gates that now run those checks: the discussion verb runs the checks the two discussion rows' gate runs, the plan verb the format-only set the two plan rows' gate runs, and the flag adds the approval check no row runs at all — which is the one place an operator can still reach it, and the usage string should say so.
  Correct the file's own package comment, which names both removed rows as the gates the verbs are zero-argument callers of.
  Every command keeps a non-empty `Short`, per the CLI/Cobra Invariant, and no flag, exit code, or envelope shape changes — only the wording.
  In the status test, the removed row's name is used as an arbitrary activity-string fixture in the one-line formatter's table; substitute a surviving row name so the fixture does not read as a pointer to a row that no longer exists.
- **Commit:** `docs(loomcli): re-point the self-check verbs' help text at the gates`

### Card 44: Confirm the sweep is complete

- **Context:**
  - `CONSTRAINTS.md`
  - `manifest/roadmap.md`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Verification only, no edit.
  Re-run the repo-wide grep for the five strings naming the removed rows and their engines over the whole worktree, excluding the git directory and the task's own mill directory, and confirm the result is empty.
  A non-empty result at this point is a file the sweep missed rather than an acceptable survivor: the preceding cards cover every class of hit, and the design doc is the one file permitted to discuss the removal historically — if it names a removed row, confirm the mention reads as history rather than as a live pointer, and rewrite it if it does not.
  Confirm in the same pass that the recipe parses to exactly fourteen rows and that the four gated rows each carry both gate keys, by reading the recipe rather than by running a test the batch's own verify already runs.
- **Commit:** none

## Batch Tests

`verify: go test ./contracts/stencils/... ./internal/lyxcwd/... ./internal/loomcli/... ./internal/loomrecipe/...` covers the four packages with assertions over the text this batch edits.
The stencils package pins the rubric substrings card 41 rewrites and the citation contract those stencils carry; `internal/lyxcwd` owns the Markdown Link Integrity scan over every `manifest/` and `docs/` file cards 38, 39 and 40 touch, including the roadmap's own links; `internal/loomcli` owns the help-tree assertions over the two verbs' `Short` strings and the status formatter's table, both edited by card 43, plus the parity test batch 5 rewrote; `internal/loomrecipe` re-reads the embedded recipe, which card 40's prose is derived from and which card 44 re-reads by hand.
Two of card 42's edits are comments inside build-tagged test files that the untagged run cannot even compile — the smoke-tagged bootstrap test in `internal/loomcli` and the integration-tagged run-level test in `internal/websterengine` — so the verify chains one compile gate per tag behind the untagged run.
Each is a real `go test` invocation carrying the tag with a `-run` pattern that matches nothing, so the tagged files are type-checked without a single tagged test executing: the smoke suite needs a real agent and a real pane, and the integration suite needs git, neither of which belongs in a batch gate.
The remaining edits in cards 42 and 39 are Go doc comments and markdown with no assertion over them, which is why card 44 is a grep gate rather than a test: the sweep's completeness has no static shape a test in this repo can see, and an empty grep is the check.
