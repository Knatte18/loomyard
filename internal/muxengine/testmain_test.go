// testmain_test.go guards this package's test binary against being run AS
// lyx: ensureHeaderPaneLocked boots the header pane with os.Executable() +
// " mux header --blocking", and inside an in-process integration test that
// executable is THIS test binary. A Go test binary invoked with positional
// args ignores them and runs its whole test suite — recursively, inside the
// pane, each recursive test booting its own tmux servers that leak when the
// outer test tears the pane down mid-run (found by fable-header-r1: the
// machine's accumulated stray tmux servers all carried recursive-fixture
// /tmp paths). The guard makes the test binary honor the header keepalive
// contract instead: print a marker, hold the pane open, stay killable.

package muxengine

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// TestMain intercepts the header-pane invocation shape ("<binary> mux ...")
// before any test runs: it stands in for `lyx mux header --blocking` by
// printing a marker and blocking forever (a sleep loop rather than a bare
// select{}, which the runtime's deadlock detector could kill in a binary
// with no other live goroutine). Every other invocation delegates to the
// normal test run.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "mux" {
		fmt.Println("muxengine test binary standing in for the header keepalive (`lyx mux header --blocking`)")
		for {
			time.Sleep(time.Hour)
		}
	}
	os.Exit(m.Run())
}
