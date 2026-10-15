<!-- This is the twill writer-fork part: the host-neutral procedure in which one fork, fork0, writes the reviewed files while the writer explores before it and reviews what it wrote after.
     internal/twilldirective renders it into the twill directive, and the spawning module hands that renderer every host value, so this block names no file, step, verb or section of any one host.
     It declares these required markers:
     {{.explore_step}}, {{.input_record}}, {{.reviewed_files}}, {{.last_file}}, {{.files_dir}}, {{.format_step}}, {{.self_check_step}}, {{.self_check_command}} and {{.disposition_place}}. -->

In this run one fork, fork0, writes the reviewed files: you explore, fork0 thinks and writes, and you review and finish what it wrote.
Do these steps in place of running `{{.format_step}}` and `{{.self_check_step}}` yourself, in this order:

a. Explore per `{{.explore_step}}` until your context holds everything the reviewed files will name: every file, member, test, fake and fixture they will cite.
   Design none of their content, neither in a reply nor in a file.
b. Do step 1 of the lens review below now, reading the rubric, so fork0 and every lens fork inherit it; when you run the lens review, skip its step 1.
c. Spawn one fork, fork0, through the Agent tool with `subagent_type: "fork"`, never passing a `name`.
   Its prompt is these lines, copied verbatim, and nothing else.
   Wait for it; ending your turn while it is outstanding is allowed.
   - You are fork0, the writer: your context holds {{.input_record}}, the exploration and the rubric.
   - Think the reviewed files through, then write them per `{{.format_step}}`, {{.last_file}} last, and run `{{.self_check_command}}` per `{{.self_check_step}}` until it passes.
   - Read only what your context lacks and the reviewed files need.
   - Never call the Agent tool, never write {{.disposition_place}}, and run no git command.
   - Return one line as your final message: the files you wrote and that `{{.self_check_command}}` passed.
     Add no reasoning, design note or summary.
d. When fork0 has returned, read {{.reviewed_files}}, each in full with the Read tool, and take them as the files to review, not your own draft: you see them without fork0's reasoning.
   Run `{{.self_check_command}}` and fix what it reports until it passes.

Then run the lens review below once; fork0 does not count against its one fork pass, and the writer it names is you.
This session spawns fork0 at most once and the lens forks at most once: a fresh session is a new writer, and a resumed one is not.

- If fork0 returns and {{.last_file}} is missing or the self-check fails, complete or fix the files yourself per `{{.format_step}}` and `{{.self_check_step}}`, and spawn no second fork0.
- A fresh session started after an earlier one ended before {{.last_file}} existed finds its partial files archived: run this prompt from its first step, fork0 included.
- A session resumed in the same conversation before the fork0 call: go on from the first step not finished, fork0 included.
- A session resumed while the fork0 call was in flight has lost fork0, and no second one is spawned: read every file in {{.files_dir}} in full, complete the files yourself as in the first rule, then run the lens review once.
- A session resumed after fork0 returned and before the lens spawn call: spawn no second fork0, finish step d if unfinished, then run the lens review once.
- A session resumed at or after the lens spawn call has lost every lens fork, and none is spawned again: resume at the first of the lens review's steps from judging the findings onward that you had not finished, with the findings already held, and record at {{.disposition_place}} that the lens review was cut short, naming each fork that had not returned as "no report".
- A session that ended after {{.last_file}} existed: the run counts as finished; while the session is live the gate re-prompts it on a missing lens review or its findings, and once it is not, the gate is evaluated once over the files as they stand, so a run with no recorded lens review fails; a later resume takes the matching resumed rule.
- A re-prompted writer spawns no fork0 and does step d first if it has not, then follows the re-prompt rule below.
