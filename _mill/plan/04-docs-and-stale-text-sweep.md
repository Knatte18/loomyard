# Batch: docs-and-stale-text-sweep

```yaml
task: 'loom: Plan-Review producer'
batch: 'docs-and-stale-text-sweep'
number: 4
cards: 6
verify: go build ./... && go test ./internal/loomengine/... ./internal/loomcli/... && go vet -tags smoke ./internal/loomcli/...
depends-on: [3]
```

## Batch Scope

This batch lands every doc and doc-comment edit the shipped change invalidates, in the same task the change lands in, per the Documentation Lifecycle.
It is one batch because it is one sweep over one scan result: the file set was produced by a repo-wide search over four patterns — the row name `Plan-Review`, the Go symbol `NamePlanReview`, commit-seam claims, and the fourteen-row count claim — and every hit is classified against which of those it is before it is touched.

Three of the four counts named "fourteen" in this repo stay fourteen and must not be edited: `internal/shedrecipe`'s engine registry, `manifest/designs/loom.md`'s own producer table, and `landingshed.Deps`' field count.
Only loom's **recipe row list** becomes fifteen.

The batch depends on batch 3 because it documents what batch 3 shipped;
nothing here changes production behaviour, and every Go edit in it is a doc comment.

Batch-local decision: "Out of scope" in this task's discussion means production **behaviour**, never doc comments.
`internal/loomengine/config.go` and `internal/loomcli/wiring.go` are behaviourally out of scope and still have doc comments this change falsifies, and cards 15 and 16 fix exactly those comments and nothing else.

## Cards

### Card 11: Bring loom's own design doc in step with the shipped segment

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
  - `manifest/designs/plan-card-format.md`
  - `contracts/specs/loom-plan-spec.md`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Five separate edits in this one file.

  **1. The two count sentences under "What it is".**
  The sentence stating the recipe "names the recipe's fourteen rows and their routing" becomes fifteen.
  The sentence immediately after it — asserting the recipe's rows and the table's entries "are the same count, but not the same set" — is now false in both halves and must be rewritten: the recipe carries fifteen rows, the table fourteen entries, and the note beneath the table explains why.

  **2. Producer-table row 9.**
  Rewrite the row so its `Producer` cell names the segment and its two rows the way row 5 already does for Discussion, and so its `Input` cell names the current plan directory as the subject and the decision record as the answer key rather than pointing at the format spec.
  Keep the row's `Kind` as `bespoke` and its `Type` as `LLM/review segment`, and keep the table at fourteen entries — row 9 stays one entry, collapsing its two recipe rows exactly as row 5 does.

  **3. The "table and the shipped recipe diverge deliberately" paragraph.**
  Its "Both count fourteen, but not the same fourteen" opening is now wrong.
  Rewrite it to say the recipe carries fifteen rows against the table's fourteen entries, and to name **both** collapsed pairs — the Discussion pair it already names, and the Plan pair this task shipped — alongside the `Plan-Sweep` row the table carries and the recipe does not.
  Keep the closing claim that the table is the human-readable design record, kept at fourteen entries by design and not required to track the recipe's row count row-for-row.

  **4. The stuck-routing example sentence.**
  The parenthetical example naming a stuck route back to `Plan-Write` is stale — in the perch pattern the Bouncer's stuck target is its own Burler, and exhausting the bounce budget escalates to a human rather than re-routing to the writer.
  Replace it with an example that is still true of the shipped recipe;
  the validator row immediately upstream of the segment is the natural choice, since it genuinely does bounce back to the writer.
  Fix the same stale example wherever else it appears in this file.

  **5. The `### Plan-Review rubric` section.**
  Rewrite it as a doc *about* the now-shipped stencil, following the two `### Discussion-Review rubric` subsections as the exact precedent: open by naming the shipped stencil file and saying that this subsection is the durable human-readable record it was transcribed from, per the Producer Pointer-Rule Invariant, not a second copy the stencil must point at.
  Delete the "no rubric exists yet for the Card format" sentence — one exists now.
  Record all four "Also flag" items, all three "Do not flag" items, and the named support-log exclusion, matching the shipped stencil's own wording closely enough that a drift between the two is visible on a side-by-side read.

  Also update the sentence listing the deliberately-last per-producer prompt/rubric tasks so it no longer names this task as pending.
  Leave the `## The gate` section alone: its "hand-wired once per phase (discussion / plan / webster)" framing is what this task makes true, not what it makes stale.

  Follow the repo's semantic-line-break rule in every line touched, and keep every relative link in the file resolvable.
- **Commit:** `docs(loom): record the shipped Plan-Review segment and its rubric in the design doc`

### Card 12: Fix the stale Plan-Review routing examples outside loom's own doc

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `contracts/specs/loom-plan-spec.md`
  - `internal/shedengine/activity_test.go`
  - `docs/overview.md`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/designs/shed.md`
  - `manifest/designs/review-finding-classification.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `manifest/designs/shed.md`, two sentences use a stuck route from `Plan-Review` back to `Plan-Write` as their worked example of `OnStuck`: one in the bounce-budget section illustrating an unbounded A-to-B-and-back cycle, and one in the section explaining that `OnStuck` makes bounce routing a per-row config value rather than a hardcoded branch.
  Neither route exists in the shipped recipe any more.
  Replace both with a routing pair that does exist, keeping each sentence's own point intact — the cycle example still needs two rows that genuinely bounce to each other, and the config-value example still needs a single concrete route.

  In `manifest/designs/review-finding-classification.md`, the numbered proposal item calling for "Plan-Review's own future rubric" now describes a shipped file.
  Reword it to name the shipped stencil and to state that what remains open is the finding-class dimension the item proposes layering on top of it — the catchment it already describes (batching/sequencing/verify-command correctness gates, prose-level nits do not) is unchanged and stays.

  **Classified and deliberately left unchanged — do not edit these:**
  `contracts/specs/loom-plan-spec.md`'s "It is reviewed by `Plan-Review`" names the segment, which still exists under that name.
  `docs/overview.md`'s reference to loom's Plan-Review segment's Bouncer is likewise a segment reference, and is now more accurate than before.
  `CLAUDE.md`'s perch-terminology paragraph lists `Plan-Review` among the segments, which is correct.
  `internal/shedengine/activity_test.go` uses the string `Plan-Review` as arbitrary fixture data in a package that knows nothing about loom's recipe;
  it is not a claim about loom and must not be churned.
  Confirm each of these five reads as described before leaving it, rather than assuming from this list.
- **Commit:** `docs(shed): repoint the stale Plan-Review routing examples at rows that still exist`

### Card 13: Document commit_seam and the fifteen-row list in the recipe design doc

- **Context:**
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedadapters/bouncer.go`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:**
  - `manifest/designs/shed-recipe.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Two edits.

  First, the two "fourteen-row" claims about loom's built list — one in the opening "The idea" paragraph, one in the "Test ownership" bullet under "Decisions this piece settled" — both become fifteen-row.
  Leave every other count in the file alone.

  Second, document the new `commit_seam` key.
  Its home is the "What's never in a recipe" section, in the "Live seams" paragraph, because that is where the rule it appears to contradict lives.
  Add a short paragraph stating that a `Config` key may **select** among the seams the told `Env` already carries, by name, without carrying one — `commit_seam` on a `Bouncer` row takes one of exactly two literal values, `plan` and `discussion`, resolving to `Env.CommitPlan` and `Env.CommitDiscussion` respectively.
  State the two rules that make it safe: an absent key is a legitimate "no seam configured" and leaves the closure nil, while a **present** key naming a closure the `Env` does not carry is a construction error rather than a silent nil, because a nil closure would silently mean "commit nothing" — the exact condition the key exists to eliminate.
  Note that this is the same shape `rubric_stencil` already has, naming a stencil rather than carrying one, so it extends the existing `Env`-versus-`Config` rule rather than forking it.

  Do not restate `shedadapters.BouncerConfig`'s own field documentation here — this doc records the recipe-facing key, and the Go doc records the field.
- **Commit:** `docs(shed-recipe): document the Bouncer row's commit_seam key and loom's fifteen rows`

### Card 14: Close the Plan-Review roadmap item and open the Discussion-segment follow-up

- **Context:**
  - `manifest/designs/loom.md`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Remove the Planned item **loom: Plan-Review producer** from the "loom: real LLM producers" group — this task completes it.
  Update that group's own intro sentence, which today says all three items below are unblocked, so its count matches what is left.

  Add one new Planned item in the same group, at the end of its list, covering the two shipped-Discussion-segment defects this task recorded and deliberately did not fix.
  The item's title names the `fix-scope` violation, since that is the concrete correction;
  its body must cover both halves, because splitting them would have two tasks editing the same two recipe rows:

  - `Discussion-Burler`'s `fix-scope: source` over the discussion artifacts instructs an agent to git-commit weft content, which the Fabric Git Invariant forbids.
    The correction is now a two-line recipe change, because this task shipped the `commit_seam` key and the `Bouncer` `Commit` closure that make the `overlay` form workable;
    what makes it its own task is that flipping the row changes shipped behaviour and its tests.
  - Both review segments resolve their `_lyx` paths against `Env.WorktreeRoot`, while the matching commit closures anchor at `AnchorPath()`.
    The two are identical while `AnchorRel` is `"."`, its default, so this is latent rather than broken;
    re-pointing the resolution root in the shared `Bouncer` entry would silently change both segments at once, which is why it belongs with the row flip rather than with this task.

  Point the item at the design doc the way the sibling items do.
  Follow the file's own numbering convention — every Planned item is written as `1.` and rendered sequentially — and the Maintenance section's own rules for how the list is edited;
  read that section before writing rather than inferring the convention from the surrounding items.
- **Commit:** `docs(roadmap): close the Plan-Review producer item and file the Discussion-segment follow-up`

### Card 15: Reword the reviews-tree ephemerality justification

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedrecipe/entries_bouncer.go`
- **Edits:**
  - `internal/loomengine/config.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `LoomReviewsDir`'s doc comment justifies the reviews tree being ephemeral with the claim that there is no commit seam for a `Bouncer` row.
  That claim stopped being true the moment `BouncerConfig.Commit` existed.

  The **conclusion survives and must not change**: the reviews tree stays ephemeral, and the directory does not move.
  Reword only the justification, to say that a `Bouncer` row's commit seam — where one is configured at all — commits the artifact under review, never this tree, so the round reports, verdicts, ledgers, focus files, and their archive siblings that land here would still be untracked dirt if they lived under the durable tree instead.

  Change nothing else in this file: no key, no accessor, no `Config` field.
  This is the one hit the commit-seam scan pattern found, and it exists because the file is behaviourally out of this task's scope while its doc comment is not.
- **Commit:** `docs(loomengine): reword LoomReviewsDir's ephemerality justification for the new commit seam`

### Card 16: Widen the segment-specific comments in loomcli

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/shedrecipe/recipe.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/wiring_test.go`
  - `internal/loomcli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  No `Env` field is added by this task and no production behaviour in this package changes — `wire()` already fills every field both segments read, `CommitPlan` included.
  Only comments change, in three places.

  In `internal/loomcli/wiring.go`, the comment introducing `StencilsDir`, `RunRoot`, `Burler`, and `Now` says they are filled for the Discussion segment specifically;
  widen it to both review segments, keeping its existing points about `StencilsDir` being one value read from one place and about `Now` being filled explicitly so a test can inject a clock.
  In the `CommitPlan` comment immediately above it, add a sentence recording its second caller: the `Plan-Bouncer` row's approved settle now reaches this same closure through the row's `commit_seam`, which is why the existing idempotence note — a second `Done` over already-committed artifacts is a no-op, because the underlying commit reports nothing committed for an already-clean tree — now covers two callers rather than one.
  Record in the same place that the commit message is deliberately shared between the two callers: it names the artifact set rather than the producer that last touched it.

  In `internal/loomcli/wiring_test.go`, `TestWire_ReviewSegmentSeamsFilled`'s doc comment names the Discussion pair specifically;
  widen the wording to both segments.
  Verify before editing that neither this test nor `TestWire_ReviewTripleMatchesLoadedConfig` needs an assertion change — they assert `Env` fields both segments share, and this task adds no `Env` field — and leave both bodies untouched if so.

  In `internal/loomcli/smoke_test.go`, the driver-liveness timing note says loom's producer table backs two of its fourteen rows, `Plan-Review` and `Webster-Review`, with stub producers.
  One of its fifteen rows, `Webster-Review`, is backed by a stub now.
  Leave the rest of that note intact: the bounce-through-Discussion-Write/Discussion-Validate lifecycle it describes still happens well before any plan row is reached, so the timing claim is unaffected.
  This file carries a `smoke` build tag, so nothing but `go vet -tags smoke` compiles it — see this batch's own test section.
- **Commit:** `docs(loomcli): widen the review-segment comments to cover both shipped segments`

## Batch Tests

`verify: go build ./... && go test ./internal/loomengine/... ./internal/loomcli/... && go vet -tags smoke ./internal/loomcli/...`

Every card in this batch is documentation, so the gate is a compile-and-regress check rather than new assertions.
`go build ./...` covers the four markdown files, which have no runnable surface at all, by proving nothing else regressed;
the two Go packages carrying edited doc comments get their own test runs.

The third clause exists because `internal/loomcli/smoke_test.go` carries a `//go:build smoke` tag, so neither `go test ./internal/loomcli/...` nor the hub's own repo-wide `done_gate` (`go test ./... && go test -tags integration ./...`) compiles it at all — an edit to that file would otherwise go entirely unchecked until someone ran the smoke suite by hand.
`go vet -tags smoke` is used rather than `go test -tags smoke` deliberately: the suite needs a real tmux server, a real detached driver process, and real advisory-lock interaction, which is not something a batch verify can stand up, while the edit under test is comment-only.
Vet type-checks the tagged file, which is the whole risk surface for a comment change.

`internal/loomengine`'s suite covers card 15's file, and `internal/loomcli`'s untagged suite covers cards 16's other two files, including `wiring_test.go`'s own two review-segment tests — which must still pass unchanged, since no `Env` field is added by this task.
