// Package loomcli is the `lyx loom` command: it bootstraps and drives one task's phase machine per worktree on the generic shed engine, and carries the verbs through which the operator and the run's parent answer its gates.
//
// The run, step, status, pause and goto bodies are the shared `internal/shedverbs` verbs, mounted here with loom's own text.
// Friction-Reflect, the last row, runs the Tier 2 friction reflection under run and under step alike, before the run records done.
// `lyx start` is the same start verb mounted a second time at the root, resolving the full engine stack exactly as `lyx loom start` does.
//
// # Start
//
// `lyx loom start` works in order.
// It resolves the recorded parent branch, seeds the status file when absent and commits the seed into the fabric before anything else touches it.
// It brings up the worktree's tmux session;
// it ensures the status strand on either driver, and the per-hub watchdog daemon is spawned best-effort.
// The status strand runs the hidden `lyx loom checklist` verb: a top content strand at an eighth share that shows the run's state line, with the note of what a running run waits on (the same note `lyx loom status` reports), over one line per recipe row, with its box, the time spent in it and its visit count.
// The verb reads the status file and the seed without arming a run, and reports its line count to reed so the strand is sized to it.
// Unless a driver is already alive, it spawns the driver the seed records, which is never chosen by a flag;
// a second invocation while a driver runs ensures the substrate instead.
//
// A live loom driver parked at a hand-back is resumed by typing one line into its pane, and start refuses after a bounded wait when that pane is not ready, since the driver may be busy having resumed on its own.
// A pane that holds a draft, an active operator or another busy reading is refused with the retryable kind `driver_busy`, the park marker kept, and the message names the draft and the way forward;
// a resume line that never appeared stays a non-retryable refusal.
// The stop-report path in the resume line is picked once per park and kept in a resume-report file beside the park marker, keyed to the marker's content, so every rerun against one park types the identical line;
// a landed resume or a fresh spawn removes the file.
// A live loom driver over a run halted at a hand-back (awaiting, blocked, paused or failed) that has not yet written its park marker is still writing its stop report, so start refuses with the kind `driver_not_parked`;
// a live driver over a running run is left working.
// Before spawning or resuming a driver, start refuses with the kind `merge_in_progress` over an unfinished merge, naming the conflicted paths and the remedy, the fabric verbs for a fabric merge and git for one fabric did not start.
// The exception is a run whose current producer is Merge-In or Finalize, which goes through over a parked fabric merge-in of its own parent branch, since that row aborts and redoes it.
// A loom driver strand reed marked retiring is never adopted: start removes it and spawns a fresh driver.
//
// start returns once the driver's readiness signal confirms it is up, and for a parked loom driver once delivery of the resume line is verified.
// For the Go driver the signal is the run lock being taken;
// for a loom driver it is the provider TUI coming up ready within shuttle's `startup_timeout_s`, with any one-time startup gate its provider requires dismissed along the way.
// A readiness refusal removes the driver strand, so the next start spawns a fresh one.
// The signal is checked only for a driver this invocation spawns, so a strand left live because shuttle got no liveness answer from reed, or could not remove the strand, is returned over by a later start without re-checking.
//
// The detached Go driver writes its stdout and stderr to the driver log in the ephemeral tree, never to start's own output;
// a loom driver strand writes no such log, since its pane holds its output.
// A loom driver strand launches from the driver stencil, read from the stencils directory at start time.
//
// A worktree opened through `lyx ide spawn`'s generated VS Code task runs `lyx reed up`, `lyx reed add --if-absent --cmd claude --name claude --focus` and `lyx reed attach`, so the operator's own session is the strand named `claude` and the run's panes are its siblings.
// To self-check, compare `$TMUX_PANE` against the tracked strands `lyx reed status` reports: tracked is fine;
// set but untracked means relaunching through that chain or proceeding without reed supervision;
// unset is unconfirmed rather than failed, since psmux on Windows may not export it.
// A worktree whose `.vscode/tasks.json` predates this convention is upgraded by deleting that file and re-running `lyx ide spawn`.
//
// # Resume
//
// `lyx loom resume` takes the bootstrap lock start takes, so the two never run at once, and decides by the run's state:
//
//   - no status file: refused, naming `lyx loom start`.
//   - done: a no-op success.
//   - awaiting at a review segment's Bouncer row with a circling decision recorded: handled as a halted run, so a live, parked driver is woken and its step acts on the decision.
//   - awaiting at a Bouncer row with no decision: refused, naming the circling verbs and then resume.
//   - awaiting at a Bouncer row with an unreadable decision: refused naming the file, to fix it or delete it and record the decision again.
//   - awaiting at any other row: refused, naming `lyx loom approve` or `lyx loom reject`, after which a batten run watching the child resumes it, or `lyx batten run <slug>` from the prime when none does.
//   - running: a no-op success when the driver is live or holds the run lock, and refused as a halted run otherwise.
//   - halted with the run lock held, or with a live driver and no park marker: refused with the kind `driver_not_parked`, to retry shortly.
//   - halted with a live driver and a park marker: resumed, refused with `merge_in_progress` over an unfinished merge under start's Publish and Finalize exception, and with `driver_busy` when the driver's pane would not take the resume line.
//   - halted with no live driver: refused over an unfinished merge with `merge_in_progress`, otherwise naming `lyx batten run <slug>` from the prime and then resume for a dead strand, and `lyx loom start` for a retiring strand or none.
//
// The driver strand is read from reed's directory, which still lists a session that died with the machine, never from the run's seed.
// A resume line that cannot be delivered is refused, naming `lyx loom status` and a re-run of resume.
//
// # Plan card verbs
//
// `lyx loom insert-card <position> <slug> --intent <intent>`, `lyx loom remove-card <position>` and `lyx loom restore-renumber` are the sanctioned way to add or drop one card of the plan and renumber the cards after it.
// A renumber moves each later card's file name prefix, heading number, explicit `Commit:` prefix and Card Index number, and writes a heading-only stub for an inserted card, which the agent fills with Edit or Write.
// The success envelope carries `plan_dir`, `moved`, `created` or `removed`, and `prose_mentions`, the lines that name a moved card by its old number; they are never rewritten, so the agent checks each by hand.
// `restore-renumber` reports `plan_dir`, `journal`, `restored` and `removed`.
//
// Each verb takes webster's state-mutation lease first and releases it before it prints, so no begin-batch records a batch between the bound check and the renumber.
// `insert-card` and `remove-card` refuse a position at or before the last card of any batch webster has begun, terminal or not, since a begun card is frozen.
// They touch only the current generation's card files and the Card Index of the overview, never an archive, a prior generation, the overview's frontmatter or any other section, and they never commit, approve or set `approved:`.
//
// Every call writes a journal under the plan's never-tracked mirror before it changes the plan directory, and removes it as its last step.
// A failure undoes the call; one whose undo fails keeps the journal, and `restore-renumber` completes the undo from it.
// The restore writes back whole pre-call files, the overview included, and removes every created name whatever it holds by then, so it discards any edit made to the journaled files since the interrupted call.
// A journal naming anything but the overview or a card file at the top level of the plan directory is refused before any file is written.
//
// The three verbs read only the resolved anchor, so they join the lightweight wiring and are not held up by an unrelated module's config.
// `renumberRefusal` words every refusal with its way forward; the rows are in `contracts/specs/refusal-spec.md`.
//
// # Validate plan overlay
//
// `lyx loom validate-plan --overlay` checks the plan with proposed plan file contents laid over it, in memory, so a fix can be checked before any file is edited.
// Stdin is one JSON object whose keys are plan file names and whose values are their full proposed content;
// the names are `00-overview.md` and the card files the Card Index on disk lists.
// The verb builds a `planparser.Overlay` over the plan directory, parses it, and runs the flag-less check set (`loomshed.ValidatePlan`) over the overlaid plan, reporting through the same envelope.
// Handle canonicalization and the done-card comparison read and rewrite through the overlay, and no byte under the plan directory is written.
// The overlay cannot change the tree the glyphs resolve against, so it validates target-list, verify-command and card-text changes, never a proposed code change, and it adds, removes or renames no plan file.
// It is exclusive with `--rework` and `--require-approved`, refused before stdin is read; a terminal or empty stdin, input that is not one object of distinct names and string values, an unusable name and a card file missing from disk are each refused with their way forward in `overlayRefusal`, and the rows are in `contracts/specs/refusal-spec.md`.
//
// # Arbiter
//
// `lyx loom arbiter <run-id> --envelope <path> [--report <path>]` is run only by the run's driver, at a stop it cannot settle.
// It spawns a cold arbiter agent on the `arbiter` model through shuttle, waits for its verdict file and prints the validated verdict.
// It is a role verb, so the orch's command index leaves it out.
//
// A stop is eligible only if the verb's closed list admits it: an error stop of kind `producer` or `bootstrap`, an `interrupted` stop under the `reinvoke` policy or with a transient class, a halted `blocked` stop other than a Publish halt that names a target, and a halted `awaiting` stop whose Bouncer row has a pending escalation.
// Every other stop, a missing status file, an unreadable envelope, `arbiter_max_spawns` of 0 and a spent budget are refused with the hand-back to the run's parent as the way forward.
//
// Each spawn is an attempt, numbered from 1 under the run's arbiter directory in `attempt-<N>`.
// `attempt.json` is written before the spawn, so the attempt counts against `arbiter_max_spawns` whatever happens next, and the budget is counted from those records so a restart never resets it.
// The directory also holds `prompt.md`, the agent's `verdict.yaml`, the verb's validated `envelope.json` and, once the re-launch consumed a `resume`, the `applied` mark.
// While an arbiter may be live an ephemeral marker names the attempt, its strand and its deadline.
// The status commit stages the arbiter directory with the run records.
//
// The stop key is the status file's current producer, history length and state.
// A re-run for a stop key an attempt already arbitrated spawns nothing:
// it returns that attempt's verdict, waits on its live arbiter for the time left to its deadline, or stops an arbiter past its deadline through shuttle's stop verb and reads `hand-on` with cause `timeout`.
// An applied `resume` whose stop key is unchanged is refused, since the run stopped at the same place again.
//
// The printed envelope carries `verdict`, `reason`, `question`, `goto`, `verbs`, `cause`, `attempt`, `attempt_dir` and `verdict_path`, and for a `resume` the `relaunch` line.
// A `resume` the verb finds unsound reads `hand-on` with cause `precondition`: no circling decision pending at an escalation, no goto target at a `blocked` stop, or a goto target the run's current row does not admit.
// The verb moves no run itself;
// `lyx shed step <run-id> --until-stop --arbiter-attempt <N>` performs the verdict's goto under the run and status locks, once per attempt.
// A pair's session end ends a live arbiter strand with the pair's other strands.
//
// The Go driver calls this same path in-process, so eligibility, `arbiter_max_spawns`, the attempts, the applied mark and the deadlines are shared;
// it passes the tackler's case brief, which the arbiter's prompt asks it to check against the facts.
//
// # Go driver
//
// `lyx loom drive` is the hidden role verb that drives a go-seeded run, spawned by `lyx loom start`, by `lyx loom resume` for a halted go run and by a hand-over.
// It takes the run's driver lock without blocking and holds it for its whole life, so a settling driver reads as live while the run lock is free between steps, and a second driver refuses at once.
// On start it removes the park marker and the resume report.
//
// The loop steps the run through the shared `--until-stop` body, saves each stop envelope under the run's drive-reports directory and decides the stop by a closed rule table over the envelope's fields and the facts it reads from disk, never over a row name or the reason text.
// The table settles a done run, a busy exit, a re-step of an interrupted step under the `reinvoke` policy up to a cap of two per stop key, and the one verify goto to the Publish failure record's target;
// it asks the tackler at a stop no rule settles, the arbiter at an awaiting stop with a pending escalation, after a tackler `escalate` and when an applied tackler `resume` returns to the same stop, and hands over at a binary-eligible stop whose last step ran another build than the `lyx` on disk.
// Everything else hands back.
// Every action appends a settle record to the friction directory, so the repair cap, the one verify retry and the one hand-over per stop key and build are counted from disk and a restarted driver keeps every cap.
//
// Before it acts on a stop the driver re-reads the status.
// A running status or a run lock another process holds ends the driver with a `running-exit` record;
// a status at another stop key is a moved run, which an orch goto sends to a re-step and anything else decides afresh, and which is never parked and gets no friction note.
// While a pause covers the slug the driver holds in the pause wait before any re-step, goto, agent spawn or hand-over.
//
// A hand-back leaves a Go-authored stop report, commits the run records and writes the park marker through the code `lyx loom commit-records --park` runs, and queues the parent messages the LLM driver's notify rules send.
// A watching batten announces the run, so only the parent notice, a question, a friction-failed done and a watched verify halt are sent; an unwatched run also gets one line naming the run-id and the report.
// A paused stop that carries no reason also writes a `go-driver-paused` friction note.
// A done run commits its records, and one that finished with `friction: failed` also gets a report and a notice without being parked.
//
// A hand-over runs only at a binary-eligible stop, once per stop key and build, over a clean tree outside the run's records and never under a pause.
// It records the hand-over, leaves the stop report and the park marker, releases the driver lock and spawns a fresh `lyx loom drive` of the on-disk build, which re-steps.
//
// A driver restarted over a halted run finds an agent attempt that no later hand-back closed and lets the agent's own call reuse it, so no second agent is spawned.
//
// # Tackler
//
// The tackler is the cold agent the Go driver asks at a stop no rule settles.
// Each call is an attempt under the run's tackler directory in `attempt-<N>`, and `attempt.json` is written before the spawn, so the attempt counts against `tackler_max_spawns` whatever happens next; 0 turns the tackler off.
// The record holds the stop key, the pause conditions, a snapshot of the task worktree, the spawn time, the deadline and the envelope and report paths.
// The directory also holds `prompt.md`, the agent's `verdict.yaml`, an escalate's `case-brief.md`, the call's validated `envelope.json` and, once a re-launch consumed a `resume`, the `applied` mark.
// While a tackler may be live an ephemeral marker names the attempt, its strand and its deadline.
//
// The tackler's verdict is `resume`, `hand-back` or `escalate`.
// A `resume` names a goto only to the run's current row or to the row the halting entry was routed from, and a blocked stop needs one;
// a `hand-back` needs a question for the run's parent, and an `escalate` needs a non-empty case brief.
// A verdict that is missing, unparseable, unsound or late reads as a hand-back whose cause is `missing`, `unparseable`, `precondition`, `timeout`, `died`, `question` or `case-brief`.
// A deadline that ran out while the hub's pause covered the run moves out by the paused time instead.
//
// A re-run for a stop key an attempt already judged spawns nothing: it returns that attempt's verdict or waits on its live tackler, unless a hand-back record for the key closed it.
// An applied `resume` whose stop key is unchanged goes to the arbiter.
// The driver consumes a `resume` through the shed's re-launch, which refuses under the status lock unless the run is still where the tackler found it, and marks the attempt applied in the same hold.
//
// Tampering is the spawn snapshot of the worktree and a fresh one differing, read only after the stop key proved unchanged:
// a tackler that changed the code worktree hands back with cause `tampered`, and a run an orch moved meanwhile discards the verdict and acts on the fresh state instead.
// A pair's session end ends a live tackler strand with the pair's other strands, since it is an interactive strand in the pair's reed session.
//
// # Peers
//
// `lyx loom peers` lists the other runs in flight in the hub, and `lyx loom peers show <slug> --record decision|plan` prints one peer's decision record or its plan's `00-overview.md` under a `content` key.
// An agent in one pair uses them to compare its run with the others without forming another worktree's path itself.
//
// The peer set is every board task whose status is a run status, whatever the run's state, with the caller's own slug left out.
// Each peer line carries `slug`, `title`, `status`, `parent_branch`, `changed_paths` (the committed paths between the peer's merge base with its parent branch and its head), `decision_record` and `plan` (whether the peer holds them) and `unreadable`.
// A peer whose pair, origin record, branch or records cannot be read is listed with that reason in `unreadable`, never dropped and never an error.
// From a task pair the list also carries `parent_commits`, the commits on the pair's parent branch since the pair forked, as short SHA and subject, newest first; the prime has none.
// Both verbs refuse in a hub with no board, and `show` refuses an unknown slug, a task holding no run status, the caller's own slug, an unknown record name and a record the peer lacks, each naming its way forward.
//
// The verbs write nothing and read only the board, the peers' two records and git history through `internal/fabricengine`.
// They resolve the location through `lyxcwd` in the group's own pre-run, as the `review` and `decision` groups do; every loom verb is hub-only, so `internal/cliwire` does not apply.
// A bare `lyx loom peers` outside a lyx worktree lists the group's verbs instead of resolving.
//
// # Parent-review alignment
//
// The wiring sets the parent-review gate's Aligner to an adapter over an `internal/alignment` session rooted at the run's `parent-review` alignment directory, one agent per round (key `round-<N>`).
// Alignment is off, and the gate delivers straight to the run's parent, when `skip_alignment` is set or the hub has no board dir.
// The decision snapshot taken at spawn covers the decision record and every board note naming the task; an agent approve is turned into an escalation when the snapshot has changed by the time it is polled, so a decision recorded during the review is never passed over.
// An escalation, a timeout or any failure leaves the gate to deliver to the parent, with the reviewer brief naming the agent's report and the reason.
// The status commit stages the alignment directory with the run records.
package loomcli
