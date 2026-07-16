// testmain_test.go wires the package's test binary into the hermetic git test
// environment: lyxtest.HermeticGitEnv() runs once before any test, so muxcli's
// git-spawning fixtures never inherit the operator's global gitconfig (see
// CONSTRAINTS.md's Hermetic Git Test Environment Invariant). It also guards
// the binary against being run AS lyx by a header pane (see TestMain).

package muxcli

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestMain first intercepts the header-pane invocation shape
// ("<binary> mux ..."): muxengine's ensureHeaderPaneLocked boots the header
// pane with os.Executable() + " mux header --blocking", and inside an
// in-process smoke test (RunCLI) that executable is THIS test binary — a Go
// test binary invoked with positional args ignores them and runs its whole
// suite, recursively, inside the pane, leaking each recursive test's tmux
// servers when the outer test tears the pane down mid-run (found by
// fable-header-r1; the leaked servers' fixture paths matched exactly). The
// guard stands in for the keepalive instead: print a marker, hold the pane
// open, stay killable (a sleep loop rather than a bare select{}, which the
// runtime's deadlock detector could kill). Otherwise it runs
// lyxtest.HermeticGitEnv() before any test spawns git, then delegates to
// the normal test run.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "mux" {
		fmt.Println("muxcli test binary standing in for the header keepalive (`lyx mux header --blocking`)")
		for {
			time.Sleep(time.Hour)
		}
	}
	lyxtest.HermeticGitEnv()
	os.Exit(m.Run())
}
