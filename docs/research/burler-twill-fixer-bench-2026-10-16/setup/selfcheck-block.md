## Self-check of your fix by forks (BLOCKING — after every fix, before the fixer-report)

Once every finding has its disposition and every change is made, and before you write the fixer-report, run this procedure once.
It checks the change set you just made, which no later step of this round reviews.

1. Read the fix-check rubric at `PAIRDIR/.lyx/btf/fix-check-rubric.md`, so every fork you spawn inherits it.
2. Spawn the three lenses below in one message through the Agent tool with `subagent_type: "fork"`, one fork per lens, and never pass a `name`.
   Each fork's prompt is the fork boilerplate below plus its lens text and nothing else: no hint at what you think is wrong, no direction to read code.
3. Wait for every fork to return; ending your turn while forks are outstanding is allowed.
4. Judge every fork finding: merge duplicates across lenses, then weigh each finding twice: its premise, against the code and the review, and the change it proposes.
   Give it exactly one of three dispositions, decided in this order; severity decides how a finding is reported, not whether it is weighed.
   - **Dispute**, on one of two grounds.
     `premise false`: the premise is false against what you read.
     `every fix worse`: the premise holds, but every change that addresses it makes the reviewed files worse.
     One sound change means the finding is fixed, not disputed.
     The evidence is cited text opened in this session, a `path:line` or glyph, a `PATTERN` entry, or a test or caller the change would break, and what it shows against the finding.
     Severity, size, cost, taste, and disagreement with the rubric or `PATTERN` never justify a dispute.
     A finding you cannot refute from what you read is not disputed.
   - **Fix with your own change**: the premise holds, the change the finding proposes is wrong or harmful, and another change resolves the premise.
     Make that change, and name the proposed change you did not make and why, with the same kinds of evidence.
   - **Fix as proposed**: otherwise.
     A finding that proposes no concrete change leaves the change to you.
   A change you make here obeys the same write surface and git discipline as every other fix of this round.
5. Record the self-check in the fixer-report, in a `## Self-check` section after the list of changes: one line per fork finding, with its lens, its severity and its disposition: `disputed` with its ground and the evidence, `fixed differently` with the proposed change you did not make, why, and what changed instead, or `fixed as proposed` with what changed.
   Name a fork that returned no report.

One fork pass per fixer session: a fixer that has already made its spawn call (step 2) spawns no further forks.

Lenses:
- `consistency`: every sibling decision, card, criterion, row or cross-reference that the changed text touches or depends on still agrees with it; a change made in one place whose twin elsewhere was left as it was.
- `claims`: every sentence the fix added or rewrote that asserts a bound, a guarantee, a behaviour, a count or a fact about the code holds against the code at this worktree's HEAD, including claims taken over from the reviewer's proposed fix.
- `way-forward`: every refusal, error, next step, row or test the fix introduced can be carried out or reached from the state it names, and leads somewhere that makes progress.

### Fork boilerplate

Copy these lines verbatim into every fork's prompt, ahead of its lens text.

- The fixer has just changed the reviewed files to fix the review's findings, and you were spawned before it writes its fixer-report.
  Check that change set through your lens as an outsider, as your context holds it, and never re-read a file your context already shows in its changed form.
- Your context already holds the review, the files as the fixer read and changed them, and the fix-check rubric, so never re-read what your context shows.
  Make at most 6 further tool calls, each for something your lens needs that your context lacks.
  End your report with one line per such call, saying what you read and why your context lacked it.
- You are read-only: never write, edit or delete a file, run no git command, and never call the Agent tool.
- Return your findings only as your final message, each with a severity (`BLOCKING`, `MEDIUM` or `LOW`, as the rubric defines them), the changed text it concerns, a one-line summary and the evidence; return "no findings" when you have none.

A fork never calls the Agent tool, so the fixer is the only session that spawns.
