// opener_linux.go opens a file with the desktop's default application on Linux.

package boardcli

import (
	"os/exec"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// openerName is the program that opens a file.
const openerName = "xdg-open"

// startOpener starts the opener on the file at path, detached, and returns the channel its exit is sent on.
// A nil exit is success; the opener is reaped whether or not the channel is read.
func startOpener(path string) (<-chan error, error) {
	cmd := exec.Command(openerName, path)
	proc.Detach(cmd)
	logger.Info("boardcli: spawning the page opener", "opener", openerName, "path", path)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	return exited, nil
}
