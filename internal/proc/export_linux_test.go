// export_linux_test.go exposes capped runs with a stand-in scope starter or none to the external tests.

package proc

import (
	"context"
	"os/exec"
)

// SelfCgroupFile is the file that names the cgroup of the process reading it.
const SelfCgroupFile = selfCgroupFile

// CgroupMount is the directory where the cgroup v2 hierarchy is mounted.
const CgroupMount = cgroupMount

// RunMemoryCappedWithStarter runs cmd capped at limit bytes in a scope that starter, standing in for systemd-run, starts.
func RunMemoryCappedWithStarter(cmd *exec.Cmd, limit int64, starter string) (killed bool, err error) {
	killed, _, err = runMemoryCapped(cmd, limit, memoryScope{starter: starter}, startDirect)
	return killed, err
}

// KilledAtOwnLimit reports whether the memory.events content of a cgroup shows a kill at the cgroup's own limit.
func KilledAtOwnLimit(events string) bool {
	return killedAtOwnLimit(events)
}

// ParseOOMKills returns the kills that the kernel messages in journal record for tasks of cgroup, oldest first.
func ParseOOMKills(journal, cgroup string) []OOMKill {
	return parseOOMKills(journal, cgroup)
}

// OOMKillCount returns the oom_kill count of the content of a cgroup's memory.events file.
func OOMKillCount(events string) int {
	return oomKillCount(events)
}

// WaitForScope waits until process pid's cgroup is the scope unit's, asking readEnded for the scope's state now and then.
func WaitForScope(ctx context.Context, readEnded func() (state string, ended bool), pid int, unit string) (string, error) {
	return waitForScope(ctx, readEnded, pid, unit)
}

// ScopeEnded reports whether the user manager at busctlPath shows the scope unit as failed or inactive.
func ScopeEnded(ctx context.Context, busctlPath, unit string) (state string, ended bool) {
	return scopeEnded(ctx, busctlPath, unit)
}

// UnitObjectPathLabel returns the last element of the D-Bus object path of the unit named unit.
func UnitObjectPathLabel(unit string) string {
	return unitObjectPathLabel(unit)
}

// ScopeStateEnded reads busctl's answer to a get-property of a unit's ActiveState.
// It reports whether that state is failed or inactive.
func ScopeStateEnded(answer string) (state string, ended bool) {
	return scopeStateEnded(answer)
}

// RunMemoryCappedWithoutScope runs cmd capped at limit bytes as a host whose user manager cannot cap a scope does.
func RunMemoryCappedWithoutScope(cmd *exec.Cmd, limit int64) (killed bool, err error) {
	killed, _, err = runMemoryCapped(cmd, limit, memoryScope{}, startDirect)
	return killed, err
}
