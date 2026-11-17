<!-- This is the twill directive: the host-neutral block that has a writer review its own finished record through lenses.
     It spawns one fork per lens, before the writer ends.
     internal/twilldirective renders it, and the spawning module hands that renderer every host value,
     so this block names no file, step, verb or section of any one host.
     It declares these required markers:
     {{.reviewed_files}}, {{.rubric_path}}, {{.lens_list}}, {{.closing_step}}, {{.spawn_point}}, {{.severity_rule}},
     {{.lens_report_path}}, {{.lens_report_spec_path}} and {{.judge_steps}}.
     {{.judge_steps}} is filled by Directive with one of the two judge part stencils.
     {{.closing_step}} is filled by Directive with the line that ends the writer's turn.
     That line runs the final self-check first, unless the host's files are checked by a gate after the turn.
     It also names these optional markers:
     {{.writer_fork}} and {{.relaunch_rules}}, each filled by Directive with one part stencil and left empty otherwise;
     and {{.checked_clause}}, which Directive leaves empty when the writer runs no self-check. -->

## Lens review by forks

{{.spawn_point}}

{{.writer_fork}}1. Read the rubric at `{{.rubric_path}}`, so every fork you spawn inherits it.
2. Spawn every lens below in one message through the Agent tool with `subagent_type: "fork"`, one fork per lens, and never pass a `name`.
   Each fork's prompt is the fork boilerplate below plus its lens text and nothing else: no design hint, no direction to read code.
3. Wait for every fork to return: in the message after the spawn, run `lym shuttle await-forks` in the foreground with the Bash tool's `timeout` at 300000, and never end your turn while a fork is outstanding, so every fork result reaches you together, in the turn after it answers `ready: true`.
   While it answers `ready: false`, run it again in a turn that does nothing else, until it has answered `ready: false` five times in a row.
   When it fails, or has answered `ready: false` five times in a row, end your turn instead, and take each fork's result as it arrives.
   Edit nothing while any fork is outstanding, whatever an early report says: start judging only once the last fork has returned, judge all findings together, and only then edit.
   Once the last fork has returned, and before you judge, save each fork's final message verbatim to `{{.lens_report_path}}`, with its lens name for `<lens>`, in the format `{{.lens_report_spec_path}}` defines; a fork that returned no report gets its file too.
{{.judge_steps}}
{{.closing_step}}

One fork pass per writer spawn.
A writer re-prompted by a gate fixes the reviewed files itself: if it has not yet made its lens spawn call (step 2), it runs this procedure once; otherwise it spawns no forks.

{{.relaunch_rules}}Lenses:
{{.lens_list}}

### Fork boilerplate

Copy these lines verbatim into every fork's prompt, ahead of its lens text.

- {{.reviewed_files}} were written{{.checked_clause}} before you were spawned.
  Review them through your lens as an outsider, as your context holds them, and never read them again.
- Your context already holds the writer's exploration, the reviewed files and the rubric, so never re-read what your context shows.
  Make at most 6 further tool calls, each for something your lens needs that your context lacks.
  End your report with one line per such call, saying what you read and why your context lacked it.
- You are read-only: never write, edit or delete a file, run no git command, and never call the Agent tool.
- Return your findings only as your final message, each with {{.severity_rule}}, the section or decision it concerns, a one-line summary and the evidence.

A fork never calls the Agent tool, so the writer is the only session that spawns.
