# Batch: fabric-git-invariant-fix

```yaml
task: 'loom: Discussion-Burler Fabric Git Invariant fix'
batch: 'fabric-git-invariant-fix'
number: 1
cards: 3
verify: go test ./... && go test -tags integration ./...
depends-on: []
```

## Batch Scope

This batch delivers the whole task: it flips the `Discussion-Burler` row's `profile.fix-scope` from `source` to `overlay` and gives `Discussion-Bouncer` the `commit_seam: discussion` key that makes the loop owner commit what the now-git-less fixer writes, adds a parse-level regression guard that would have caught the original violation, and rewrites every comment and doc whose text asserts something about the Discussion row that stops being true.
There is no external interface for a later batch to consume — this is the only batch.

Batch-local decisions beyond `## Shared Decisions` in the overview:

- The guard test file is named `overlay_seam_guard_test.go`, matching the existing `seam_enforcement_test.go` naming convention in the same package (one guard concern per file, named for the rule it enforces rather than for the package).
- The guard is factored as one exported-to-the-package helper function taking a parsed recipe, so the shipped-recipe assertions and the synthetic negative cases both call the identical rule rather than two hand-kept-in-step copies.

## Cards

### Card 1: Flip the Discussion segment to overlay-plus-seam, guarded and proved

- **Context:**
  - `CONSTRAINTS.md`
  - `contracts/recipes/recipes.go`
  - `internal/burlerengine/prompt.go`
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/seam_enforcement_test.go`
  - `internal/loomrecipe/shape_test.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/shedbuild/parse.go`
  - `internal/shedbuild/recipe.go`
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/entries_burler.go`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomrecipe/sequence_test.go`
- **Creates:**
  - `internal/loomrecipe/overlay_seam_guard_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Perform the work in the order below; the ordering is the TDD proof this task requires and is an execution step, never a separate commit.

  **Step 1 — write the guard first, against the unmodified recipe.**
  Create `internal/loomrecipe/overlay_seam_guard_test.go` in package `loomrecipe`.
  It may import `github.com/Knatte18/loomyard/contracts/recipes`, `github.com/Knatte18/loomyard/internal/shedbuild`, and `github.com/Knatte18/loomyard/internal/lyxdirs` freely — `internal/loomrecipe/seam_enforcement_test.go`'s import allowlist binds production files only and skips every `_test.go` file, so this file is outside its reach.
  Use `lyxdirs.LyxDirName` for the overlay-directory prefix rather than writing the literal string, matching the convention `internal/loomrecipe/fixture_test.go` already follows and satisfying the Lyxdirs Single-Declarer Invariant's spirit.

  Factor the rule as one package-level helper, `assertOverlayBurlerCommitSeams(t *testing.T, r shedbuild.Recipe)`, taking an already-parsed `shedbuild.Recipe`.
  For every `shedbuild.Row` whose `Engine` is the `BurlerRound` string:
  read `Config["profile"]` as a `map[string]any`, then its `"target"` sub-map, then that sub-map's `"paths"` entry as a slice of strings.
  When the row declares no `profile`, no `target`, or no `paths` entry at all, the row is outside the rule and the helper skips it silently — that absence is the structural exemption, not an allowlist entry.
  When `paths` is present but any element is not a string, or `profile`/`target` is present but not a map, call `t.Fatalf` naming the row: an unexpected shape must fail loudly rather than be skipped, because a silently-skipped row is a guard that does not guard.

  For a row that does declare target paths, determine which of them are overlay paths — those whose first path segment equals `lyxdirs.LyxDirName`.
  If none are, the row is outside the rule and is skipped.
  If at least one is, apply both halves of the rule:

  - The row's `profile["fix-scope"]` must be the string `overlay`.
    Report a missing key and a non-`overlay` value as distinct failures, each naming the row.
  - Collect the second path segment of every overlay target path — the overlay subdirectory each lives under.
    If the row's overlay target paths do not all share one single such subdirectory, call `t.Errorf` naming the row and the differing subdirectories: a row straddling two overlay trees has no single correct seam and the guard must not guess one.
    Otherwise find the row in the same `Recipe` whose `Engine` is the `Bouncer` string and whose `Segment` equals this row's `Segment`.
    If no such row exists, `t.Errorf` naming the Burler row and its segment.
    If it exists, read its `Config["commit_seam"]` as a string: a missing or empty key is a failure naming the row, and a present value that is not equal to the shared overlay subdirectory is a failure reporting both the value found and the value required.

  Add the shipped-recipe test, `TestShippedRecipe_OverlayBurlersCommitThroughSeam`: parse `recipes.LoomRecipe` with `shedbuild.Parse`, fail the test on a parse error, and pass the result to `assertOverlayBurlerCommitSeams`.
  This half is a straight read of the real embedded file and must never mutate a copy of it.

  Add the synthetic negative cases as a second test, `TestOverlayBurlerCommitSeams_RejectsBadShapes`, table-driven over small hand-written recipe YAML documents fed through the same `shedbuild.Parse`.
  Each case runs the helper against a `*testing.T` whose failures are captured rather than propagated — use a nested `t.Run` with a helper that records whether the rule reported a failure, so a case asserting rejection does not fail the outer test.
  The cases to cover, each of which must be reported rather than skipped: an overlay-targeting Burler carrying `fix-scope: source`; one carrying `fix-scope: overlay` whose partner Bouncer omits `commit_seam` entirely; one whose partner Bouncer carries the wrong seam value; one whose `segment` names no Bouncer row at all; and one whose target paths straddle two different overlay subdirectories.
  Also cover two accept cases so the rule is shown not to be a rejection-only assertion: a Burler declaring only `instructions` and no target paths passes with `fix-scope: source`, and a well-formed overlay Burler with a correctly-valued partner seam passes.

  The whole file is a pure in-memory parse — it must spawn no process, run no `exec.Command`, use no `gitexec`, and copy no fixture tree, so it stays untagged under the Test Tier Purity Invariant.

  **Step 2 — run the guard and record that it fails.**
  Run the new test against the unmodified recipe.
  It must fail on the `Discussion-Burler` row twice: once for the `fix-scope` value and once for the partner `Discussion-Bouncer` row's absent `commit_seam`.
  It must not fail on the Plan or Webster rows.
  If it does not fail exactly that way, the guard is wrong and must be corrected before proceeding — a guard that passes against the known-bad input proves nothing.

  **Step 3 — flip the recipe.**
  In `contracts/recipes/loom-recipe.yaml`, change the `Discussion-Burler` row's `profile.fix-scope` value from `source` to `overlay`, and add a `commit_seam: discussion` key to the `Discussion-Bouncer` row's `config` block.
  Change nothing else about either row: `run_subdir`, `artifact_paths`, `rubric_stencil`, `max_bounces`, the mutual `on_stuck` edges, the target paths, the `fasit` block, and `tool-use: true` all stay exactly as they are.
  Rename no row — the recipe header comment records that a row rename without a matching `loomshed.Name*` constant rename breaks resume for in-flight tasks.

  **Step 4 — rewrite the five recipe comment sites whose text the flip makes false.**
  These comments are load-bearing recorded design decisions and three of them cite the Discussion row's `source` value as their own reason to exist, so leaving them would have the file assert in four places a fact it contradicts two lines away.

  1. The `Discussion-Burler` row's `fix-scope` key gains a rationale comment in the shape the `Plan-Burler` row's already has: name the Fabric Git Invariant as the reason the value is `overlay`, and name the `commit_seam` partner key on `Discussion-Bouncer` as what actually commits the round's writes.
  2. The new `commit_seam: discussion` key on `Discussion-Bouncer` gains a rationale comment mirroring `Plan-Bouncer`'s "required rather than optional here" reasoning: an overlay round runs no git at all and nothing else in the segment commits, so without the seam every approved fix would stay uncommitted in the weft working tree.
  3. The `Plan-Burler` row's `fix-scope` comment currently says the value is deliberately NOT the shipped Discussion row's `source` and that the Discussion row's violation is recorded and left to its own roadmap item.
     The two rows now match, so it must say that instead, and must no longer point at a roadmap item this change closes.
  4. The `Webster-Burler` row's `fix-scope` comment currently justifies `source` as matching the shipped Discussion row rather than `Plan-Burler`'s `overlay`.
     It is now the only `source` row in the recipe, so it must explain why it alone is legitimate — its target is the repo's own warp source, which the Fabric Git Invariant names as the one explicitly permitted agent commit — rather than pointing at a row that no longer matches it.
  5. The `Webster-Bouncer` row's comment currently says it carries no `commit_seam` key "deliberately unlike Plan-Bouncer".
     It is now unlike both other Bouncer rows, and the comment must say so.

  **Step 5 — move the sequence assertion.**
  In `internal/loomrecipe/sequence_test.go`, change the `commitDiscussionCalls != 1` check to `!= 2` and rewrite both its preceding comment and its `t.Errorf` message.
  The comment currently explains that only one segment commits through this seam so the count stays 1; it must instead state that the count is now two callers — `Discussion-Write`'s own commit plus `Discussion-Bouncer`'s approval commit through the row's `commit_seam` key — mirroring the comment already sitting on the `commitPlanCalls != 2` assertion immediately below it.
  The failure message must name both callers, matching that neighbouring message's shape.
  Change nothing else in the file: the `commitPlanCalls != 2` assertion, the three `fakeLoomBurler` round count, and the three bouncer-judge spawn count all stay as they are.

  **Step 6 — re-run the guard and the suite.**
  The guard must now pass against the shipped recipe, and the full batch `verify:` command must pass.
  Do not modify `internal/shedrecipe/entries_bouncer.go`, `internal/shedrecipe/entries_burler.go`, `internal/burlerengine/prompt.go`, or `internal/loomrecipe/shape_test.go` — all four already ship the behaviour this card relies on, and a needed change in any of them means the fix went wrong somewhere.
- **Commit:** `fix(loom): flip Discussion-Burler to overlay with a discussion commit seam`

### Card 2: Retarget the two Go doc comments that name the wrong row

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/shedrecipe/entries_bouncer.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
  - `internal/shedadapters/bouncer_commit_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Two doc comments outside the recipe assert facts card 1 made false.
  Both are comment-only edits — change no code, no test body, and no assertion.

  In `internal/shedadapters/bouncer_commit_test.go`, rewrite the doc comment on `TestBouncer_Commit_NilIsNotAnError`.
  It currently justifies the test as pinning the shipped `Discussion-Bouncer` row's behaviour as unchanged because that row's `BouncerConfig` never sets `Commit`.
  That row now does set `Commit`, so the comment must instead name `Webster-Bouncer` as the shipped nil-`Commit` row it pins, on the same grounds — that row carries no `commit_seam` key because its Burler partner commits its own fixes.
  The test's body and its three expectations do not change, and neither do the other two tests in the file.

  In `internal/loomcli/wiring.go`, extend the doc comment on the `CommitDiscussion` closure.
  It already documents the closure's idempotence — `CommitAnchoredPaths` reports `committed == false` for an already-clean, already-tracked path and the closure discards that result, returning only the error.
  Add the note that this idempotence now covers two callers rather than one, since the `Discussion-Bouncer` row's approved settle reaches this same closure through the row's `commit_seam: discussion` config key.
  Phrase it to match the equivalent sentence the `CommitPlan` closure's doc comment immediately below already carries for the identical reason, so the two read as one pattern.
  The closure body does not change: its pathspec, its `NewMutations("")` record, its commit message, and its `EnvSyncOptions()` all stay exactly as they are.
- **Commit:** `docs(loom): retarget the two doc comments naming the old Discussion-Bouncer shape`

### Card 3: Record the per-segment commit split in the module doc and split the roadmap item

- **Context:**
  - `CLAUDE.md`
  - `CONSTRAINTS.md`
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/burlerengine/prompt.go`
- **Edits:**
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Both edits are prose in `.md` files and must follow the repo's semantic-line-break rule: one sentence per line, an additional break at an internal independent-clause boundary, a plain newline as the soft break, and never a fixed-column hard wrap.

  In `manifest/designs/loom.md`, work on the `## The gate` section.

  First, correct the sentence in that section's black-box paragraph reading that loom drives all phases identically because the verdict contract is invariant, "only the review *profile* (rubric + fasit) differs per phase".
  Rewrite it rather than supplementing it — it is already inaccurate today, since the fixer round's write surface and the segment's commit seam differ per phase too, and this change sharpens that divergence.
  The corrected sentence must name the fix-scope and the commit seam as the other per-phase axes alongside the profile, keeping the black-box claim itself (loom drives every phase identically because the verdict contract is invariant) intact.

  Second, add a short explicit record to the same section of which review segment fixes what and commits how: the Discussion and Plan segments fix overlay content and commit it through the loop owner's commit seam, while the Webster segment fixes warp source and commits each fix itself.
  Cite the Fabric Git Invariant as the reason the split exists — it reserves every weft commit to the loop owner in Go and names the agent's warp commit as the one exception.
  Keep it to the cross-segment picture; the per-row rationale belongs in the recipe comments card 1 wrote and must not be duplicated here.

  In `manifest/roadmap.md`, split the Planned item for this task rather than moving it whole.
  It is the item under `### loom: real LLM producers` beginning with the `Discussion-Burler` `fix-scope: source` violation, and its second paragraph folds in two further defects — both review segments resolving their overlay paths against `Env.WorktreeRoot` while the matching commit closures anchor at `AnchorPath()`, and neither segment clearing its Bouncer run directory when a downstream row bounces back through the segment.
  Neither folded defect is in this task's scope, so:

  - Move a Done entry into the `## Done` section covering the shipped half only — the `Discussion-Burler` row now runs `fix-scope: overlay` and `Discussion-Bouncer` commits its approved settle through `commit_seam: discussion`, restoring compliance with the Fabric Git Invariant, with a parse-level guard added so the class of violation cannot ship again.
    Give it a `See` line pointing at the gate section of the loom design doc, matching the two existing Done entries' shape.
  - Leave a new Planned item in the `### loom: real LLM producers` sub-category carrying the two folded defects, preserving their existing wording and their existing `See` line, and drop the now-shipped first paragraph from it.
  - Write every item's ordered-list marker as the literal `1.`, per the file's own `## Maintenance` note that numbering is automatic and restarts at 1 in each section.
- **Commit:** `docs(loom): record the per-segment commit split and split the roadmap item`

## Batch Tests

`verify:` is `go test ./... && go test -tags integration ./...`, the repo-wide sweep in both tiers.

This is deliberately unscoped rather than a per-package run over the three changed packages (`internal/loomrecipe`, `internal/shedadapters`, `internal/loomcli`), and the justification is the subject of the change itself: `contracts/recipes/loom-recipe.yaml` is embedded into the binary by `//go:embed` in `contracts/recipes/recipes.go`, with no on-disk runtime copy, so every package that builds the loom graph consumes it.
A scoped run would not catch a consumer elsewhere in the tree that the flipped row's new `commit_seam` construction path now reaches — notably `requireSeam`'s guard on `Env.CommitDiscussion`, which any recipe-building test with a nil closure would now trip.
The integration tier is included for the same reason and because it is the shape the configured `pipeline.done_gate` already runs.

What the sweep must show:

- The new `internal/loomrecipe/overlay_seam_guard_test.go` passes against the shipped recipe after card 1's flip, and its synthetic negative cases each report the shape they target.
- `internal/loomrecipe/sequence_test.go` records exactly two `CommitDiscussion` invocations and, unchanged, exactly two `CommitPlan` invocations, three `fakeLoomBurler` rounds, and three bouncer-judge spawns.
- Coverage that must keep passing untouched, where a needed change signals the fix went wrong: `internal/shedrecipe/entries_bouncer_test.go`'s `TestBouncerEntry_CommitSeam` in full, `internal/shedadapters/bouncer_commit_test.go`'s three cases (card 2 changes one doc comment and nothing else), `internal/burlerengine`'s `TestComposePrompt_FixScope` and its `Profile.validate` coverage, and `internal/loomrecipe`'s `coverage_guard_test.go`, `shape_test.go`, `resume_test.go`, `revalidate_test.go`, `recipe_test.go`, and `seam_enforcement_test.go`.

No new integration- or smoke-tagged test is added.
The change is a recipe value plus a commit-seam wiring that the untagged fixture already drives end-to-end through a real `Shed` run with fake closures, and driving a genuine weft commit would need a two-worktree git pair that `internal/loomrecipe`'s own tier deliberately excludes.
