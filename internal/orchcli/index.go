// index.go builds the watcher's command-index source: the operator index read from the `lyx` binary the orch strand's pane exported.

package orchcli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// indexWayForward is the way forward every unusable-binary refusal names.
const indexWayForward = "run `lyx orch stop`, then `lyx orch start`"

// newIndexSource returns the index source the watcher renders the role file from.
// Each call reads the strands, finds the orch strand, and runs `help index` through run with the `lyx` path that strand recorded.
// It errors, naming the way forward, when there is no orch strand, when the strand recorded no path, or when the recorded path no longer exists.
func newIndexSource(strands strandOps, run func(bin string) (string, error)) func() (string, error) {
	return func() (string, error) {
		all, err := strands.Strands()
		if err != nil {
			return "", err
		}
		orch := orchStrands(all)
		if len(orch) == 0 {
			return "", fmt.Errorf("orch: no orchestrator strand is tracked, so no lyx binary is recorded: %s", indexWayForward)
		}
		bin := orch[0].LyxBin
		if bin == "" {
			return "", fmt.Errorf("orch: the orchestrator strand %s records no lyx binary, since it was launched before reed recorded one: %s", orch[0].GUID, indexWayForward)
		}
		if _, err := os.Stat(bin); err != nil {
			return "", fmt.Errorf("orch: the lyx binary %s the orchestrator strand recorded is not readable: %v: %s", bin, err, indexWayForward)
		}
		return run(bin)
	}
}

// runHelpIndex runs `<bin> help index` and returns its output.
// It never resolves `lyx` through PATH: the caller names the binary.
// A failure returns the error with the command's stderr.
func runHelpIndex(bin string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, "help", "index")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	logger.Info("orch: running help index", "bin", bin)
	if err := cmd.Run(); err != nil {
		logger.Info("orch: help index failed", "bin", bin, "err", err)
		return "", fmt.Errorf("orch: %s help index failed: %w: %s", bin, err, strings.TrimSpace(stderr.String()))
	}
	logger.Info("orch: help index finished", "bin", bin, "bytes", stdout.Len())
	return stdout.String(), nil
}
