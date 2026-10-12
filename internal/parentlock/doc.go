// Package parentlock owns the parent lock: one record per parent branch that lets a single run at a time move that parent, valid exactly while its owner process lives.
//
// The package is told every directory it works on and derives none beyond Dir, which spells the ephemeral directory name only through internal/lyxdirs.
//
// # Record
//
// A Record is Branch, Owner and AcquiredAt, stored as JSON at `<Dir>/<hex(branch)>.json`.
// The branch is hex-encoded into the file name, so a name with a slash, `..` or a case clash on Windows can neither escape nor collide in the directory.
// The first write creates Dir.
// An Owner names the run (slug, pair path, run id) and the process (pid, start time); SameRun compares the run, SameProcess the run and the process.
//
// # Liveness
//
// A record is valid while its owner process is alive, meaning the pid lives and carries the recorded start time.
// A host that sleeps keeps the lock and a crashed owner frees it at once.
// No clock decides validity: Store.Now only stamps AcquiredAt and ticket times.
//
// # Queue
//
// Every read-modify-write runs under a file lock on `<Dir>/<hex(branch)>.lock`, so two acquirers never both win.
// TryAcquire takes a free record, a record whose owner is not alive and a record naming the acquirer's own run, which rewrites the owner process.
// A free or stale record goes to the oldest live ticket, kept at `<Dir>/<hex(branch)>.tickets/<pid>-<start>.json`.
// An acquirer that is not first in the queue keeps its ticket and is not granted; a dead waiter's ticket is removed in the attempt that finds it, and a granted acquirer's own ticket is removed.
//
// # Waiting
//
// Acquire loops TryAcquire until granted, with no time limit, polling from one second and doubling to thirty seconds while the holder is unchanged.
// A Waiter tells it three optional seams: OnWait when the wait starts and the holder changes, Paused before every sleep, and Sleep in place of the timer.
// A pause, a cancelled context and an attempt error each return with the caller's ticket removed.
// The wait only waits and takes; it never frees another owner's live record.
// A caller keeps its wait on disk as a Wait record at WaitRecordPath, which ReadWait ignores once its writing process is dead.
//
// # Idle
//
// An IdleWatch decides that a holder has stopped making progress: Observe reports true when no agent is busy and one has waited for the Bound on the caller's awake clock, so suspended time never counts.
// An unreadable agent state counts as nobody waiting.
//
// # Release and hand-off
//
// Release and HandOff compare on the owner process: a record owned by another process of the same run, such as batten's or a newer loop's, is left alone.
//
// # Force releases
//
// ForceRelease removes one record whatever its owner and ReleaseAll removes every record.
// Neither stops the owning run or touches tickets; the released run reacquires at its next lock-taking row.
//
// An unreadable record returns an error wrapping ErrUnreadable naming the record path and changes nothing; List reports it with its error text instead.
package parentlock
