// opener_linux.go opens a file in the user's default application on Linux.

package boardcli

import (
	"os/exec"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// openerName is the program that opens a file on this platform.
const openerName = "xdg-open"

// startOpener starts the opener on path, detached, and delivers its exit on the returned channel.
func startOpener(path string) (<-chan error, error) {
	cmd := exec.Command(openerName, path)
	proc.Detach(cmd)
	logger.Info("boardcli: spawning the opener", "args", cmd.Args)
	if err := cmd.Start(); err != nil {
		logger.Warn("boardcli: opener spawn failed", "args", cmd.Args, "cause", err)
		return nil, err
	}
	exited := make(chan error, 1)
	// This wait is the opener's reap, and keeps the exit status a failed opener is reported by.
	go func() { exited <- cmd.Wait() }()
	return exited, nil
}
