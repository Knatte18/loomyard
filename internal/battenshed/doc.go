// Package battenshed owns the four task-worktree batten producers: creating the task worktree,
// seeding its own inner run, running that run inside it to a terminal state, and tearing it down.
// Any producer list may name them by reference, the same way internal/landingshed frames its own
// two producers.
//
// Told-geometry tier: this package takes every absolute path it operates on from its caller and
// has no direct production import of internal/lyxcwd, per PATTERN-told-geometry.
// Its seam_enforcement_test.go enforces that membership mechanically.
//
// This package describes one repository throughout. It is not in the Fabric Vocabulary
// Invariant's owner set, so none of its identifiers, string literals, or comments may name either
// fabric-internal side -- write "the task worktree" and "the pair" instead of naming either side
// by name.
//
// The InnerRun row notices a child that left running, a driver strand found dead while the child runs, and a running child whose status file stays unchanged for the quiet window (notice_quiet_min) with its driver alive, and it notifies through the injected Notify seam (notice.go).
// A notice is one line, sent once per episode; an episode ends when the child returns to running or the status file changes, and a marker under the row's scratch directory keeps a batten restart from notifying an episode already notified.
// Control flow is unchanged: a notice is informational, a Notify error is only warned about, and the outcomes, bounce budget, halts and waits are the same with or without it.
// A driver that is alive but parked -- a provider waiting on an interactive prompt its launcher never answered -- or that stops of its own accord with the run still non-terminal is told apart from a working one only by the quiet window; an operator attaches to the child's session to tell the cases apart.
//
// The InnerRun row waits on its child inside its call rather than returning once per poll.
// Every poll_interval_s it stats the child's status file, and decodes it when its modification time differs from the last decode or notice_probe_s has passed.
// It returns a budget-exempt Stuck only when something is worth a history entry:
// the child's state changed, and the Stuck's Path and Reason name the change;
// batten's own status carries pause_requested;
// or the status file cannot be stat'ed.
// An arm's own event ends it too: an awaiting child's decision is acted on and then waited out, and a done child's driver ending or its grace elapsing returns Done.
// File stats and small file reads run on every check; anything costing a process or a multiplexer round trip (the driver and review reads, the forced decode, a not-parked resume retry, the notice step) runs at most once per notice_probe_s.
// Bound: a running child is waited on without a time limit, bounded by "lyx batten pause" (honoured within one check), cancellation and the notices;
// a running child whose driver is dead and whose status file does not change returns nothing from the wait, the driver-dead notice being the only signal.
//
// Seed-Child and the InnerRun row also keep the batten-watched marker of the task worktree's run (MarkWatched), which holds this batten's pid while its notices have a destination that is also the child driver's parent.
// Seed-Child asks once the seed is committed, so the child's first driver launch already renders the watched rule;
// InnerRun asks at the start of every Call and once per notice_probe_s inside the wait.
// A failure is a Warn, reads as not held, and never changes a verdict.
// Bound: only the driver's "run stopped" message is removed, and only where a live batten with a notice destination replaces it at render time.
// A driver launched unwatched and adopted later keeps its own rule, a duplicate and never a loss.
// A driver launched watched keeps the silent rule whatever happens to batten afterwards,
// so a batten that dies after the render, or an orch strand that disappears after it, leaves the run's stops unannounced until a batten runs again, which then sends the notices not yet sent.
//
// A running child whose spawn this batten process has not confirmed is spawned again,
// so a restarted batten brings a driverless child back up:
// the confirmation marker holds the pid of the process that wrote it,
// and a marker from another pid or in the old layout reads as unconfirmed.
// A child with a live or retiring driver strand in its reed state, or a held run lock, is adopted instead, with the confirmation recorded for this process,
// so a second driver is never stacked.
//
// A child that halts (blocked, paused or failed) is a budget-exempt wait, not a failure of the Run-Shed row:
// batten never spawns or resumes a halted child, logs one Warn per halt episode, and keeps waiting with the child's state, error, current producer and the resume command ("lyx loom resume" in the task worktree) as its reason.
// A halted or awaiting child whose reed state holds a dead driver strand has its pair's strands revived through reed resume, once per halt episode per batten process;
// the run stays halted and nothing is resumed,
// so the operator's "lyx loom resume" wakes the revived driver.
// A retiring strand is not revived and a child with no driver strand has none to revive;
// a revive that fails, and a child with no driver strand, make the reason also name "lyx loom start" in the task worktree.
// The wait has no time limit and spends no bounce budget; "lyx batten pause" stops it within one check, and the row reads the child as running again once the operator resumes it.
//
// Its counterpart is equally by design: a driver that finishes NORMALLY leaves its strand and its
// run directory behind. Nothing here tears either down as part of a clean finish -- only the
// whole-worktree teardown row cleans up, at the very end, by removing the worktree they live in.
// The done arm does wait for the driver strand, up to the driver-exit grace window, so the driver can finish its stop report (loom's post-run friction reflection included); a driver still alive past the window is ended by the teardown's session shutdown and recorded as an abandoned session.
//
// Worktree-Teardown's two halves, session shutdown and worktree removal, call the pair-teardown composite (internal/pairteardown) as its EndSession and RemovePair phases.
// Session shutdown no longer skips a gone task worktree, since the composite ends that session by name.
// A pair of which nothing remains is the done post-condition for both halves: session shutdown has nothing left to end, and the removal reports the pair not found.
//
// The two producers that hold the hub's prime lock, Worktree-Create and Worktree-Teardown, wait for a contended lock instead of halting on the first try:
// they poll it every two seconds for up to ten minutes, stop at once when the context is cancelled, and return Stuck only when the bound is spent (primelockwait.go).
//
// It declares its own unexported entryErr/cancelErr helpers (ctx.go) and its own reportStuck
// carrier (stuck.go) for the same deliberate-duplication reason internal/preflightshed/doc.go and
// internal/landingshed/stuck.go already record: each producer-owning package carries its own copy
// rather than sharing one across packages it otherwise has no reason to depend on.
package battenshed
