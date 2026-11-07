// testmain_test.go runs the package's tests under tmuxkit.Main, after gitkit.HermeticGitEnv for the tests that spawn git.
// Their temp files live in the process run directory and go with it.
// It sits in the external test package because gitkit reaches dotgit through lymcwd.

package dotgit_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}
