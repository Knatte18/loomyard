<!-- This is the twill directive: the host-neutral block that has a writer review its own finished record through lenses, one fork per lens, before it ends.
     internal/twilldirective renders it, and the spawning module hands that renderer every host value, so this block names no file, step, verb or section of any one host.
     It declares these required markers:
     {{.reviewed_files}}, {{.rubric_path}}, {{.lens_list}}, {{.final_self_check_command}}, {{.disposition_place}}, {{.spawn_point}}, {{.governing_input}}, {{.dispute_evidence}} and {{.severity_rule}}.
     It also names two optional markers, {{.writer_fork}} and {{.relaunch_rules}}, each filled by Directive with one part stencil and left empty otherwise. -->

## Lens review by forks

{{.spawn_point}}

{{.writer_fork}}1. Read the rubric at `{{.rubric_path}}`, so every fork you spawn inherits it.
2. Spawn every lens below in one message through the Agent tool with `subagent_type: "fork"`, one fork per lens, and never pass a `name`.
   Each fork's prompt is the fork boilerplate below plus its lens text and nothing else: no design hint, no direction to read code.
3. Wait for every fork to return; ending your turn while forks are outstanding is allowed.
4. Judge every finding: merge duplicates across lenses, then weigh each finding twice: its premise, against the code and {{.governing_input}}, and the change it proposes.
   Give it exactly one of three dispositions, decided in this order; severity decides how a finding is reported, not whether it is weighed.
   - **Dispute**, on one of two grounds.
     `premise false`: the premise is false against what you read.
     `every fix worse`: the premise holds, but every change that addresses it makes the reviewed files worse.
     One sound change means the finding is fixed, not disputed.
     The evidence is cited text opened in this session, {{.dispute_evidence}}, and what it shows against the finding.
     Severity, size, cost, taste, and disagreement with the rubric or `PATTERN` never justify a dispute.
     A finding you cannot refute from what you read is not disputed.
     Keeping a waiver, a deferral or an unplanned test against a finding is a dispute and needs the same evidence.
   - **Fix with your own change**: the premise holds, the change the finding proposes is wrong or harmful, and another change resolves the premise.
     Make that change, and name the proposed change you did not make and why, with the same kinds of evidence.
   - **Fix as proposed**: otherwise.
     A finding that proposes no concrete change leaves the change to you.
5. Record the lens review at `{{.disposition_place}}`: one line per finding, with its lens, its severity and its disposition: `disputed` with its ground and the evidence, `fixed differently` with the proposed change you did not make, why, and what changed instead, or `fixed as proposed` with what changed.
   Name a fork that returned no report.
6. Run `{{.final_self_check_command}}` again until it passes, then end your turn.

One fork pass per writer spawn.
A writer re-prompted by a gate or a parent-review reject fixes the reviewed files itself: if it has not yet made its lens spawn call (step 2), it runs this procedure once; otherwise it spawns no forks.

{{.relaunch_rules}}Lenses:
{{.lens_list}}

### Fork boilerplate

Copy these lines verbatim into every fork's prompt, ahead of its lens text.

- {{.reviewed_files}} were written and passed the self-check before you were spawned.
  Review them through your lens as an outsider, as your context holds them, and never read them again.
- Your context already holds the writer's exploration, the reviewed files and the rubric, so never re-read what your context shows.
  Make at most 6 further tool calls, each for something your lens needs that your context lacks.
  End your report with one line per such call, saying what you read and why your context lacked it.
- You are read-only: never write, edit or delete a file, run no git command, and never call the Agent tool.
- Return your findings only as your final message, each with {{.severity_rule}}, the section or decision it concerns, a one-line summary and the evidence.

A fork never calls the Agent tool, so the writer is the only session that spawns.
