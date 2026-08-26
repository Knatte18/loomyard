# Batch: recipe-wiring-and-regression

```yaml
task: 'loom: Plan-Write/Plan-Validate approval deadlock (F7)'
batch: 'recipe-wiring-and-regression'
number: 6
cards: 5
verify: go test ./internal/loomrecipe/...
depends-on: [5]
```

## Batch Scope

This is the keystone batch: it turns the seam on in the shipped recipe and lands the regression test that would have caught F7 on the day row 8 shipped.

Every card here has to move together.
Adding `approve_seam: plan` to the `Plan-Bouncer` row makes `loomrecipe.New` fail at `requireSeam` against a nil `Env.ApprovePlan`, so the fixture's own `env.ApprovePlan` must be filled in the same batch or every test in the package fails at construction.
Flipping the fake `Plan-Write` to stop self-approving is what makes the nineteen-row sequence test genuinely exercise the deadlock, and it passes only once the seam is on.
And the existing `Plan-Revalidate` regression test injects its regression by clearing the approval flag, which the seam now undoes by construction, so that test's corruption has to be re-pointed in the same commit range that introduces the masking.

Batch-local decision: the replacement corruption is an **orphan card file**, and the constraint behind that choice is sharp.
`planValidate.Call` maps a `ParsePlan` failure to a returned error, never to `Stuck`, so an unparseable corruption aborts the run before the bounce assertion is ever reached.
The corruption must therefore be parseable-but-invalid.
Writing an unindexed extra `.md` file into the plan directory satisfies that: `ParsePlan` only opens files the Card Index names, so it succeeds, and `checkIndexFileConsistency` then reports `index-file-mismatch` for the file no card names.
Two corruptions that look plausible and do not work are recorded here so nobody re-derives them: a Card Index entry naming an absent card file hard-errors in `parseCardFile`, and dropping the index entry while leaving the file on disk hard-errors in `parseCardIndex` on an empty index.

## Cards

### Card 21: Turn the approve seam on in the shipped recipe

- **Context:**
  - `internal/shedrecipe/entries_bouncer.go`
  - `contracts/stencils/loom/loom-template-plan.md`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `approve_seam: plan` to the `Plan-Bouncer` row's `config:` block, beside its existing `commit_seam: plan` key, with a comment stating that the approved settle writes the approval flag through this seam immediately before the commit seam fires, so the flag lands inside the commit rather than as working-tree dirt afterwards, and that this row is the only place in the list that knows the plan passed review.
  Add the key to no other row: neither `Discussion-Bouncer` nor `Webster-Bouncer` has an artifact with an approval flag, and both keep the key absent, which leaves their seam nil exactly as an omitted `commit_seam` already does.
  Correct the `Plan-Burler` row's `fasit.instructions` prose, which currently tells the fixer round that the mechanical checks over the plan format are already enforced upstream by `Plan-Validate` — true for fifteen of the sixteen check IDs but no longer true for `plan-unapproved`, which now moves downstream to `Plan-Revalidate`.
  Carve out that one exception in the same sentence rather than editing a bare count.
  In the same instructions string, add the self-approval prohibition explicitly: the fixer round may never write the `approved:` key in the plan's overview file.
  The rule binds this row for a sharper reason than it binds the writer — the fixer runs inside the review segment, so a fixer that set the flag would be approving the very artifact the round is judging — and it is not a hypothetical, since this row runs `fix-scope: overlay` with the plan directory in its `target.paths`, making the overview a file it is already permitted to write.
  Leave the neighbouring `Webster-Burler` row's `fasit.instructions` untouched: it names `Plan-Validate` and `Plan-Revalidate` together, and together those two rows do still enforce all sixteen.
- **Commit:** `21: loom-recipe: approve_seam on Plan-Bouncer, fixer prohibition on Plan-Burler`

### Card 22: Fix the fixture that let F7 escape CI

- **Context:**
  - `internal/planparser/approve.go`
  - `internal/planparser/parse.go`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:**
  - `internal/loomrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Fill `ApprovePlan` in the `shedrecipe.Env` literal `buildSequenceFixture` builds, as a closure running the real `planparser.SetApproved` over the same plan directory the fixture already computes for the fake shuttle's own plan-role branch.
  Use the real function rather than a fake: the seam under test is the write, and a fake that merely recorded the call would let a broken writer pass.
  Change `fakeLoomShuttle`'s `"plan"`-role branch — the stand-in for the `Plan-Write` row — so it writes the unapproved overview the real writer is required to write, rather than the approved one it writes today.
  That single argument flip is the regression test for F7: the fake writer currently self-approves, which the plan stencil forbids the real writer from doing, so the fixture has been handing the review gate a plan the production writer can never produce.
  Flip the `buildSequenceFixture` seed call that pre-writes a plan directory before the run to seed an unapproved plan too.
  That seed is inert at the gate — the plan writer's own rotation archives every top-level `.md` file in the plan directory before the shuttle runs, which is why the fake rewrites the whole directory rather than only its declared output file — but leaving it approved would be dishonest about what the fixture models.
  Before flipping it, grep this package's other test files for tests that read that seeded plan's approval value directly, and fix or re-point any that do.
  Re-point `fakeLoomBurler`'s corruption field off the approval flag: it currently rewrites a named overview path with an unapproved overview, which the new seam undoes moments later by construction.
  Replace it with a field naming the plan directory into which `Run` writes one extra unindexed card file that no Card Index entry names, producing an `index-file-mismatch` finding the seam can never mask.
  Rename the field so it no longer claims to write an overview, and update both its own doc comment and `fakeLoomBurler`'s type-level doc comment to describe the orphan-file injection and why an unparseable corruption would not do.
  Update `buildSequenceFixture`'s own doc comment where it describes the fake writer as producing an approved plan.
- **Commit:** `22: loomrecipe: stop the fake Plan-Write self-approving`

### Card 23: Assert the seam actually wrote the flag

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/planparser/parse.go`
- **Edits:**
  - `internal/loomrecipe/sequence_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `TestSequence_FullRunBlocksAtPublish`'s existing expected-history list must still hold unchanged after card 22 — all nineteen entries, in the same order, with the plan gate reaching `Batchifier` through a `Done` at both `Plan-Validate` and `Plan-Revalidate`.
  Under the pre-fix code the fake writer's flip would make `Plan-Validate` report `Stuck` and the sequence bounce, so this list passing is the assertion that closes F7.
  Add one further assertion in the same test: after the run, parse the fixture's own plan through `planparser.ParsePlan` and assert the returned plan's `Approved` field is true, so a fixture that silently stopped exercising the seam — a nil `ApprovePlan`, a no-op closure, a fake writer that started self-approving again — cannot pass.
  Update the test's doc comment and the expected-list's own doc comment where either describes the plan the fake writer produces.
- **Commit:** `23: loomrecipe: assert the plan is approved after a clean run`

### Card 24: Re-point the post-segment regression test at a format fault

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/planparser/validate.go`
- **Edits:**
  - `internal/loomrecipe/revalidate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `TestSequence_PlanRevalidateCatchesPostSegmentRegression` currently scripts its regression by clearing the approval flag, which the seam now rewrites moments later, so the bounce it asserts would stop happening.
  Re-point it at card 22's renamed corruption field, setting it to the fixture's plan directory rather than to the overview path, so the injected regression is the orphan card file and the finding is `index-file-mismatch`.
  The test's own name, subject, and every assertion stay as they are: `Plan-Revalidate` reports `Stuck` and the history entry immediately following it names `Plan-Write`.
  Update the test's doc comment, which names the `plan-unapproved` check as the mechanism, to name the format fault instead, and state why the corruption must be parseable-but-invalid — the plan gate maps a parse failure to a returned error rather than to `Stuck`, so an unparseable corruption would abort the run before the bounce assertion is reached.
  Keep the existing paragraph stating the test deliberately asserts nothing about what happens after the bounce.
- **Commit:** `24: loomrecipe: re-point the revalidate regression at index-file-mismatch`

### Card 25: The negative cases, expressed both dynamically and statically

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/overlay_seam_guard_test.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/loomshed/planvalidate.go`
- **Edits:** none
- **Creates:**
  - `internal/loomrecipe/approveseam_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Removing `approve_seam` from the shipped row is not reachable through the sequence fixture — the recipe is parsed unconditionally from the embedded document, and a nil `Env.ApprovePlan` fails at `requireSeam` before the run starts — so express the negative case two ways in one new test file.
  Dynamically: build the sequence fixture, substitute `env.ApprovePlan` with a **non-nil no-op** closure that writes nothing, and assert the run halts at `Plan-Revalidate` with a `Stuck` and bounces to `Plan-Write` rather than reaching `Batchifier`.
  The guard only checks non-nil, so construction succeeds, the segment approves, nothing writes the flag, and this pins that `Plan-Revalidate` is genuinely enforcing the approval it is now the only row to check.
  Statically: parse hand-authored recipe YAML through `shedbuild.Parse` exactly as `overlay_seam_guard_test.go` already does for its own fixtures, and assert three things — the shipped embedded recipe carries `approve_seam: plan` on the `Plan-Bouncer` row, `require_approved: true` on the `Plan-Revalidate` row, and neither key on any other row;
  a document naming `approve_seam` on a row whose `Env` seam is nil fails to build;
  and a document naming an unknown `approve_seam` value fails to build.
  Give the file a file-level comment naming its single subject: the approval seam is wired as shipped, and a mis-wiring is rejected rather than silently accepted.
  Keep the file untagged and offline — no git spawn, no process spawn — per the Test Tier Purity Invariant.
- **Commit:** `25: loomrecipe: negative cases for the approval seam wiring`

## Batch Tests

`verify: go test ./internal/loomrecipe/...` runs the whole package suite, which is where every assertion in this batch lives and where card 21's recipe edit is observed.

The package-wide scope is the right one and narrower would be wrong: card 21 turns a seam on in the embedded recipe every test in the package builds from, so `sequence_test.go`, `revalidate_test.go`, `resume_test.go`, `shape_test.go`, `recipe_test.go`, `coverage_guard_test.go`, `overlay_seam_guard_test.go`, and `seam_enforcement_test.go` are all regression surface for it, not just the three files the batch edits.
`resume_test.go` in particular drives the fixture six separate times and is the most likely place for card 22's fixture changes to surface an unintended consequence.

The whole package is untagged tier 1 — offline, no git, no process spawn — so running all of it is fast.
