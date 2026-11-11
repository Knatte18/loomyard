// Package tenderengine implements the pause, resume, revive and status sequences behind `lym tender`, over seams.
//
// A pause records an entry and waits a short grace for busy sessions and network operations to finish.
// It then interrupts every covered session still in a running turn with Esc, and reports what it saw.
// A resume lifts entries and gives each session it interrupted one line to carry on.
// A revive gives one carry-on line to each session stalled on an API error.
// Status reads the live entries and the holds a pause leaves, and changes nothing.
//
// Every path and every probe is told through Deps; the engine resolves no working directory and spawns nothing.
// Pair is one hub pair as the sequences see it, and the CLI fills it over shuttle and reed.
//
// # Pause
//
// A pause covers the whole hub for an empty slug and one pair for a slug.
// Under the record lock it adds the entry with its grace end and the caller; a live entry of the same scope adds none.
// A request's `Caller` replaces the deps' caller on the entry; the session-skip rule still reads the deps' caller.
// It then polls each second until a stop condition holds:
// every covered session reads idle and no covered network operation is in flight;
// the grace end passes;
// or the entry is no longer live.
// An entry resumed during the grace ends the sequence with no Esc.
// A request's `Added` callback is called once with the entry a pause adds, after the record update and before the grace.
// It is never called for a scope that already had a live entry.
// Its error ends the pause at once, wrapped, with the entry left live.
//
// Each covered session that reads a running turn is recorded in the entry under the record lock.
// Only then is it interrupted through shuttle's guarded interrupt.
// A pause killed before its Esc therefore leaves the session recorded.
// The caller's own session and a session another live entry owns are skipped.
// The keys go in passes over the sessions, each followed by one wait of the provider's settle time;
// a pause therefore waits each settle once, whatever the number of sessions.
// A session that still reads a running turn after the first settle is guarded and interrupted again, so at most twice.
// Its pane is read once after the second key's settle.
// A pane the guard finds with no running turn gets no key.
// After the first key's settle that is `interrupted`;
// on the first key it is `no_running_turn`, and the session stays recorded.
// A session whose turn ended with background work is sent no key and not recorded;
// it is listed under `background` with its outstanding tasks and holds no grace.
// A pane that is not ready, a failed capture and an open rewind picker are reported as refused with the error's message.
// A session whose pane still shows a running turn after the second settle is reported as `still_busy`.
// An entry that is no longer live at a check stops the step, so no Esc follows a resume.
// The engine sends Esc only, never text; it kills no pane, strand or process and never touches the orch.
// The hub has one orch per worktree that runs one, the prime's and each beam's.
// Every reading of "the orch" here covers all of them.
// Under a hub entry the report says one of three things about an orch:
// that it ran the pause;
// that it is left to a live orch watcher of any orch;
// or that it is not interrupted because no orch watcher is live.
//
// # Resume
//
// An empty slug resumes every live entry; a slug resumes that pair's own entry.
// A slug that only a live hub entry covers is refused, naming `lym tender resume`.
// An adopt pause is an operator pause: a resume of its slug lifts it, and a pause of its slug answers already paused.
// Under the record lock each resumed entry moves to the resumed list,
// and each session it owns that another live entry still covers moves to that entry with no line.
// After the lock, each remaining non-orch session gets `pauserecord.ResumeLine` once through the guarded send.
// There is no busy pre-check: a recorded session gets its attempt whatever its state reads now.
// A strand that is not live and a send the guard refuses get no line and are named with the reason.
// The reason is the guard's own for a held send and the error's message for any other failure, such as a rewind picker.
// A send refused because a live entry added after that lock covers the session is claimed into that entry.
// The claim uses `pauserecord.ClaimSession`, and the report lists the session as moved;
// the later resume of that entry gives it its line.
// A claim that finds no covering entry means the pause was lifted since the send, so the line is sent once more.
// A second refusal is claimed once more and never sent a third time.
// A prime session, which has the empty slug, is given its line through the prime pair.
// Each orch's session is left to its own orch watcher.
// `Lift` runs the same sequence for the one live entry whose scope, caller and pause time equal an `EntryRef`.
// It lifts nothing for an entry no longer live.
//
// A lift that lifted at least one entry then spawns what a pause deferred, after the resume lines.
// The seams are `Deps.PushPair`, `Deps.PushPrimeRecords` and `Deps.SyncBoardIfOwed`, and a nil seam spawns nothing.
// `PushPair` runs once for each pair a lifted entry covered:
// every pair of `Deps.Pairs` for a hub entry, the pair of its slug for a slug entry.
// These pairs are left out:
// a pair a live entry of the updated record still covers;
// a pair `Deps.Owners` reads owned by another machine;
// and a pair no longer in `Deps.Pairs`.
// An owners reading that fails spawns no pair push, since an owner it cannot read may be another machine.
// `PushPrimeRecords` and `SyncBoardIfOwed` run once each.
// The report lists the spawns under `spawned` as `pair:<slug>`, `prime-records` and `board-sync`.
// It lists a failed spawn or owners reading under `spawn_failed`; neither fails the lift.
//
// # Revive
//
// An empty slug covers the whole hub; a slug covers that pair.
// For each live session of each task pair in scope, Revive reads the session's state.
// It skips a session that does not read idle-stalled with cause `api-error`, reporting the state read.
// It never waits for a session.
// A stalled session is then sent `ReviveLine` through the conditioned send.
// The send re-reads the state at its ready reading.
// It refuses at once if the session does not read stalled on an API error.
// The coverage read of the record and the send run inside one `pauserecord.Update` that changes no entry.
// They hold the record's write lock for one send at a time.
// A session a live entry covers at that read is reported `paused` and sent nothing, since resume gives it its line.
// A send with a turn start seen is `sent`, and one that landed with none seen is `sent_no_turn_start`.
// An unlanded send is `not_landed`, and a session that changed state meanwhile is skipped.
// Any other refusal, such as a draft, operator activity or the rewind picker, is `refused` with its reason.
// No send is retried.
//
// For the hub scope only, Revive then reads the state of each orch `Deps.Orchs` lists, through that orch's own `Pair`.
// No other prime session is read, revived or reported.
// The report carries one `orchs` result per orch, naming its strand.
// An orch stalled on an API error gets its line as a checked creel inject, injected through its `Orch.InjectRevive`.
// The pending check and the inject run inside one `pauserecord.Update` that changes no entry.
// That lock also serialises tender's task-session revive sends.
// Under it a live hub entry answers `paused`.
// A revive inject of tender's own that has no outcome yet answers `pending` through `Orch.RevivePending`.
// Otherwise the line is injected and the report says `injected` with the inject's id.
// So two concurrent revives inject at most once.
// The engine never types into the orch's pane.
//
// Bound: Revive types one fixed line only into a session that reads stalled on an API error at the send's own reading.
// For the orch it types only through creel's own checked delivery.
// It sends no key, kills nothing and changes no live entry.
// A second revive therefore finds a revived session busy or done and sends nothing.
// It finds the orch's inject still without an outcome and injects none.
// A pause, resume or orch watcher record write waits at most one send's hold.
// It may instead wait one orch revive's creel lookup and inject.
// Each is capped by the Go client's call timeout, since `pauserecord.Update` blocks on the lock with no timeout.
// Coverage reads take no lock.
// The orch's condition is read before the inject instead of at typing.
// A session that leaves the stall within the inject's one-minute time to live can still get the line typed.
// That starts one extra turn.
// A pause added while a line is in flight takes its entry after the session's turn start reads or its wait ends.
// Its grace poll therefore reads a started turn busy.
// A line that landed with no turn start seen is reported `sent_no_turn_start`.
// Status shows that session busy under any live entry, so the operator can pause that slug again.
//
// # Status
//
// Status reports the live entries, the network operations in flight, and the holds.
// Operations run under a parent lock are marked.
// The holds are of two kinds:
// each parent branch a covered run holds with the runs queued behind it;
// and each covered run waiting in a queue with the runs behind it.
// It adds these read-only parts:
//
//   - Every live session of every task pair and then of the prime.
//     Each has its state, cause and since, and whether a live entry covers its slug.
//     A prime session is covered only by a hub entry.
//     A session busy on background work lists the kind of each outstanding task under `background`.
//     A session whose state reading fails is listed with the state `unknown` and the error as its cause.
//   - The processes running a replaced binary:
//     each process whose exe is a deleted file with the running lym's own file name.
//     Its cwd must be the hub or inside it.
//     A platform that cannot list processes reports the part unsupported,
//     and any other failed reading is reported in the part.
//     The rest of the report is unaffected either way.
//   - The processes: each live process registered in the owner record, by owner, with its role, pid and age.
//     A record reads as an orphan when its owner is a slug that no registered worktree carries as its base name.
//     The slug must also have no live batten record to read as an orphan.
//     The hub owner is never an orphan.
//     A record is not an orphan either when the process's working directory lies inside a registered worktree.
//     Also listed are the lym processes in a deleted directory of the hub, outside every registered worktree.
//     These are processes that no record registers and whose exe is not deleted.
//     A platform that cannot list processes keeps the owned records and reports the unregistered list unsupported.
//     A failed record or worktree reading is reported in the part;
//     with the worktrees unread no record reads as an orphan.
//   - The owners: each local pair with the machine that owns it.
//     The part then lists each run-held board entry owned by another machine that has no local pair.
//     The orch therefore never reads a run owned elsewhere as stalled or landed.
//     A seam that is not told reports none, and a failed reading is reported in the part.
//
// Status reads no transcript and no pane content and writes nothing, so any caller may run it.
//
// # Reap
//
// Reap takes one fresh reading of the processes part and ends exactly the orphans it lists.
// It ends them through the `Deps.EndOwner` and `Deps.EndProcess` seams, and a nil seam ends nothing.
// An orphan owner whose every record reads orphan is ended whole, one `EndOwner` call per owner.
// `EndProcess` ends an orphan record of an owner that keeps a record working inside a registered worktree.
// It is given the record's start time.
// Each unregistered orphan is ended the same way with the start time of the same reading.
// A pid reused since is left alone.
// These are never orphans, so a reap never ends them:
// the hub owner, a pair that exists, a process in a registered worktree and a slug whose batten reads running.
// The report lists what it ended and what survived, and the processes reading it acted on.
// A survivor returns the report with an error naming each survivor's pid and registered role.
// The error also names the way forward: run the verb again once the survivor is gone.
// Any other end error is returned after the remaining orphans are tried.
package tenderengine
