Twill-plan bench directive, round 2 (2026-10-11); rounds 3, sonnet and sonnet-medium-fable reuse it byte for byte.

## Step 2b — Lens review by forks: spawn before you write

This step sits between Step 2 and Step 3, and its spawn is the first thing you do once Step 2's reading is done.
Your first Write of any file under `PAIRDIR/_lyx/plan/` comes after the Agent call in item 2 below, never before it.
Do not draft a card, the overview or any part of the plan in a file before that call; the forks wait for the plan, so spawning first costs you nothing.

1. When Step 2's reading is done, read the rubric at `/home/knatte/Code/loomyard-LYXHUB/_board/_lyx/stencils/loom/loom-rubric-plan-review.md`, then state the cards you will write in a text reply, one line each: number, slug, the decisions it carries.
2. In the same turn, spawn every lens below in one message through the Agent tool with `subagent_type: "fork"`, one fork per lens, and never pass a `name`.
   Each fork's prompt is the fork boilerplate below plus its lens text and nothing else: no design hint, no direction to read code.
3. Now do Step 3, Step 4 and Step 5: write the plan, and run `lyx loom validate-plan` until it passes.
4. Write the ready marker `PAIRDIR/.lyx/twill-plan/plan.ready` only after the self-check passes, creating its parent directory first.
   The forks wait for that file and review nothing before it exists.
5. Wait for every fork to return; ending your turn while forks are outstanding is allowed.
6. Judge every finding, and merge duplicates across lenses.
   Fix the plan, or reject the finding.
   A rejection cites the code you read to reject it: a `path:line` or glyph you opened in this session, and what that code shows against the finding.
   An uncited rejection is not allowed: a finding you cannot refute from code you have read is fixed, not rejected.
   Keeping a waiver, a deferral or a test you chose not to plan against a finding is a rejection, and needs the same citation.
7. Record the lens review in the file `PAIRDIR/.lyx/twill-plan/lens-review.md`: one line per finding, with its lens, its severity, and `fixed` with what changed, or `rejected` with the citation and why.
   Name a fork that returned no report.
8. Run `lyx loom validate-plan` again until it passes, then end your turn.

One fork pass per writer spawn.
A gate re-prompt, a parent-review reject or a later turn of the same session never spawns forks again.

A writer relaunched after its process died has lost every fork it spawned, and never spawns again.
On its next turn it resumes at the first of the steps from writing the files onward that it had not finished, with the findings it already holds.
It records in the file `PAIRDIR/.lyx/twill-plan/lens-review.md` that the lens review was cut short, naming each fork that had not returned as "no report", or that no forks were spawned when it died before the spawn finished.

The ready marker and the lens-review file are two files you create beside the plan, outside the plan directory, and you create their parent directory too.
They are untracked state that nobody commits.

Lenses:
- criteria-coverage: Focus on the plan against the decision record: list every decision and every acceptance criterion, and for each find the card whose Intent carries it and the planned test row that would fail if it were broken. A criterion the plan argues holds without a test, waives or defers is a finding, and so is a decision, failure exit or branch the record names that no row reaches. Apply the Plan-Review rubric's "Fidelity to the answer key". Report anything else you notice too — emphasis, never exclusion.
- row-reachability: Focus on whether each planned test row can do what the card says: open every test function a card extends or names, and the fakes, fixtures and helpers it runs on, and check that the row's input reaches the branch it claims (the fake can produce that state at that point of the sequence), that its assertion would fail if the code were wrong, and that each new fake field or helper the row needs is listed on the card. Apply the Plan-Review rubric's "Reachability of a prescribed refusal" and "Verify coverage". Report anything else you notice too — emphasis, never exclusion.
- forced-edits: Focus on what each change forces outside the members a card names: for every member a card changes, find its callers, the tests that assert its output or wording, and the comments, doc files and stated reasons (such as a `//testtiming:keep` reason) that describe it, and report each one the change makes wrong that no card lists. Check that each `Create` handle's package is the package clause of the file the Intent puts it in, an external `_test` package included, and that each listed glyph is the one `lyx quarry` gives. Apply the Plan-Review rubric's "Member glyphs on an `Edit` card" and "The re-sign arrow". Report anything else you notice too — emphasis, never exclusion.
- test-economy: Focus on test economy: for every new top-level test and every new row, open the test file it lands in and the existing tests of the same function, and report a new top-level test that should be a row of an existing test, a row that repeats a behavior an existing test already reaches, and a new top-level test that its package's row in `cmd/lyx/testdata/test-budget.yaml` does not budget. Apply the Plan-Review rubric's "Test economy". Report anything else you notice too — emphasis, never exclusion.

### Fork boilerplate

Copy these lines verbatim into every fork's prompt, ahead of its lens text.

- Your first and only action until the file `PAIRDIR/.lyx/twill-plan/plan.ready` exists is `lyx shuttle await PAIRDIR/.lyx/twill-plan/plan.ready`, run with the Bash tool's `timeout` at 300000.
  On `ready: false`, run the same command again in a turn that does nothing else.
  Read nothing, judge nothing and write no note before `ready: true`.
- Give up after the wait has reported `ready: false` 15 times in a row, and return the one-line report "no record to review".
- After `ready: true`, read the files under `PAIRDIR/_lyx/plan/` (`00-overview.md` and every card file its Card Index names) and review them through your lens as an outsider.
- Your context already holds the writer's exploration and the rubric, so never re-read what your context shows.
  Make at most 6 further tool calls, each for something your lens needs that your context lacks.
  End your report with one line per such call, saying what you read and why your context lacked it.
- You are read-only: never write, edit or delete a file, run no git command, and never call the Agent tool.
- Report defects only, never a check that passed.
  Each finding carries a severity: `BLOCKING` when the plan as written fails its verify or ships a wrong behavior, `MEDIUM` when a decision, criterion or branch goes unreached or untested, `LOW` otherwise.
- Return your findings only as your final message, each with its severity, the card or section it concerns, a one-line summary and the evidence: the `path:line` or glyph you read that shows it.

A fork never calls the Agent tool, so the writer is the only session that spawns.
