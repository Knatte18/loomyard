// fixture_test.go implements testEnv, the package-internal test scaffolding every later test file
// in this package reuses: a minimal shedrecipe.Env and a shedbuild.ShedPaths, every path derived
// from one t.TempDir() root, filling only the six fields the three batten entries read and
// leaving the rest of Env zero -- which is legal, since each entry validates exactly the fields it
// reads.

package battenrecipe

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// testEnv returns a minimal filled shedrecipe.Env and a shedbuild.ShedPaths, every path derived
// from one t.TempDir() root.
func testEnv(t *testing.T) (shedrecipe.Env, shedbuild.ShedPaths) {
	t.Helper()

	dir := t.TempDir()

	env := shedrecipe.Env{
		Slug:       "test-slug",
		ScratchDir: dir,
		CreateWorktree: func(context.Context) error {
			return nil
		},
		InnerRun: battenshed.InnerRunDeps{
			Spawn: func(context.Context) error { return nil },
			ResolveStatus: func() (string, string, error) {
				return filepath.Join(dir, "loomrun-status.json"), filepath.Join(dir, "loomrun-status.json.lock"), nil
			},
			ReadStatus: func(string, string) (shedengine.Status, bool, error) {
				return shedengine.Status{}, false, nil
			},
		},
		Teardown: battenshed.TeardownDeps{
			Shutdown: func(context.Context) (string, error) { return "", nil },
			Remove:   func(context.Context) error { return nil },
		},
		PrimeLock: battenshed.PrimeLock{
			Path: filepath.Join(dir, "prime.lock"),
			Acquire: func() (func() error, bool, error) {
				return func() error { return nil }, true, nil
			},
		},
	}

	paths := shedbuild.ShedPaths{
		StatusPath:     filepath.Join(dir, "status.json"),
		LockPath:       filepath.Join(dir, "run.lock"),
		StatusLockPath: filepath.Join(dir, "status.json.lock"),
		MaxBounces:     0,
		CommitStatus:   nil,
	}

	return env, paths
}
