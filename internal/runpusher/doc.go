// Package runpusher is the background process that brings one worktree's run commits to origin in order.
// A transition commits its records locally and starts the pusher for its records worktree, never waiting for a push.
//
// # One pusher per target
//
// A Target names the branch a pusher serves.
// It is the records of the prime or of a task pair, the board's branch, or the landing target of the prime's code branch.
// A pair's target carries its run's slug.
// The prime's pusher carries the commits of every batten run and names no one run.
// LockPath is the pusher's lock, one per target.
// A process that holds it is the target's pusher, and a second one exits at once.
//
// # Starting
//
// Start and StartIfAhead spawn the pusher verb, `lym loom pusher`, when every start condition holds:
// the lock is free, and no refused push of the target's branch stands, as TargetRefused reads it.
// For a pair, no pause may cover the run, and no hand-over mark may name it.
// StartIfAhead also requires something origin lacks, and writes or drops the target's ledger entry as it reads.
// The board is ahead while it holds an uncommitted file or an unpushed commit,
// and every target is ahead while its entry holds a landing.
// The landing target is also ahead while the prime's code branch carries an unsettled landing commit.
// PruneLedger drops the entries of worktrees that are gone.
// A process with FABRIC_SKIP_GIT or FABRIC_SKIP_PUSH set starts none.
// The spawn is detached and runs in the hub's Bolt directory.
// It is registered under the hub's owner with the push role, so a run's teardown, cancel or hand-over never ends it.
// RestartPusher is the one start a dobby tick and a resume share: a failed start is logged and never fails its caller.
//
// # Running
//
// Run is the pusher's body.
// It takes the lock, then pushes until the target is level with origin, and releases the lock.
// It pushes the target at most once per interval in force, one push attempt with no transport retry of its own.
// Each attempt reads the pause first, then runs the guard, then pushes.
// The board's pause read is the pause gate's, so its pusher yields to a pause that covers the hub.
// An owed push doubles the interval up to the ceiling, and a push that lands returns it to the base.
// No owed push posts a notice.
// A refused push, a hand-over, a pause and a guard stop end the pusher; the next commit, tick or resume starts it again.
// A commit made while the pusher exits is pushed by it or by the pusher that commit started.
//
// The push ledger holds each target's entry: when the push became owed, the last attempt and the interval in force.
// A pusher started within the interval waits out its rest, so the timing holds across pusher processes.
// The entry's push-now mark skips one wait, and a change to the ledger carrying it ends a wait early.
// A level target's entry is removed unless it holds landings, held records, dropped landings or owed hand-overs.
// A parent target is also ahead while it owes a hand-over or the reset past a dropped commit.
// A landing target whose push left every landing standing waits on the push-now mark alone.
//
// # Parent targets
//
// Runs land on the prime's code branch and on a beam pair's code branch.
// The target of the first is the landing target, and the target of the second is the beam pair's own.
// RecordLanding adds a landing to that target's entry.
// A parent pass brings the owners up from origin and records again a landing the ledger lost.
// It then settles each landing origin's parent holds, with the run's done write.
// It drops a landing whose work origin holds under another commit.
// The reset and the hand-over a landing leaves owing are made again on every pass until they succeed.
// It pushes the newest tip nothing holds back, and never a commit the parent no longer holds.
// Nothing moves while another machine owns the parent or a carried run: the push is refused and recorded.
// A refused parent target starts no pusher; SettleRefused settles it once its way forward ran.
package runpusher
