# Batch: loomrecipe-regression-and-docs

```yaml
task: 'loom: interactive Discussion-Write'
batch: 'loomrecipe-regression-and-docs'
number: 5
cards: 3
verify: go test ./internal/loomrecipe/ ./internal/lyxcwd/
depends-on: [3, 4]
```

## Batch Scope

This batch closes the task: the `loomrecipe` regression pair that pins the doc's trap as solved, and the repo-level documentation rewrite the design's reversal reaches.

It depends on batch 3 (the probe must exist for the bounce test to mean anything) and on batch 4 (the mode selector must exist for `docs/overview.md`'s key list to be accurate).
The regression pair is what makes the whole task provable end to end: same on-disk artifacts, opposite verdicts, decided purely on whether a live matching run exists.

Batch-local decision: `fakeLoomShuttle`'s `Attach`, left at the minimal always-not-found shape by batch 3, becomes scriptable here — a found flag plus a returned `shuttleengine.Result` and an `attachCalls` counter — so one test can drive the not-found (bounce) path and the other the found (crash-resume) path against the same fixture.
No real run directory, `run.json`, or reed state is involved at this tier: `Attach`'s own on-disk scan is proven in batch 2's `attach_test.go`, and re-proving it here would breach the `Test Tier Purity Invariant`'s spirit for no extra coverage.

The `manifest/roadmap.md` move is the one roadmap edit this task makes, and it is the sanctioned kind: a Planned item completing.
Per `CLAUDE.md`, the roadmap moves only on completing or adding a planned item.

## Cards

### Card 13: the bounce-respawns / live-run-attaches regression pair

- **Context:**
  - `_mill/discussion.md`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomrecipe/sequence_test.go`
  - `internal/loomrecipe/shape_test.go`
  - `internal/loomshed/loomshed.go`
  - `internal/shedengine/run.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/spec.go`
  - `contracts/recipes/loom-recipe.yaml`
  - `manifest/designs/loom.md`
- **Edits:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/resume_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Make `fakeLoomShuttle`'s `Attach` scriptable in `fixture_test.go`: add fields for the `found` bool, the returned `shuttleengine.Result`, and an optional error, plus an `attachCalls` counter and the recorded `shuttleengine.Spec`.
  Update the fake's doc comment to describe the new branch alongside its existing per-role `Run` behaviour, in the same style, including the note that `attachCalls` lets a test assert the probe ran even when it reported not-found.
  Keep the default zero value at not-found, so every existing `loomrecipe` test keeps driving the unchanged archive-then-spawn path.

  Add two named tests to `resume_test.go`, and name them so a future reader sees they are the point of the task:

  The **bounce** case. Drive `Discussion-Validate` to `shedengine.Stuck` with both discussion artifacts present on disk and the fake's `Attach` reporting not-found, and assert `Discussion-Write` **respawns** — that `fakeLoomShuttle.runCalls` advanced, that the probe ran first (`attachCalls` advanced too), that the row did **not** report `Done` off bare file existence, and that the run does not ping-pong until `Discussion-Validate`'s bounce budget is exhausted.
  Reuse the file's existing `resetCurrentProducer` helper to plant the on-disk snapshot rather than hand-writing status JSON, and the bounce-budget assertions in `TestBounceRouting_BudgetExhaustionBlocks` as the shape to check against.

  The **crash-resume** case. The same artifacts present, but with the fake's `Attach` reporting found with `Outcome: shuttleengine.OutcomeDone`, and assert `Discussion-Write` **attaches**: `runCalls` did not advance, the artifacts on disk were not archived to timestamped siblings, and the row reported `shedengine.Done` with the first output file as its pointer.

  Each test's doc comment must state what it closes: these two cases are the whole `Discussion-Validate`-bounce-versus-interrupted-interview discrimination `manifest/designs/loom.md`'s "interactive-mode trap" paragraph left open, and they differ in exactly one input — whether a live matching run exists — with byte-identical on-disk state otherwise.
- **Commit:** `test(loomrecipe): pin that a bounce respawns and a live run attaches`

### Card 14: rewrite `loom.md`'s crash-recovery discipline and `shed.md`'s restatement

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
  - `internal/shuttleengine/attach.go`
  - `internal/shuttleengine/doc.go`
  - `internal/shedadapters/doc.go`
  - `internal/shedadapters/singlellm.go`
  - `manifest/roadmap.md`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/designs/loom.md`
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  This design changes the *discipline* `manifest/designs/loom.md` states, not merely the one paragraph that flagged the trap.
  Edit each of the following sites, and no others:

  The sentence "**loom resumes on output FILES, not on live processes.**" becomes a two-part rule: loom resumes on output files **and** on live-agent evidence, with file existence never used on its own to skip a step.

  Ladder step 1 ("Is there a complete output file? → the step finished") stays in the ladder but is narrowed: it is checked *inside* an attached or started run's own wait loop, where file existence answers "did *this agent* finish" and there is an agent to attribute it to, never as a producer-level shortcut, where it answers only "do files exist" — which a `Discussion-Validate` bounce makes true without any agent having run.
  Do not delete step 1: it is what lets an attached run notice a completion that landed during the outage.

  Ladder steps 2 and 3, which this task implements for the first time, stop being described in the future tense.
  Name where each now lives: step 2 is `shuttleengine.Runner.Attach` probed by `shedadapters.SingleLLMProducer.Call`, and step 3 is that call's unchanged archive-then-spawn fallback.

  The "**The interactive-mode trap.**" paragraph is replaced by its resolution: the crash-versus-bounce question is answered by whether an agent for this producer is still alive — a surviving `run.json` matching the spec's output files, whose persisted `Outcome` is still `"running"` and whose `StrandGUID` reed still tracks with a live pane — never by whether the output files exist.
  Record the accepted residual from `accepted-residual-the-completed-crash-window` in the same place, stated plainly rather than left for someone to discover: a crash in the window between a run reaching `done` and Shed persisting that outcome re-runs the completed step from scratch, which in interactive mode means the operator answers the whole interview twice.
  Say why that trade is the right direction — the only thing that would rescue it is the producer-level file-existence check that is the trap itself, and a frequent hard failure is worse than a narrow one costing only rework — and name the future fix that is deliberately not designed here: a marker distinguishing "these files were produced by a run that reached done" from "these files are merely present".

  The Graceful-pause cross-reference to "the same resume-on-files discipline as crash recovery" must track the reworded rule.

  **The section heading `### Crash recovery — resume on output files, not live processes` is not renamed**, so the anchor `#crash-recovery--resume-on-output-files-not-live-processes` stays valid.
  `manifest/roadmap.md` and this same file's Graceful-pause bullet both link it, and `CONSTRAINTS.md`'s **Markdown Link Integrity** invariant binds.
  If a future editor judges the heading actively misleading, renaming it is a separate change that must update both inbound links in the same commit — say so in the doc so the constraint is visible to the next reader.

  In `manifest/designs/shed.md`, the sentence restating the resume-on-output-files rule for the generic `ShedProducer` contract — the one about a gate producer or a terminal producer simply re-running on resume "since the resume-on-output-files rule degrades gracefully" — must track the same rewording.
  Shed's own contract is unchanged (it still re-`Call`s `current_producer` unconditionally); what changed is that a producer may now answer that call by attaching rather than by starting fresh, and the doc should say so without implying Shed gained a new concept.

  Every edited line uses semantic line breaks per `CLAUDE.md`: one sentence per line, plain newlines, no fixed-column hard wrap, no trailing double-spaces.
- **Commit:** `docs(loom): rewrite crash recovery as files-plus-live-agent-evidence`

### Card 15: `docs/overview.md`'s key list and `roadmap.md`'s Planned-to-Done move

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/loom.md`
  - `internal/loomengine/template.yaml`
  - `internal/loomengine/config.go`
  - `internal/configcli/configcli.go`
  - `internal/lyxcwd/docslink_test.go`
  - `CONSTRAINTS.md`
  - `CLAUDE.md`
- **Edits:**
  - `docs/overview.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `docs/overview.md`'s loom module entry, both halves of the "loom's config module (`loom.yaml`, holding the `discussion`/`plan` role model-specs and `discussion_timeout_min`/`plan_timeout_min`) exists and reconciles via `lyx config reconcile`" sentence need touching.
  `discussion_interactive` joins the enumerated key list.
  The reconcile mention carries the `--apply` form: `lyx config reconcile` alone is a dry run that reports added and removed keys and writes nothing, so the bare verb reconciles nothing.
  The Discussion-producer sentence immediately below it gains the fact that the producer now has two modes — autonomous by default, interactive when `discussion_interactive` is set — and that both prompt renderings already ship in the same stencil.

  In `manifest/roadmap.md`, move the **loom: interactive Discussion-Write** item out of the `## Planned` section's `### loom: real LLM producers` sub-category and into `## Done`, written as `1.` like every other item — numbering is automatic and restarts per section, so no renumbering is needed anywhere.
  Per the file's own Maintenance rules, a Done entry is a name plus one or two sentences and carries a link to where its durable detail now lives: point at `manifest/designs/loom.md`'s crash-recovery section, which now carries the resolution rather than the open trap, matching what both existing `loom:` Done entries already do, and keep the item's own one-sentence summary short.
  Check whether the `### loom: real LLM producers` sub-category's own preamble sentence ("What 'loom: write and wire in the real LLM producers' split into … both items below are unblocked") still reads correctly with only one item left beneath it, and adjust it if not.
  Do not touch the sibling **loom: `Discussion-Burler`'s `fix-scope: source` violates the Fabric Git Invariant** item.

  Every markdown link in both files must still resolve, file part and `#anchor` alike — `CONSTRAINTS.md`'s **Markdown Link Integrity** invariant is machine-enforced by `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks`, which this batch's `verify:` runs.
  Semantic line breaks per `CLAUDE.md` throughout.
- **Commit:** `docs: record discussion_interactive in overview and move the roadmap item to Done`

## Batch Tests

`verify: go test ./internal/loomrecipe/ ./internal/lyxcwd/` covers both halves of this batch.

`internal/loomrecipe` runs the new regression pair in `resume_test.go` alongside the whole existing sequence, resume, revalidate, shape, and coverage-guard suite, which is the right scope: `fixture_test.go`'s `fakeLoomShuttle` is shared by every test in the package, so a change to it that broke an existing row would surface immediately.
`coverage_guard_test.go`'s `TestCoverageGuard_EveryLoomRowHasAnEngine` additionally re-proves the `Shed Recipe Registry Invariant` across the whole change.

`internal/lyxcwd` is in scope solely for `docslink_test.go`'s `TestEnforcement_MarkdownLinks`, which is the machine check behind the **Markdown Link Integrity** invariant and the only thing that catches a broken link or a dangling `#anchor` in the four markdown files cards 14 and 15 edit — in particular the preserved `#crash-recovery--resume-on-output-files-not-live-processes` anchor that `manifest/roadmap.md` and `manifest/designs/loom.md` both link.
Without this package in the verify scope, a docs-only card would have no runnable gate at all.

Both packages are untagged Tier 1.
