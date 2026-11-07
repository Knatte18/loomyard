// Package lymtemp owns lym's per-user temp root in the system temp directory.
// Inside it, each process owns one locked run directory.
//
// The root is `<system temp>/lym-<uid>` off Windows and `<system temp>\lym` on Windows.
// On Windows the system temp directory is already per user.
// The root is created with mode 0700 and refused unless it is a real directory.
// Off Windows it must also have mode exactly 0700 and be owned by the user.
// Nothing falls back to the bare system temp directory: a refused root is an error, and nothing is created.
//
// A run directory is `r` plus four random lower-case letters or digits, created 0700 with `os.Mkdir`;
// a taken name is retried up to a bound and then refused naming the root.
// It holds a lock file `run.lock`, which its process holds until it removes the directory.
// The names stay short so a tmux socket path below a run directory stays under the `sun_path` limit.
// The limit holds down to a root of `/dev/shm` for a ten-digit uid.
// A process whose system temp directory is itself a run directory under a lym root takes that root as its own,
// so a child works beside its parent and never inside it.
//
// Dir, MkdirTemp, CreateTemp and Remove are the process API.
// Dir creates the process's run directory on first use, MkdirTemp and CreateTemp create under it, and Remove deletes it.
//
// Root.Sweep removes the run directories of dead owners.
// It reads only the root's direct children.
// A child is a candidate only when its name carries the run prefix and it is a directory that is no link.
// A child within the creation grace of its modification time is skipped;
// the grace covers the window between a run directory's creation and its owner's lock.
// Past the grace, a child with no lock file, or whose lock file the sweep can take, is removable.
// The OS releases a dead owner's lock, so the next start's sweep removes a killed process's run directory.
// With apply false the sweep removes nothing and lists the same candidates.
//
// A removal that fails because the directory is not empty is retried until the sweep's one shared budget runs out;
// one still failing then is reported as a failure.
// The not-empty symptom is a concurrent sweeper's lock probe recreating the lock file.
// Any other removal error is reported after one attempt.
// A sweep waits at most the budget plus one retry delay and one removal, in total across all its directories.
// The wait is reached only while something keeps re-adding entries to a dead directory.
// A concurrent sweeper's lock probe does so, and so does a process outliving the directory's dead owner.
// Every new run pays that wait for as long as it lasts.
// A persistent failure of another kind costs one attempt per dead directory.
// The sweep never reads inside a live run directory and never touches an unprefixed entry, a file or a link.
//
// The package imports the standard library and `internal/lock` only.
package lymtemp
