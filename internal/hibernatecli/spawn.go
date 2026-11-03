// spawn.go starts the one OS process the hibernate module owns: the machine's hibernate command.

package hibernatecli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// runHibernate runs `systemctl hibernate`, which returns once the job is queued.
// A non-zero exit returns an error carrying the exit and the last line of the output.
func runHibernate() error {
	cmd := exec.Command("systemctl", "hibernate")
	logger.Info("hibernate: running systemctl hibernate")
	output, err := cmd.CombinedOutput()
	logger.Info("hibernate: systemctl hibernate returned", "error", err)
	if err == nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	return fmt.Errorf("systemctl hibernate: %w: %s", err, lines[len(lines)-1])
}
