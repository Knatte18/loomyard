// Package lyxtemp owns lyx's per-user temp root in the system temp directory, and one locked run directory per process inside it.
//
// The root is `<system temp>/lyx-<uid>` off Windows and `<system temp>\lyx` on Windows, where the system temp directory is already per user.
// It is created with mode 0700 and refused unless it is a real directory, which off Windows also means mode exactly 0700 and an owner equal to the user.
// Nothing falls back to the bare system temp directory: a refused root is an error, and an error path creates nothing.
//
// A run directory is `os.MkdirTemp(root, "r")` holding a lock file `run.lock`, which its process holds until it removes the directory.
// The names stay short so a tmux socket path below a run directory stays under the `sun_path` limit.
// A process whose system temp directory is itself a run directory under a lyx root takes that root as its own, so a child works beside its parent and never inside it.
//
// Dir, MkdirTemp, CreateTemp and Remove are the process API: Dir creates the process's run directory on first use, MkdirTemp and CreateTemp create under it, and Remove deletes it.
//
// Root.Sweep removes the run directories of dead owners.
// It reads only the root's direct children and considers a child only when its name carries the run prefix and it is a directory that is no link.
// A child within the creation grace of its modification time is skipped, which covers the window between a run directory's creation and its owner's lock.
// Past the grace a child with no lock file, or whose lock file the sweep can take, is a candidate: the OS releases a dead owner's lock, so a killed process's run directory is removed by the next start's sweep.
// With apply false the sweep removes nothing and lists the same candidates.
// Bound: the sweep never reads inside a live run directory and never touches an unprefixed entry, a file or a link.
//
// The package imports the standard library and `internal/lock` only.
package lyxtemp
