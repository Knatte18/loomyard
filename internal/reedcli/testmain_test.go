// testmain_test.go wires the package's test binary into the hermetic git test
// environment: lyxtest.HermeticGitEnv() runs once before any test, so reedcli's
// git-spawning fixtures never inherit the operator's global gitconfig (see
// CONSTRAINTS.md's Hermetic Git Test Environment Invariant). It also guards
// the binary against being run AS lyx by a header pane (see TestMain).

package reedcli

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestMain intercepts the header-pane invocation and prevents re-execution recursion.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "reed" {
		fmt.Println("reedcli test binary standing in for the header keepalive (`lyx reed header --blocking`)")
		for {
			time.Sleep(time.Hour)
		}
	}
	lyxtest.HermeticGitEnv()
	os.Exit(m.Run())
}
