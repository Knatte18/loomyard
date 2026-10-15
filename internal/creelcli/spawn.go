// spawn.go starts the detached creel daemon a verb's client asks for when none answers.

package creelcli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// spawnDaemon starts `<this binary> creel daemon --hub-path <hub>` detached from the hub directory, with null stdio.
// Under go test it starts nothing and returns an error, so no test re-executes the test binary.
func spawnDaemon(hubPath string) error {
	if testing.Testing() {
		return errors.New("creel: the daemon is not spawned under go test")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("creel: resolve this binary: %w", err)
	}
	cmd := exec.Command(exe, "creel", "daemon", "--hub-path", hubPath)
	cmd.Dir = hubPath
	proc.Detach(cmd)
	// A detached start with no wait: the spawn is logged and there is no teardown to observe.
	logger.Info("creel: spawning the detached hub daemon", "exe", exe, "hub", hubPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", exe, err)
	}
	return nil
}
