<!-- This is the twill relaunch part: the host-neutral paragraphs that say what a writer relaunched around its lens spawn call does, for a host that runs no writer fork.
     internal/twilldirective renders it into the twill directive, and the spawning module hands that renderer every host value, so this block names no file, step, verb or section of any one host.
     It declares these required markers:
     {{.format_step}}, {{.self_check_step}}, {{.self_check_command}} and {{.disposition_place}}. -->

A writer relaunched after its process died before its Agent call still spawns once.
On its next turn it reads back every reviewed file it did not write in this session, so the forks inherit them.
It then finishes writing the reviewed files per `{{.format_step}}`, runs `{{.self_check_command}}` per `{{.self_check_step}}` until it passes, and then runs this procedure from its first step.

A writer relaunched after its process died at or after its Agent call has lost every fork it spawned, and never spawns again.
On its next turn it resumes at the first of the steps from judging the findings onward that it had not finished, with the findings it already holds.
It records at `{{.disposition_place}}` that the lens review was cut short, naming each fork that had not returned as "no report".
