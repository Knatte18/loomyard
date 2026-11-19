// startedlines.go renders and parses the stderr line a gate tier run prints to say how many test binaries it started.
// It also names the one tier a copies call may never run.

package gateslot

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// BilledTier is the number of the tier that drives a billed LLM session.
// A `lym gate test --copies` call never runs it.
const BilledTier = 3

// startedLinePattern matches the line FormatStartedLine renders.
var startedLinePattern = regexp.MustCompile(`^gate test: started (\d+) test binaries in (.+)$`)

// StartedRun is one started line read back: the run it names and the packages whose test binary started.
type StartedRun struct {
	// Run is the run's label, such as `tier2` or `tier2 copy 1`.
	Run string
	// Started is the number of packages whose test binary started in the run.
	Started int
}

// FormatStartedLine renders the line a tier run prints to say that started packages launched a test binary in run.
// A run that started none prints it with 0.
func FormatStartedLine(run string, started int) string {
	return fmt.Sprintf("gate test: started %d test binaries in %s", started, run)
}

// ParseStartedLines reads every started line of output, in order, and skips every other line.
func ParseStartedLines(output string) []StartedRun {
	var runs []StartedRun
	for _, line := range strings.Split(output, "\n") {
		match := startedLinePattern.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if match == nil {
			continue
		}
		started, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		runs = append(runs, StartedRun{Run: match[2], Started: started})
	}
	return runs
}
