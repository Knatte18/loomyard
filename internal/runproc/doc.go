// Package runproc owns the owner record of a run's processes and the one sequence that ends them.
//
// Every detached or long-lived process lyx starts for a run registers a Record under its owner: the run's slug, or HubOwner for the hub's own processes.
// A record holds the pid, the process group when the process leads it, the start time, the role (`RoleDriver`, `RoleLoop`, `RoleBatten`, `RolePush`, `RoleSync`, `RoleDobby` or `RoleCapture`), the command line and the registration time.
// Register writes `<dir>/<owner>/<pid>-<start time>.json` through `internal/state`, so a key the record does not declare survives a rewrite by an older binary.
// The package is told its directory and derives no path.
// `HubOwner` is `_board`, a name the fabric slug check refuses as reserved hub geometry, so no slug can take it.
//
// No process removes its own record.
// List prunes on read: it drops every record whose pid is dead or now carries another start time, with its lock file, and skips a file that does not decode.
// Where no start time can be read, as on Windows, a record with an empty start time lives while its pid does.
// Owners lists every owner directory the same way.
//
// End, EndSparing and EndRole are the only ways an owner's registered processes are ended, and End is the base sequence.
// For an owner's live records, leaving out the caller and its ancestors, it reads every process once, then sends SIGTERM to each recorded leader's group, to each recorded non-leader by pid, and to every process below a record: through its own group when it leads one, by pid otherwise, and not at all when a group already signalled holds it.
// A signalled group's other live processes are watched like its leader, so each is awaited, sent SIGKILL and reported; a signal call that fails still counts as sent, so its processes are reported while they live.
// It waits up to `endGrace` for the signalled processes to disappear, then sends SIGKILL to what is left, and waits `killBound`.
// Every signal follows a comparison with the last reading: a pid is signalled only while its start time matches, and a group only while a live process still reports it with a start time at or after the recorded one.
// A pid that is a zombie, absent or carries another start time counts as gone.
// End re-lists the owner up to `endPasses` times, so a process registered during the end is reached.
// It returns each process it ended and, for processes that survive SIGKILL, a SurvivorsError that names them.
// EndSparing is End with named roles left alone, the processes below them included; its one caller is the session end that is not a teardown, which spares `RoleDobby`.
// EndRole is End for one role: it ends only the live records of that role under the owner, and the processes below them, with End's signals, grace, kill bound and passes, and leaves every other record and the processes below it alone.
// EndProcess runs the same sequence on one unregistered pid, for the orphans `lyx tender reap` ends.
//
// On Windows no start time can be read and no group can be signalled, so End, EndSparing, EndRole and EndProcess signal nothing.
//
// The package imports the standard library, `internal/proc`, `internal/state` and `internal/logger` only.
package runproc
