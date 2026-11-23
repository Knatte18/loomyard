// Package orchcli is the CLI for the hub orchestrator:
// the long-lived session in the hub's prime worktree, or in any beam, that runs the task loop for the operator.
// `BeamUp` and `BeamDown` bring that session up and take it down.
// Its verbs host it as an interactive agent in a reed strand and cycle its context before it fills:
// `status`, `refresh`, `distill`, `withdraw`, `stop`, `rules` and the watcher behind them.
// The orch's procedure lives in its role text (contracts/orchrole), not in this package or the launch prompt.
//
// # The orch in a beam
//
// Every verb runs in the hub's prime worktree and in a beam; every other worktree refuses it.
// There is one orch per worktree, with its own files, strand and watcher.
// `BeamUp` refuses a worktree another machine owns, a worktree with an anchor change standing on it or on the prime,
// and a beam whose board entry a run other than the beam's own landing holds.
// Each refusal names its way forward.
// A beam held by its own pawl run's landing is no refusal: the orch starts alone, with a warning.
// A dobby, relay or runs strand that fails to start is a warning, never a stop.
// `BeamUpWorktree` brings up a worktree's dobby and its orch for a verb that needs the dobby live.
// `BeamUpOptions.SkipOrch` leaves the beam with its dobby and no orch.
// `BeamDown` removes the orch strand, ends the worktree's dobby, then the orch session's registered provider process.
// Each end goes through `runproc.EndRole`, so no other process of the beam is signalled.
// A step's failure does not skip the later steps, and every failure is returned.
// `OrchLive` is the liveness predicate other modules call.
//
// # The role file and the note
//
// `BeamUp` records the orch session's id in orch's state and resumes it after a reboot, a `stop` or a dead strand.
// When this machine holds no transcript of the session it launches fresh from the last note, else the start stencil;
// the internal/orchengine package documentation's `Continuing after a reboot` section has the detail.
// A context cycle asks the session for an orch note, following `.lym/orch/note-template.md`.
// `lym orch refresh` requests a clear cycle and `lym orch distill` a compact cycle, whatever `cycle_mode` says.
//
// # The orch's provider process
//
// After each launch `BeamUp` registers the session's provider process in `internal/runproc` as `runproc.RoleOrch`.
// A resume the provider refuses because a live process holds the session ends the holder when the record names it.
// A holder lym has not registered refuses the start, naming its pid.
// A resume refused because this machine holds no transcript launches fresh, and the report's warnings say so.
// Any other failed resume fails the start, which launches nothing fresh.
//
// # The cycle request
//
// `refresh` and `distill` write a request marker the watcher acts on at the session's next idle moment.
// A hub pause holds the cycle machine, so a request made under a pause waits for the resume.
// `withdraw` removes a pending request of either mode in any phase and never refuses;
// it removes only the marker, never a started cycle's state.
//
// # The watcher leaves a beam it no longer owns
//
// A beam another machine owns loses its orch strand and its processes on this machine.
// While a hold this machine wrote covers the worktree's subject, a foreign owner leaves the strand up.
// The watcher ends every process this machine runs for the subject, except the dobby.
// It removes the strand even when that end failed; a failing end never keeps the orch up.
//
// # Status restarts a dead watcher
//
// `status` is what the orch and the operator run when the orch goes quiet,
// so a live strand with no watcher gets a new watcher from it.
// A dead strand gets none: `BeamUp` relaunches strand and watcher together.
// `watch.lock` keeps one watcher, and `stop` and the restart serialize on `start.lock`,
// so a restart never binds a watcher to a strand `stop` removes.
//
// # The orch rules
//
// A repository's orch rules are the heddle entry `orch-rules`, aimed at heddle's `orch-prompt` target.
// `lym orch rules --from <file>` replaces that entry's text with the file's content; an empty file clears the rules.
//
// # Config after a binary change
//
// No orch verb reconciles the hub's config after a binary change;
// `lym beam start` does, through `hubreconcile.Ensure`, before it calls `BeamUp`.
//
// # The origin reconcile at bring-up
//
// With `BeamUpOptions.Prepare`, `BeamUp` runs the hub-scope origin reconcile from the prime.
// It runs it once it holds the start lock and before it decides anything about the strand.
// So the orch never acts on a stale view of origin.
// A caller that already judged origin leaves `Prepare` false; the refusals still run.
// The reconcile never refuses the bring-up: a stop, an unreachable origin and a reconcile error are reported.
// A failure after the reconcile ran carries the report's mutations, with `Partial` true when the record is non-empty.
//
// # The prime claim and the end of owned runs at bring-up
//
// After the reconcile, `Prepare` claims the prime for this machine when the board records no prime owner.
// Before it touches any strand, it ends the processes of each owned_elsewhere entry, its dobby too for a beam.
// A subject read as owned elsewhere therefore leaves no process of its runs, its orch or its dobby on this machine.
// A failed claim or end is a warning and never refuses the bring-up.
//
// # Withdrawing around a fresh launch
//
// Wherever a bring-up resets the orch state onto a session, it signs creel calls as the watcher's program
// and runs two rounds of withdraw and release on the orch strand.
// The first round runs before the bring-up starts or binds a session and before it removes the old strand.
// Its failure, a refusal or an unreachable daemon alike, fails the bring-up with nothing started, bound or removed.
// The second round runs after the reset state is saved, and catches a step an old watcher queued in between.
// Its failure does not fail the bring-up;
// the error text is recorded in the orch state's `fresh_launch_failure`, which the watcher retries on every tick.
//
// # The session-start hook
//
// The orch run's spec installs the provider's context-after-compaction hook.
// The hook runs the hidden verb `lym orch resume-context` from the acting orch's own anchor.
// The verb prints the resume pointer as the provider's additional-context JSON and writes the delivery mark.
// On failure it writes no mark, so the watcher types the pointer itself.
//
// # Reaching the orch from another module
//
// A module sends the orch a notice as creel mail and never types into the orch session itself.
// `PrimePaths` is the one accessor for the orch's told paths.
// `NotifyPrime` posts a line to the prime's orch strand and posts nothing, with no error, when none is recorded.
// `NotifyParent` and `NotifyFrom` route a run's notice to the nearest live orch up its beam chain.
// They route to the prime's orch when none is live.
// `NotifyAsker` routes a hold notice to the orch the held agent's own question reached.
// `PostAsProgram` signs a post as a lym program alone.
// `PostAsCaller` signs as the calling agent's own strand when that strand is not an orch.
// Creel lets only an orch strand's pane claim a program name.
// `HubOrchs` lists every worktree an orch may run in, skipping a pair still being created.
// `InstallPushNotifier`, `InstallReconcileNotifier` and `InstallRunMailDropper` wire notices to those routes.
// They serve fabric, the hub config reconcile and pairteardown; the root pre-run installs them for every lym process.
package orchcli
