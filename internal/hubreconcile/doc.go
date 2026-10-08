// Package hubreconcile reconciles a hub's config once after a binary change, under a hub lock, and commits everything it wrote.
// Its one entry point is Ensure; a start verb calls it before loading any module config.
//
// The package is told the hub's geometry and derives none (PATTERN-told-geometry), so it never imports internal/lyxcwd.
//
// # Stamp, lock and key
//
// The build stamp is <BoardDir>/.lyx/config/build-stamp.json and the hub lock <BoardDir>/.lyx/config/reconcile.lock, both in the never-tracked mirror of the hub-wide config dir.
// The stamp holds a build key, the hex SHA-256 of the binary's VCS revision, its modified flag and the config registry's fingerprint, with the revision and the flag in readable form for the log.
// A stamp naming the running key makes Ensure return after one file read, with no lock and no git; an absent or unreadable stamp counts as stale.
// The lock wait defaults to DefaultLockWait and is an Options field; a lock still held when it runs out returns *LockTimeoutError and touches nothing.
// The holder's own per-worktree commits wait on each pair's write lock as a plain commit does, unbounded.
//
// # The walk
//
// Under the lock the stamp is read again, and a fresh one returns nil.
// The walk reconciles the hub-wide config and commits it on the board, then reconciles the prime, then lists the hub's code worktrees again and reconciles each pair not yet walked.
// It repeats until a listing finds no new pair, so a pair registered while the walk ran is walked before the stamp is written.
// Each worktree's applied files are committed in that worktree, and the board commit's push failure is logged and never fatal.
// Worktrees that committed before a failure keep their commits.
//
// # Skips
//
// A pair that is not complete, as a pair still being created or one a killed creation left partway, is skipped before anything is opened or written.
// That skip leaves the stamp alone: a pair being created is reconciled by its creator's single-pair call, and a partial pair never keeps every later start verb walking.
// A worktree whose directory is gone is skipped as removed.
// A worktree that is mid-merge is skipped as well: it is not written when the merge state is seen up front, and a merge that begins between the write and the commit restores the files to their prior bytes.
// A mid-merge skip leaves the stamp absent, so the next start verb retries.
//
// # The pair call
//
// Options.Pair reconciles that one pair unconditionally, whatever the stamp says, under the same lock and with the same mid-merge refusal.
// It reads and writes no stamp, and only that pair's config files are committed.
//
// # Errors
//
// *WorktreeError names the worktree, the file and the cause of a reconcile or commit failure, and ends with the way forward.
// *LockTimeoutError names the lock.
//
// # Accepted gaps
//
// A fabric add whose fork read the prime before a walk committed it, and whose pair became enumerable only after the walk's last listing, keeps the prime's older config; Loom-Preflight's retired-key refusal backs that up.
// Two risks are accepted: a downgrade flip-flops the stamp between two binaries, and a lyx-authored records commit in a running pair can land beside a driver's own work.
package hubreconcile
