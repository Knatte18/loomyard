// Package orchcli is the CLI for the hub orchestrator: the long-lived session in the hub's prime worktree that runs the task loop for the operator.
// Its verbs (`start`, `status`, `refresh`, `distill`, `withdraw`, `stop`, `cleanup`, `rules` and the watcher behind them) host that session as an interactive agent in a reed strand and cycle its context before it fills; this comment records the loop the session runs, which the verbs exist to serve.
//
// # The role file and the note
//
// The procedure below lives in the role stencil, not in the launch prompt.
// `start` renders it to `.lyx/orch/role.md` and launches the session with a one-line pointer at it; skills (`scribe:prose`, `scribe:conversation`, `ly:board`) are loaded by shuttle before that pointer on every launch path, `--adopt` and `--continue` included.
// `start` records the orch session's id in orch's state, which `lyx orch start --continue` resumes after a reboot or a dead strand; see the internal/orchengine package documentation's `Continuing after a reboot` section.
// A context cycle asks the session for an orch note at the path it names, following `.lyx/orch/note-template.md`, in place of a general handoff skill.
// `lyx orch refresh` requests a clear cycle and `lyx orch distill` a compact cycle, whatever `cycle_mode` says; both write the note first.
// The role file's `## Commands` section holds the operator command index.
// Its `## Hub-wide rules` and `## This orch's rules` sections hold the hub-wide rules file on Bolt and this orch thread's own rules file under its worktree's `_lyx/orch/`, re-read at every delivery.
// `start` and `resume-context` render it in-process from the binary they run as, which the pane names;
// the watcher outlives a deploy, so it runs `help index` from the `lyx` path the orch strand recorded and goes idle with a reason naming `lyx orch stop` when that path is missing or gone.
//
// # The cycle request
//
// `refresh` and `distill` write a request marker the watcher acts on at the session's next idle moment.
// A hub pause holds the watcher's cycle machine, so a request made under a pause waits for the resume, and the timeout that abandons it counts no pause time.
// With a live watcher and a live hub pause entry, the envelope of both verbs adds `held_by_pause: true` and a `note`, and the request is still written; with no live watcher the request is removed and the envelope carries no such key.
// `status` reports the marker as `cycle_request`: null while none is pending, else an object with `mode`, `requested_at` (RFC 3339 UTC, null for a marker that does not parse) and `held_by_pause`.
// A pause record that cannot be read counts as no pause.
// `withdraw` removes a pending request of either mode in any phase and answers `withdrawn: true`.
// With none pending it answers `withdrawn: false` with `phase`, and with `cycle_trigger` only when the phase is neither idle nor resuming, since `toIdle` does not reset the trigger and a reload start does not set it.
// It never refuses: it removes only the marker, never a started cycle's state, and a marker present during a cycle has not started, because a cycle start consumes the marker it starts from under the orch state lock.
//
// # The orch rules
//
// `lyx orch rules --from <file>` replaces the hub-wide rules file on Bolt with the file's content, pulling Bolt first and pushing before and after the commit.
// The verb commits only that one path, and an empty file clears the rules.
// The orch thread's own rules file is committed through `lyx fabric commit`.
//
// # The orchestrator acts and fixes
//
// The orchestrator talks to the operator about what to build, priorities and design choices.
// Everything else is its own: start runs, review PRs, un-wedge stuck runs, approve, land, deploy, clean up, record findings and start the next task.
// Un-wedging includes small code fixes committed and pushed to `main` from the prime, followed by the production deploy.
// Runs that can run in parallel do, and only a task whose packages overlap a running task is held back, through its board `depends_on`.
// A ready PR is "awaiting", never "blocked".
// Findings go on the board, never into GitHub issues.
// Every status report on a running run carries its `lyx reed attach` command.
//
// # The run loop, per task
//
//  1. Create the task worktree with `lyx fabric add <slug>`, then start its run from inside it with `lyx loom start`.
//     The run's status file is `_lyx/shed/<slug>/status.json` under the worktree, and its `state` is one of running, awaiting, blocked, failed or done.
//  2. Keep no watch loop of your own: a batten notice posted as batten's program arrives in full as its own `[creel] #<id> from <shortname>:BATTEN: …` line, one posted under a strand claim as mail announced by creel's reminder that you read, and on either you check the run with `lyx batten status <slug>`.
//  3. At an awaiting Publish, review the PR: a small diff yourself, with build, vet and test over the touched packages; a large one through a read-only background subagent.
//     Then `lyx loom approve` in the worktree; batten resumes the child itself, so no `lyx loom start` follows.
//     Finalize squash-merges, closes the PR and marks the board task done.
//  4. On blocked or failed, read `error` in the status file and rerun the failing tests to tell a flaky failure from a real one.
//     A Publish blocked on a failed verify reaches the orchestrator only on its second failure in the batten run, since batten sends the first back to the record's target and resumes it itself.
//     A real regression in the task is fixed in the task worktree by a background subagent, with code commits only, no `lyx fabric`, no `_lyx` or `.lyx` edits and no push, and the run is started again.
//     A lyx bug is fixed on `main`, deployed, and the run is started again.
//     Never merge the parent into a task while webster is mid-batch, because record-batch then refuses the batch; a merge-in is safe only once every batch is recorded, for example at a blocked integration verify.
//  5. Batten's Land row lands the run: it pulls the prime, runs the hub's landing `deploy` command and prunes the board, and Worktree-Teardown then removes the pair.
//     The orchestrator acts on a Land or teardown failure notice and on a ready-task notice, starting the tasks it names, and deploys by hand with `update-plugins.sh` on a ready notice or at its next wake after an approval while the hub's landing `deploy` key is empty.
//     It reads the drive report only when the run had trouble (friction findings are filed as LoomYard issues, which reach the board through intake when the operator asks for it) and records findings.
//
// # Priorities
//
// In order: fewer and recoverable stops, since a false stop costs as much as a wrong landing and every refusal needs a way forward;
// an automated loop, through batten, the driver's own repairs and the orchestrator's context cycling, by `/compact` or `/clear` as orch.yaml's `cycle_mode` says;
// records that survive teardown; and the operator's surface, meaning panes, the IDE workspace and the launch line.
//
// # Config after a binary change
//
// `start` reconciles the hub's config after a binary change, once per build, before it loads any module config or launches anything; a failure stops it with the reconcile's own message and way forward.
// No other orch verb reconciles.
//
// # The origin reconcile at start
//
// Once `start` holds the start lock, and before it decides anything about the strand, it runs the hub-scope origin reconcile from the prime, so the orch never acts on a stale view of origin.
// The reconcile never refuses the start: a stop, an unreachable origin and a reconcile error are reported for the orch to act on, and a run already driving is reported live or stopped when origin moved its branch.
// The envelope gains `reconcile` (the report), `failed_pushes` (the hub's outstanding failed pushes), and the `mutations` and `partial` pair of a mutating verb, since the reconcile can remove pairs, write snapshot tags and move branches.
// An unreachable origin or a reconcile error is a `warning`.
// A failure after the reconcile ran carries the report's `mutations`, with `partial` true when the record is non-empty.
// A fresh launch's prompt gains a line summarising the report and the outstanding failed pushes, the owned-elsewhere lines of the summary included.
// Under an operator's live hub pause the reconcile's network calls wait at the network gate until the lift.
//
// # The prime claim and the owner pause at start
//
// After the reconcile, `start` first claims the prime for this machine through `claimPrime`, writing only when the board records no prime owner.
// A claim, a claim lost to another machine and a failure are reported under the reconcile warnings, and none refuses the start.
// Before it touches any strand, `start` then pauses each owned_elsewhere entry of the report through `pauseOwned`, which `InstallOwnerPause` stores for the process; the root pre-run installs `tendercli.PauseOwnedElsewhere`, since `tendercli` imports this package and the reverse import would cycle.
// The paused slugs with their owners are the `owner_pauses` envelope key, and a failed or unwired pause is a warning naming that `lyx orch start` pauses it again.
// A nil `claimPrime` or `pauseOwned`, as a hand-built start harness has, is skipped.
//
// # Withdrawing around a fresh launch
//
// Wherever `start` resets the orch state onto a session, a relaunch, `--continue`, `--adopt` and an adopted live strand alike, it signs creel calls as the watcher's program and runs two rounds on the orch strand's name, the listed strand's name or else the fixed `<shortname>:orch`.
// A hub with no shortname record fails `start` before any round when no orch strand is listed.
// The first round runs before `start` starts or binds a session and before it removes the old strand: a withdraw of the old cycle's queued steps, and a release only when an orch strand is listed.
// A failed withdraw, a refusal or an unreachable daemon alike, fails `start` with creel's error and a way forward, and nothing is started, bound or removed.
// Creel's refusals for a program claim from a non-orch pane or for an unknown pane process carry no code, so that way forward is one fixed text naming both causes.
// The second round runs after the reset state is saved, a withdraw and a release that catch a step an old watcher still running queued in between.
// A failed first release or a failure in the second round does not fail `start`; the error text is recorded in the orch state's `fresh_launch_failure`, which the watcher retries on every tick.
// A resume line a withdraw removed is handed to `orchengine.ClearResumeNoticed` with the orch GUID the state records at that call, so a continued or adopted session gets the line posted again.
//
// # The session-start hook
//
// The orch run's spec is the one that installs the provider's context-after-compaction hook.
// Its command is built through the POSIX shell dialect, which the provider runs hooks under on every OS: a change of directory to the prime's anchor, then the bare hidden verb `lyx orch resume-context`.
// The verb therefore runs in the prime whatever directory the session has moved to, and its own prime refusal bounds a failed change of directory.
// It prints the resume pointer as the provider's additional-context JSON and nothing else, and writes the delivery mark the watcher reads.
// A failure prints the JSON error envelope and writes no mark, so the watcher types the pointer itself.
//
// # Reaching the orch from another module
//
// `PrimePaths` is the one accessor other modules use for the orch's told paths, so the orch state is reached without re-deriving it.
// A module sends the orch a notice as creel mail and never types into the orch session itself.
// `NotifyPrime` is the way a task-worktree module does that: it takes the task worktree's location, the module's program name and a line, resolves the prime itself, and posts the line to the prime's orch strand through creel.
// It reports whether the line was posted, and a prime with no orch strand recorded posts nothing and is no error.
// `PrimeOrchStrand` answers the name of that strand: the name reed's state holds for the recorded GUID, or the fixed `<shortname>:orch` name for a GUID reed no longer holds, which creel then accepts or refuses.
// `InstallPushNotifier` routes fabric's failed-push notice to the prime orch as mail signed `FABRIC`, for every lyx process from the root pre-run.
// It resolves the hub from the working directory as the network gate does, so a detached push child starting in the board worktree is covered, and installs nothing outside a hub.
package orchcli
