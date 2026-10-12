<!-- This is the twill directive: the host-neutral block that has a writer review its own finished record through lenses, one fork per lens, before it ends.
     internal/twilldirective renders it, and the spawning module hands that renderer every host value, so this block names no file, step, verb or section of any one host.
     It declares these required markers:
     {{.reviewed_files}}, {{.ready_marker}}, {{.rubric_path}}, {{.lens_list}}, {{.format_step}}, {{.self_check_step}}, {{.self_check_command}}, {{.disposition_place}}, {{.spawn_point}}, {{.rejection_rule}} and {{.severity_rule}}. -->

## Lens review by forks

{{.spawn_point}}

1. Read the rubric at `{{.rubric_path}}`, so every fork you spawn inherits it.
2. Spawn every lens below in one message through the Agent tool with `subagent_type: "fork"`, one fork per lens, and never pass a `name`.
   Each fork's prompt is the fork boilerplate below plus its lens text and nothing else: no design hint, no direction to read code.
3. Write the reviewed files per `{{.format_step}}`, and run `{{.self_check_command}}` per `{{.self_check_step}}` until it passes.
4. Write the ready marker `{{.ready_marker}}` only after the self-check passes, creating its parent directory first.
   The forks wait for that file and review nothing before it exists.
5. Wait for every fork to return; ending your turn while forks are outstanding is allowed.
6. Judge every finding with the same skepticism, and merge duplicates across lenses.
   {{.rejection_rule}}
7. Record the lens review at `{{.disposition_place}}`: one line per finding, with its lens and whether it was fixed or rejected and why.
   Name a fork that returned no report.
8. Run `{{.self_check_command}}` again until it passes, then end your turn.

One fork pass per writer spawn.
A gate re-prompt, a parent-review reject or a later turn of the same session never spawns forks again.

A writer relaunched after its process died has lost every fork it spawned, and never spawns again.
On its next turn it resumes at the first of the steps from writing the files onward that it had not finished, with the findings it already holds.
It records at `{{.disposition_place}}` that the lens review was cut short, naming each fork that had not returned as "no report", or that no forks were spawned when it died before the spawn finished.

Lenses:
{{.lens_list}}

### Fork boilerplate

Copy these lines verbatim into every fork's prompt, ahead of its lens text.

- Your first and only action until the file `{{.ready_marker}}` exists is `lyx shuttle await {{.ready_marker}}`, run with the Bash tool's `timeout` at 300000.
  On `ready: false`, run the same command again in a turn that does nothing else.
  Read nothing, judge nothing and write no note before `ready: true`.
- Give up after the wait has reported `ready: false` 15 times in a row, and return the one-line report "no record to review".
- After `ready: true`, read {{.reviewed_files}} and review them through your lens as an outsider.
- Your context already holds the writer's exploration and the rubric, so never re-read what your context shows.
  Make at most 6 further tool calls, each for something your lens needs that your context lacks.
  End your report with one line per such call, saying what you read and why your context lacked it.
- You are read-only: never write, edit or delete a file, run no git command, and never call the Agent tool.
- Return your findings only as your final message, each with {{.severity_rule}}, the section or decision it concerns, a one-line summary and the evidence.

A fork never calls the Agent tool, so the writer is the only session that spawns.
