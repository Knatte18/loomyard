// opener_windows.go opens a file in the operator's default browser on Windows.

package boardcli

import (
	"os/exec"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// openerName is the program that opens a file.
const openerName = "rundll32"

// startOpener starts the opener on path, detached, and returns the channel its exit is sent on.
func startOpener(path string) (<-chan error, error) {
	cmd := exec.Command(openerName, "url.dll,FileProtocolHandler", path)
	proc.Detach(cmd)
	logger.Info("boardcli: starting the opener", "opener", openerName, "path", path)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	// This wait also reaps the opener when nobody receives from the channel.
	go func() { exited <- cmd.Wait() }()
	return exited, nil
}
