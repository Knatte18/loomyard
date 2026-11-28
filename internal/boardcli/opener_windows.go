// opener_windows.go opens a file in the default browser.

package boardcli

import (
	"os/exec"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// openerName is the program that opens a file in the browser.
const openerName = "rundll32"

// startOpener starts the opener on path, detached, and delivers its exit once.
func startOpener(path string) (<-chan error, error) {
	cmd := exec.Command(openerName, "url.dll,FileProtocolHandler", path)
	proc.Detach(cmd)
	logger.Info("boardcli: starting the opener", "opener", openerName, "path", path)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		logger.Info("boardcli: the opener exited", "opener", openerName, "err", err)
		exited <- err
	}()
	return exited, nil
}
