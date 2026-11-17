// export_linux_test.go exposes the capped run of a host with no user manager to the external tests.

package proc

import "os/exec"

// RunMemoryCappedWithStarter runs cmd capped at limit bytes in a scope that starter, standing in for systemd-run, starts.
func RunMemoryCappedWithStarter(cmd *exec.Cmd, limit int64, starter string) (killed bool, err error) {
	return runMemoryCapped(cmd, limit, memoryScope{starter: starter})
}

// RunMemoryCappedWithoutScope runs cmd capped at limit bytes as a host whose user manager cannot cap a scope does.
func RunMemoryCappedWithoutScope(cmd *exec.Cmd, limit int64) (killed bool, err error) {
	return runMemoryCapped(cmd, limit, memoryScope{})
}
