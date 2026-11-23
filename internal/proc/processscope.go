// processscope.go caps the memory of a process that is already running and reads the OOM kills its scope saw.

package proc

// OOMKill is one process the kernel's OOM killer ended.
type OOMKill struct {
	// PID is the pid the killed process had.
	PID int
	// Name is the command name the kernel logged for it.
	Name string
}

// CapProcessMemory moves the running process pid into a new transient scope named unit.
// The scope caps the memory at limit bytes and allows no swap.
// It exists so a long-lived process tree, such as an agent's pane, has a runaway descendant killed inside its scope.
// The kill happens before the host runs out of memory.
// Every process pid starts from then on joins the scope; one it started before stays where it was.
// The scope's OOM policy is continue, so a kill at the cap ends the process the kernel picks and nothing else.
// The scope is collected once it empties.
// It returns the scope's cgroup path below the cgroup v2 mount.
// An error wraps ErrMemoryCap and names its cause.
// A cause is no `busctl` on PATH.
// Another is a user manager that refuses the scope or never moves the process.
// A third is a user manager that does not hand the memory controller to its scopes.
// On Windows it always returns an error that wraps ErrMemoryCap and errors.ErrUnsupported.
func CapProcessMemory(pid int, unit string, limit int64) (cgroup string, err error) {
	return capProcessMemoryPlatform(pid, unit, limit)
}

// ProcessOOMKills returns the cgroup of process pid and the count of processes the kernel's OOM killer ended in it.
// The count covers a kill at the cgroup's own limit and one at the limit of a cgroup above it or of the host.
// On Windows it always returns errors.ErrUnsupported.
func ProcessOOMKills(pid int) (cgroup string, kills int, err error) {
	return processOOMKillsPlatform(pid)
}

// OOMKillsInCgroup returns the processes of cgroup that the current boot's kernel journal records as OOM-killed.
// It returns them oldest first.
// cgroup is a path below the cgroup v2 mount, as CapProcessMemory and ProcessOOMKills return it.
// A journal that records none returns none.
// So does a journal the caller may not read, as outside the adm and systemd-journal groups.
// An error names a journal that could not be read at all.
// On Windows it always returns errors.ErrUnsupported.
func OOMKillsInCgroup(cgroup string) ([]OOMKill, error) {
	return oomKillsInCgroupPlatform(cgroup)
}
