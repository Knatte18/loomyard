<!-- This is the twill directive: the host-neutral block that has a writer review its own finished record through lenses, one fork per lens, before it ends.
     internal/twilldirective renders it, and the spawning module hands that renderer every host value, so this block names no file, step, verb or section of any one host.
     It declares these required markers:
     {{.reviewed_files}}, {{.rubric_path}}, {{.lens_list}}, {{.format_step}}, {{.self_check_step}}, {{.self_check_command}}, {{.final_self_check_command}}, {{.disposition_place}}, {{.spawn_point}}, {{.rejection_rule}} and {{.severity_rule}}. -->

## Lens review by forks

{{.spawn_point}}

1. Read the rubric at `{{.rubric_path}}`, so every fork you spawn inherits it.
2. Spawn every lens below in one message through the Agent tool with `subagent_type: "fork"`, one fork per lens, and never pass a `name`.
   Each fork's prompt is the fork boilerplate below plus its lens text and nothing else: no design hint, no direction to read code.
3. Wait for every fork to return; ending your turn while forks are outstanding is allowed.
4. Judge every finding with the same skepticism, and merge duplicates across lenses.
   {{.rejection_rule}}
5. Record the lens review at `{{.disposition_place}}`: one line per finding, with its lens and whether it was fixed or rejected and why.
   Name a fork that returned no report.
6. Run `{{.final_self_check_command}}` again until it passes, then end your turn.

One fork pass per writer spawn.
A gate re-prompt, a parent-review reject or a later turn of the same session never spawns forks again.

A writer relaunched after its process died before its Agent call still spawns once.
On its next turn it finishes writing the reviewed files per `{{.format_step}}`, runs `{{.self_check_command}}` per `{{.self_check_step}}` until it passes, and then runs this procedure from its first step.

A writer relaunched after its process died at or after its Agent call has lost every fork it spawned, and never spawns again.
On its next turn it resumes at the first of the steps from judging the findings onward that it had not finished, with the findings it already holds.
It records at `{{.disposition_place}}` that the lens review was cut short, naming each fork that had not returned as "no report".

Lenses:
{{.lens_list}}

### Fork boilerplate

Copy these lines verbatim into every fork's prompt, ahead of its lens text.

- The writer wrote {{.reviewed_files}} and passed its self-check before spawning you.
  Review them through your lens as an outsider, as your context holds them, and never read them again.
- Your context already holds the writer's exploration, the reviewed files and the rubric, so never re-read what your context shows.
  Make at most 6 further tool calls, each for something your lens needs that your context lacks.
  End your report with one line per such call, saying what you read and why your context lacked it.
- You are read-only: never write, edit or delete a file, run no git command, and never call the Agent tool.
- Return your findings only as your final message, each with {{.severity_rule}}, the section or decision it concerns, a one-line summary and the evidence.

A fork never calls the Agent tool, so the writer is the only session that spawns.
