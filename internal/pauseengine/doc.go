// Package pauseengine implements the pause, resume and status sequences behind `lyx pause` and `lyx resume`, over seams.
//
// A pause records an entry in the pause record and waits a short grace for busy sessions and network operations to finish.
// It then interrupts every covered session still in a running turn with Esc, and reports what it saw.
// A resume lifts entries and gives each session it interrupted one line to carry on.
// Status reads the live entries and the holds a pause leaves, and changes nothing.
//
// Every path and every probe is told through Deps; the engine resolves no working directory and spawns nothing.
// Pair is one hub pair as the sequences see it, and the CLI fills it over shuttle and reed.
//
// # Pause
//
// A pause covers the whole hub for an empty slug and one pair for a slug.
// Under the record lock it adds the entry with its grace end and the caller; a live entry of the same scope adds none.
// It then polls each second until every covered session reads idle and no covered network operation is in flight, the grace end passes, or the entry is no longer live.
// An entry resumed during the grace ends the sequence with no Esc.
//
// Each covered session that reads a running turn is then recorded in the entry under the record lock, and only then interrupted through shuttle's guarded interrupt, so a pause killed before its Esc leaves the session recorded.
// The caller's own session and a session another live entry owns are skipped.
// The keys go in passes over the sessions, each followed by one wait of the provider's settle time, so a pause waits each settle once whatever the number of sessions.
// A session whose state still reads a running turn after the first key's settle is guarded and interrupted once more, so at most twice, and its pane is read once after the second key's settle.
// A pane the guard finds with no running turn gets no key: after the first key's settle that is `interrupted`, and on the first key it is `no_running_turn`, a session that stays recorded.
// A session whose turn ended with background work is sent no key and not recorded; it is listed under `background` with its outstanding tasks and holds no grace.
// A pane that is not ready, a failed capture and an open rewind picker are reported as refused with the error's message, and a session whose pane still shows a running turn after the second settle as `still_busy`.
// An entry that is no longer live at a check stops the step, so no Esc follows a resume.
// The engine sends Esc only, never text; it kills no pane, strand or process and never touches the orch.
// Under a hub entry the report says whether the orch ran the pause, is left to a live orch watcher, or is not interrupted because no watcher is live.
//
// # Resume
//
// An empty slug resumes every live entry; a slug resumes that pair's own entry.
// A slug that only a live hub entry covers is refused, naming `lyx resume`.
// Under the record lock each resumed entry moves to the resumed list, and each session it owns that another live entry still covers moves to that entry with no line.
// After the lock, each remaining non-orch session gets `pauserecord.ResumeLine` once through the guarded send.
// There is no busy pre-check: a recorded session gets its attempt whatever its state reads now.
// A strand that is not live and a send the guard refuses get no line and are named with the reason, which is the guard's own for a held send and the error's message for any other failure, such as an open rewind picker.
// The orch's session is left to the orch watcher.
//
// # Status
//
// Status reports the live entries, the network operations in flight with the ones run under a parent lock marked, and the holds:
// each parent branch a covered run holds with the runs queued behind it, and each covered run waiting in a queue with the runs behind it.
// It adds two read-only parts:
//
//   - Every live session of every task pair and then of the prime, each with its state, cause and since, and whether a live entry covers its slug.
//     A prime session is covered only by a hub entry.
//     A session whose state reading fails is listed with the state `unknown` and the error as its cause.
//   - The processes running a replaced binary: each process whose exe is a deleted file with the running lyx's own file name and whose cwd is the hub or inside it.
//     A platform that cannot list processes reports the part unsupported, and any other failed reading is reported in the part; the rest of the report is unaffected either way.
//
// Status reads no transcript and no pane content and writes nothing, so any caller may run it.
package pauseengine
