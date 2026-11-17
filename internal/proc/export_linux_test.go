// export_linux_test.go exposes the capped run of a host with no user manager to the external tests.

package proc

import "os/exec"

// RunMemoryCappedWithStarter runs cmd capped at limit bytes in a scope that starter, standing in for systemd-run, starts.
func RunMemoryCappedWithStarter(cmd *exec.Cmd, limit int64, starter string) (killed bool, err error) {
	killed, _, err = runMemoryCapped(cmd, limit, memoryScope{starter: starter}, startDirect)
	return killed, err
}

// KilledAtOwnLimit reports whether the memory.events content of a cgroup shows a kill at the cgroup's own limit.
func KilledAtOwnLimit(events string) bool {
	return killedAtOwnLimit(events)
}

// RunMemoryCappedWithoutScope runs cmd capped at limit bytes as a host whose user manager cannot cap a scope does.
func RunMemoryCappedWithoutScope(cmd *exec.Cmd, limit int64) (killed bool, err error) {
	killed, _, err = runMemoryCapped(cmd, limit, memoryScope{}, startDirect)
	return killed, err
}
