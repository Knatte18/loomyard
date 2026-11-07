// Package runproc owns the owner record of a run's processes and the one sequence that ends them.
//
// Every detached or long-lived process lym starts for a run registers a Record under its owner:
// the run's slug, or HubOwner for the hub's own processes.
// A record holds the pid, the process group when the process leads it, and the start time.
// It also holds the role, the command line and the registration time.
// The role is one of the `Role` constants.
// A fixer strand role that `internal/gatefixer` declares is also valid; its `Registrar` registers it.
// Register writes `<dir>/<owner>/<pid>-<start time>.json` through `internal/state`,
// so a key the record does not declare survives a rewrite by an older binary.
// The package is told its directory and derives no path.
// `HubOwner` is the Bolt checkout's directory name, which the fabric slug check refuses as reserved hub geometry,
// so no slug can take it.
//
// No process removes its own record.
// List prunes on read:
// it drops every record whose pid is gone or carries another start time, together with its lock file;
// it skips a file that does not decode.
// A zombie has ended and only waits for its parent's wait.
// List leaves its record out of the result but keeps the file until the zombie is reaped;
// members of the zombie leader's group may still run, and End still reaches them through the kept record.
// Where no start time can be read, as on Windows, a record with an empty start time lives while its pid runs.
// Owners lists every owner directory the same way.
//
// End, EndSparing and EndRole are the only ways an owner's registered processes are ended, and End is the base sequence.
// For an owner's live records, leaving out the caller and its ancestors, End reads every process once.
// It then sends SIGTERM to three targets:
// each recorded leader's group;
// each recorded non-leader, by pid;
// every process below a record.
// A process below a record is signalled through its own group when it leads one and by pid otherwise;
// it is not signalled at all when a group already signalled holds it.
// A signalled group's other live processes are watched like its leader, so each is awaited, sent SIGKILL and reported;
// a signal call that fails still counts as sent, so its processes are reported while they live.
// It waits up to `endGrace` for the signalled processes to disappear, then sends SIGKILL to what is left.
// Afterwards it waits `killBound`.
// Every signal follows a comparison with the last reading:
// a pid is signalled only while its start time matches;
// a group only while a live process still reports it with a start time at or after the recorded one.
// A pid that is a zombie, absent or carries another start time counts as gone.
// End re-lists the owner up to `endPasses` times, so a process registered during the end is reached.
// It returns each process it ended and, for processes that survive SIGKILL, a SurvivorsError that names them.
// EndSparing is End with named roles left alone, the processes below them included.
// The session end that is not a teardown uses it to spare `RoleDobby`.
// EndRole is End for one role.
// It ends only the live records of that role under the owner, and the processes below them.
// It uses End's signals, grace, kill bound and passes, and leaves every other record and the processes below it alone.
// EndProcess runs the same sequence on one unregistered pid, for the orphans `lym tender reap` ends.
//
// On Windows no start time can be read and no group can be signalled.
// End, EndSparing, EndRole and EndProcess therefore signal nothing.
//
// The package imports only the standard library and these internal packages:
// `internal/proc`, `internal/state`, `internal/logger` and `internal/lymcwd`.
package runproc
