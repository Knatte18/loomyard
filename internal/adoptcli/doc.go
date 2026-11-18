// Package adoptcli is the adopt module.
// Its verbs are `lym adopt <beam>...`, `lym adopt root` and `lym adopt --all`.
// It also has `lym adopt --here` and `lym adopt --archives <subject>...`.
// They take the flags `--take-hold`, `--no-orch`, `--here` and `--archives`.
// It takes over beam projects, or root, and the runs under them from the other machine of the operator.
// It calls each owning module's function in-process, so no module learns another's state,
// and it has no engine package of its own.
//
// # The verb
//
// The verb runs from the hub's prime and from any beam, and acts on the prime's location in both;
// `battencli.ResolveBattenPrime` resolves the prime.
// `runAdopt` runs the whole adopt in the caller's process, over tracked state, and prints one envelope at its end.
// Nothing is detached and no adopt record is written.
// A re-run of the same subjects goes on from what Bolt and the run records on origin hold.
// `request` holds the slugs, `Root`, `All`, `TakeHold` and `NoOrch`;
// the word `root` among the slugs sets `Root`, and an entry named `root` cannot be named.
// `adoptDeps` holds every step's seam, which `hubDepsAs` fills over the hub and `TestAdopt` fakes.
//
// The arguments are beam projects (a project entry whose `beam` is true) and `root`, which names the prime.
// `--all` names `root` and every beam project that is not done.
// Only a beam project and the prime record an owner, so an adopt moves the named projects alone:
// each one's owner (the prime owner for `root`), its pair, and the runs whose parent is its worktree.
// A nested beam keeps its own owner.
// `lym adopt root` moves root's tasks and the tasks of the grouping projects below root.
// It never moves a beam's members, whose parent is that beam.
//
// `validateRequest` reads the local Bolt through `namedEntries` and refuses what cannot be adopted.
// An unknown slug is refused naming `lym board list`, and a note naming `lym board promote`.
// A done entry is refused naming that it already landed.
// A task is refused naming `lym adopt <beam>` or `lym adopt root`, the `AdoptSubject` that moves it.
// A grouping project is refused naming the same adopt, which moves its tasks, and `lym beam create <slug>`.
// No argument without `--all` is refused, and so is the take-hold flag with `--all`, since a hold names its subjects.
// An anchor change standing under the subjects, or under the runs this machine holds, is refused before any write.
// The call sets `NoOrch` when the caller's strand name, `agentname.StrandNameEnv`, parses as an orch role.
//
// # The steps
//
// `runForeground` runs the steps in order and prints `adopt: <step> done` on stderr as each ends.
// Before each of the steps after the first it pulls Bolt and reads the hold again through `holdStands`.
// A hold another machine took, or one that cleared, ends the adopt.
// The error names the machine that owns the subjects.
// It says the adopt moved or cleared, so nothing more runs here.
// No dobby starts.
//
//  1. `hold`: `boardengine.WriteAdopt` writes the hold, the owner of each subject and this machine's registry entry
//     in one pull-first Bolt commit, pushed before it returns.
//     The hold names each subject's prior owner.
//     This machine's hold over the same subjects stands as it is, its kind, fix task and try counts included.
//     A hold over other subjects, or another machine's hold, is refused with its own way forward.
//     An unreachable origin is refused naming the re-run once origin answers.
//  2. `pairs`: each adopted beam this machine has no pair for gets it from origin.
//  3. `takeover`: `takeOverRuns` takes over each open run under the subjects that `movedRun` judges moved.
//     A run's pair is added from origin first when this machine lacks it,
//     and `fabricengine.TakeOverRun` then fetches, archives, resets the pair onto origin and pushes the takeover entry.
//     A beam under an adopted root that keeps its own owner on another machine is reported as owned elsewhere and left.
//  4. `catchup`: the catch-up over the whole hub, which pushes what this machine owes and hands over what it lost.
//     A failing catch-up ends the adopt, since the deploy and the check read origin's state.
//  5. `deploy`: when root is a subject and the landing config deploys into the prime's branch,
//     the hub's landing deploy runs if the prime's head is ahead of the running lym's build.
//     A failed deploy ends the adopt with the hold standing, so no run goes on under the old lym.
//  6. `workspace`: the hub's workspace files are regenerated; a failure is a warning naming `lym fabric reconcile`.
//  7. `check`: the hub's `adopt` gate runs in each subject's worktree, with its output on the terminal.
//  8. `beam`: each subject is brought up, the prime's dobby first when root is a subject.
//     `--no-orch` leaves each orch down, and a beam whose pawl run holds the landing hold gets none.
//
// # Which runs move
//
// `movedRuns` lists the open runs under the subjects by their record on the parent's records branch on origin:
// an open task or beam project whose record is not done, whose parent is an adopted beam, or root when root is adopted.
// The pawl run of each adopted beam is listed with it.
// `movedRun` judges each by the takeover entry its record holds and by the hold:
//
//   - an entry naming this machine means an earlier pass took the run, so it is not moved;
//   - an entry naming another machine moves it, the fix task's run included, so a take-hold back moves it again;
//   - with no entry, the fix task of an adopt-fix hold that this machine filed is not moved;
//   - with no entry, any other run is moved when its subject's prior owner in the hold is another machine.
//
// The hold keeps the prior owner across a re-run.
// An adopt killed after the first step still moves the runs it had not taken and leaves the ones it had.
// A run that is not moved needs nothing here, and the catch-up pushes its owed commits.
//
// # The hold
//
// The hold is the Bolt record that stops every other machine's adopt and every run's start while this one runs.
// It stands from the `hold` step until a green check clears it with the bolt-hold pause entries.
// Only `boardengine`'s setters write it: `WriteAdopt`, `FixHold` and `ClearHold`.
// A failing check files a fix task.
// It turns the hold into an adopt-fix hold that names the task, its filer and the try count of each subject.
// The fix task's landing runs `lym adopt check` over the hold again, so a green check clears it.
//
// `--take-hold` moves another machine's hold over the same set of subjects to this machine.
// It skips no check and lets no run go on before a green check.
// A shell-guard row denies it to agents.
// The refusal a plain adopt returns over another machine's hold names waiting or asking the operator, never the flag.
//
// # The here call
//
// `lym adopt --here` takes no subject, `root` or `--all`.
// Its subject is the worktree the call runs in: `root` in the prime, the beam in a beam.
// `hereRequest` pulls Bolt once and reads the subject's owner.
// A subject no other machine owns ends `owned_here` with no write and nothing started.
// A pull that cannot reach origin reads the local Bolt and adds a warning to the end.
// A worktree that is neither the prime nor an open beam project ends `no_subject`.
// Any other subject runs the adopt of that subject alone.
//
// # The archives list
//
// `lym adopt --archives <subject>...` lists the old owner's `archive-owner/` tags.
// It lists a tag only when its commit is missing from the slug's branch on origin.
// It takes beam projects and `root`, refuses every other flag, runs no step and writes no hold.
// The end envelope of an adopt whose owner write stood names that call.
//
// # The check
//
// The hidden `lym adopt check <subject>` runs `runHoldCheck` over the subjects of this machine's hold, first first.
// The adopt's `check` step runs the same function in this process.
// A hold of this machine that stands is the only thing it checks for:
// with none, or with another machine's hold, it checks nothing and ends `no_hold`.
// It exits 0 once the check returned a result, and prints the state, the fix task and the lines it posted.
//
// `runCheck` runs the hub's `adopt` gate in-process in a subject's worktree through `gatecli.RunGate`.
// It takes `check.lock` of the subject without blocking, so one check of a subject runs at a time,
// and it reconciles the hub config first, so no check goes green on the template's tiers.
// It writes the progress to the subject's `check.json` and posts each tier to the subject's nearest live orch.
// It also sets the check mark on the prime's reed session, which `lym beam status` shows beside the record.
// A subject another machine owns posts that and runs no tier.
// Each subject's files sit under `fabricengine.AdoptScratchDir`: `check.json`, `check.lock`, `check.out` and `suite.log`.
//
// The end of a check is settled over the whole hold:
//
//   - with every subject green, Bolt is pulled and the hold read again; a hold another machine took meanwhile is left,
//     and nothing is brought up.
//     Otherwise each subject's fix task is removed, the hold is cleared and the bolt-hold entries are lifted;
//   - with a subject failed, `failedEnd` runs the fix loop of the first failing subject in the hold's order.
//
// `failedEnd` takes the first case that applies:
//
//  1. The subject's own fix task stands open and healthy: nothing is filed, and the notice names the task.
//  2. Another open front task stands in the subject's worktree: nothing is filed, and the notice names it.
//  3. The try count in the hold is at `adopt_fix_tries`: nothing is filed, and the notice names the failing tests,
//     the standing task and the hold's re-run.
//  4. The own fix task is a draft or halted: it is removed through `fronttask.Remove` right before the filing.
//  5. A task is filed through `fronttask.File` with the recipe `darn`, the label `bug` and the action `adopt-check`.
//     The try count is written into the hold between the entry's creation and its release.
//
// `failedEnd` never starts a pass of the task: its parent's dobby starts it from the empty status.
// Bound: at most `adopt_fix_tries` fix tasks per subject between two hand adopts, and at most one at a time.
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
// Whether origin holds an entry's branches is read from the board, not from origin.
// A run a machine took over is owned only once its takeover entry reached origin.
// A push cut short leaves it with its old owner until the re-run.
package adoptcli
