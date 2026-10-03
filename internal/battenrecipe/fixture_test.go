// fixture_test.go implements testEnv, the package-internal test scaffolding every later test file
// in this package reuses: the shared full Env from envkit and a shedbuild.ShedPaths whose paths
// sit beside the Env's own status file.

package battenrecipe

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
)

// testEnv returns the shared full shedrecipe.Env and a shedbuild.ShedPaths in the Env's own temp root.
func testEnv(t *testing.T) (shedrecipe.Env, shedbuild.ShedPaths) {
	t.Helper()

	env := envkit.FullEnv(t)
	dir := filepath.Dir(env.StatusPath)

	paths := shedbuild.ShedPaths{
		StatusPath:     env.StatusPath,
		LockPath:       filepath.Join(dir, "run.lock"),
		StatusLockPath: env.StatusLockPath,
		MaxBounces:     0,
		CommitStatus:   nil,
	}

	return env, paths
}
