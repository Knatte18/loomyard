// Package runpusher is the background process that brings one worktree's run commits to origin in order.
// A transition commits its records locally and starts the pusher for its records worktree, never waiting for a push.
//
// # One pusher per records worktree
//
// A Target names the worktree a pusher serves: the prime, or a task pair with its run's slug.
// The prime's pusher carries the commits of every batten run and names no one run.
// LockPath is the pusher's lock, one per records worktree.
// A process that holds it is the worktree's pusher, and a second one exits at once.
//
// # Starting
//
// Start and StartIfAhead spawn `lym loom pusher --worktree <path> [--slug <slug>]` when every start condition holds:
// the lock is free, and no refused-push record stands.
// For a pair, no pause may cover the run, and no hand-over mark may name it.
// StartIfAhead also requires commits origin lacks.
// A process with FABRIC_SKIP_GIT or FABRIC_SKIP_PUSH set starts none.
// The spawn is detached and runs in the hub's Bolt directory.
// It is registered under the hub's owner with the push role, so a run's teardown, cancel or hand-over never ends it.
// RestartPusher is the one start a dobby tick and a resume share: a failed start is logged and never fails its caller.
//
// # Running
//
// Run is the pusher's body.
// It takes the lock, then pushes until the worktree is level with origin, and releases the lock.
// Each attempt reads the pause first, then runs the guard, then pushes.
// PushUntilLanded retries an owed push without end, backing off from one second to a minute.
// It calls its notice once the wait first reaches the minute.
// A refused push, a hand-over, a pause and a guard stop end the pusher; the next commit, tick or resume starts it again.
// A commit made while the pusher exits is pushed by it or by the pusher that commit started.
package runpusher
