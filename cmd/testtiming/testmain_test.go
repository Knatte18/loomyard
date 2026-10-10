// testmain_test.go runs the test binary under tmux isolation, which the integration-tagged prebuild test requires of its package, and makes a re-exec'd test binary a stand-in `go` when the prebuild test asks for one.
// It carries no build tag, so it compiles into every tag set.

package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// standInGoRecordEnv names the file a re-exec'd test binary, acting as a stand-in `go`, records its arguments into.
const standInGoRecordEnv = "TESTTIMING_STAND_IN_GO_RECORD"

// TestMain runs a stand-in `go` when standInGoRecordEnv is set, and the tests under tmuxkit.Main otherwise.
func TestMain(m *testing.M) {
	if record := os.Getenv(standInGoRecordEnv); record != "" {
		os.Exit(runStandInGo(record, os.Args[1:]))
	}
	os.Exit(tmuxkit.Main(m))
}

// runStandInGo writes args, one per line, to record and creates the empty file a `-o` among them names, as a build would, and returns the process exit code.
func runStandInGo(record string, args []string) int {
	if err := os.WriteFile(record, []byte(strings.Join(args, "\n")), 0o644); err != nil {
		return 1
	}
	if i := slices.Index(args, "-o"); i >= 0 && i+1 < len(args) {
		if err := os.WriteFile(args[i+1], nil, 0o755); err != nil {
			return 1
		}
	}
	return 0
}
