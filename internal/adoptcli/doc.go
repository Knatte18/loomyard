// Package adoptcli is the adopt module.
// Its verbs are `lym adopt <beam>...`, `lym adopt root`, `lym adopt --all` and `lym adopt --here`.
// It also has `lym adopt --archives <subject>...`, with the flags `--take-hold` and `--no-orch`.
// It takes over beam projects, or root, and the runs under them from the other machine of the operator.
// It calls each owning module's function in-process, so no module learns another's state,
// and it has no engine package of its own.
//
// # The verb
//
// The verb runs from the hub's prime and from any beam, and acts on the prime's location in both.
// It runs the whole adopt in the caller's process, over tracked state, and prints one envelope at its end.
// Nothing is detached, and a re-run goes on from what Bolt and the run records on origin hold.
//
// The subjects are beam projects and `root`, which names the prime; `--all` names `root` and every beam not done.
// An adopt moves the named subjects alone: each one's owner, its pair, and the runs whose parent is its worktree.
// `lym adopt root` also moves the tasks of the grouping projects below root, and never a beam's members.
// A refusal names the verb that does the job instead.
// An anchor change standing under the subjects, or under the runs this machine holds, is refused before any write.
//
// The steps run in this order: hold, pairs, takeover, catchup, deploy, workspace, check, beam.
// A failing catch-up or deploy ends the adopt with the hold standing.
// `--no-orch` leaves each orch down, and the check runs in each subject's worktree.
// Each step after the first pulls Bolt and reads the hold again.
// A hold another machine took, or one that cleared, ends the adopt and no dobby starts.
// An adopt killed part-way still moves the runs it had not taken and leaves the ones it had.
//
// # The hold
//
// The hold is the Bolt record that stops every other machine's adopt and every run's start while this one runs.
// It stands until a green check clears it; a failing check turns it into an adopt-fix hold that names a fix task.
// Only `boardengine`'s setters write it.
//
// `--take-hold` moves another machine's hold over the same subjects to this machine.
// It skips no check and lets no run go on before a green check.
// A shell-guard row denies it to agents.
// The refusal a plain adopt returns over another machine's hold never names the flag.
//
// # The here call
//
// `lym adopt --here` takes no subject, `root` or `--all`; its subject is the worktree the call runs in.
// A subject no other machine owns ends `owned_here` with no write and nothing started.
// A worktree that is neither the prime nor an open beam project ends `no_subject`.
//
// # The archives list
//
// `lym adopt --archives <subject>...` lists the old owner's `archive-owner/` tags whose commit is missing on origin.
// It refuses every other flag, runs no step and writes no hold.
//
// # The check
//
// The hidden `lym adopt check <subject>` runs the hub's `adopt` gate in each subject's worktree.
// The adopt's `check` step does the same.
// With no hold of this machine standing, it ends `no_hold`.
// All subjects green clears the hold; a failed subject files a fix task, bounded by `beam.adopt_fix_tries` per subject.
//
// # The envelope
//
// The end envelope is ok for an adopt whose check went green and an error envelope for any other end.
// Either way it holds what the adopt did.
// `slugs` and `prime_recorded` are always present.
// `hold` is the hold while one stands, and `fix_task` the task the check filed or found standing.
// `prime_check` is the check state of root, and `check_lines` the lines the check posted.
// `deploy` reports a deploy the call ran, and `warnings` a deploy state that did not read and a workspace failure.
// `prime_beam_steps` and `prime_beam_warning` report the prime's beam steps.
// `archives` names `lym adopt --archives` over the subjects whose owner write stood.
// Per slug it carries a pair added and an owner recorded.
// It carries the run as taken over, none or owned elsewhere by `owned_by`.
// It also carries the subject's `check` state, the `beam_steps` run and the error.
// A beam held by its landing carries the way to follow the landing and resume it.
//
// # The bounds
//
// Adopt stops no driver live elsewhere; such a driver pauses at its next owner check.
// A run a machine took over is owned only once its takeover entry reached origin;
// a push cut short leaves it with its old owner until the re-run.
package adoptcli
