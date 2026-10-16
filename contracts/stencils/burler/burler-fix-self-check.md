<!-- This is burler's fix self-check part: the procedure by which a twill fixer has read-only forks check its own change set before it writes the fixer-report.
     internal/burlerengine's selfCheckBlock (prompt.go) reads and fills it, and composeFixPrompt injects it as burler-step-fix's optional self_check marker value in a round whose profile names a fixer fan.
     It declares two required markers, {{.rubric_path}} and {{.lens_list}}: the deployed fix-check rubric's path, and the fixer fan's lenses as one bullet per lens in fan order. -->
## Self-check of your fix by forks (BLOCKING — after every fix, before the fixer-report)

Once every finding has its disposition and every change is made, and before you write the fixer-report, run this procedure once.
It checks the change set you just made, which no later step of this round reviews.

1. Read the fix-check rubric at `{{.rubric_path}}`, so every fork you spawn inherits it.
2. Spawn the lenses below in one message through the Agent tool with `subagent_type: "fork"`, one fork per lens, and never pass a `name`.
   Each fork's prompt is the fork boilerplate below plus its lens text and nothing else: no hint at what you think is wrong, no direction to read code.
3. Wait for every fork to return; ending your turn while forks are outstanding is allowed.
4. Judge every fork finding: merge duplicates across lenses, then weigh each finding twice, its premise and the change it proposes, and give it one of the three dispositions of the Fix-everything rule above, decided in that order.
   Severity decides how a finding is reported, not whether it is weighed.
   A change you make here obeys the same write surface and git discipline as every other fix of this round.
5. Record the self-check in the fixer-report, in a `## Self-check` section after the list of changes: one line per fork finding, with its lens, its severity and its disposition, as the Fix-everything rule names them.
   Name a fork that returned no report.

One fork pass per fixer session: a fixer that has already made its spawn call (step 2) spawns no further forks.
That holds when your gate re-prompts you and when you are resumed after your forks were lost: spawn nothing, judge the findings you hold, and name each lost fork as "no report".

Lenses:

{{.lens_list}}
### Fork boilerplate

Copy these lines verbatim into every fork's prompt, ahead of its lens text.

- The fixer has just changed the reviewed files to fix the review's findings, and you were spawned before it writes its fixer-report.
  Check that change set through your lens as an outsider, as your context holds it, and never re-read a file your context already shows in its changed form.
- Your context already holds the review, the files as the fixer read and changed them, and the fix-check rubric, so never re-read what your context shows.
  Make at most 6 further tool calls, each for something your lens needs that your context lacks.
  End your report with one line per such call, saying what you read and why your context lacked it.
- You are read-only: never write, edit or delete a file, run no git command, and never call the Agent tool.
- Return your findings only as your final message, each with a severity (`BLOCKING`, `MEDIUM` or `LOW`, as the rubric defines them), the changed text it concerns, a one-line summary and the evidence, and start it with the list your lens asks for, if it asks for one; return "no findings" when you have none.

A fork never calls the Agent tool, so the fixer is the only session that spawns.

