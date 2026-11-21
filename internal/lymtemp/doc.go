// Package lymtemp owns lym's per-user temp root in the system temp directory.
// Inside it, each process owns one locked run directory.
//
// The root is `<system temp>/lym-<uid>` off Windows and `<system temp>\lym` on Windows.
// The root is created with mode 0700 and refused unless it is a real directory.
// Off Windows it must also have mode exactly 0700 and be owned by the user.
// Nothing falls back to the bare system temp directory: a refused root is an error, and nothing is created.
//
// A run directory is `r` plus four random lower-case letters or digits, created 0700.
// It holds a lock file `run.lock`, which its process holds until it removes the directory.
// The names stay short so a tmux socket path below a run directory stays under the `sun_path` limit.
// A process whose system temp directory is itself a run directory under a lym root takes that root as its own,
// so a child works beside its parent and never inside it.
//
// Dir, MkdirTemp, CreateTemp and Remove are the process API.
//
// Root.Sweep removes the run directories of dead owners.
// A child of the root is a candidate only when its name carries the run prefix and it is a directory that is no link.
// A child within the creation grace of its modification time is skipped.
// The grace covers the window between a run directory's creation and its owner's lock.
// The OS releases a dead owner's lock, so the next start's sweep removes a killed process's run directory.
// With apply false the sweep removes nothing and lists the same candidates.
// The sweep never reads inside a live run directory and never touches an unprefixed entry, a file or a link.
//
// The sweep gives every directory in its tree owner read, write and search permission.
// It does so before removing a dead run directory, because a killed test binary can leave one read-only.
// That pass is best-effort: a failure in it is ignored, and the removal's own result is what the sweep reports.
//
// A removal that fails because the directory is not empty is retried until the sweep's one shared budget runs out;
// one still failing then is reported as a failure.
// Any other removal error is reported after one attempt.
// The wait is reached only while something keeps re-adding entries to a dead directory,
// and every new run pays it for as long as it lasts.
//
// The package imports the standard library and `internal/lock` only.
package lymtemp
