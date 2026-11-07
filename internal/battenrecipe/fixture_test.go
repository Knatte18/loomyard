// fixture_test.go holds testEnv, the package-internal test scaffolding that the other test files in this package reuse.
// It pairs the shared full Env from envkit with a shedbuild.ShedPaths whose paths sit beside the Env's status file.

package battenrecipe

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
)

// testEnv returns the shared full shedrecipe.Env and a shedbuild.ShedPaths beside the Env's status file.
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
