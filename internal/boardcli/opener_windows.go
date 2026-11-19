// opener_windows.go starts the default-browser opener on Windows.

package boardcli

import (
	"os/exec"

	"github.com/Knatte18/loomyard/internal/logger"
)

// openerName is the command that opens a file in the default browser.
const openerName = "rundll32"

// startOpener starts the opener on path and returns the channel its exit arrives on.
func startOpener(path string) (<-chan error, error) {
	logger.Info("board open: starting browser opener", "opener", openerName, "path", path)
	return waitOnOpener(exec.Command(openerName, "url.dll,FileProtocolHandler", path))
}
