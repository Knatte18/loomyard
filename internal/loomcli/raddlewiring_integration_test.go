// raddlewiring_integration_test.go drives createBeamFromParent over a hub forged by hubforge.
// The package's testmain_test.go arms the hermetic git test environment for the whole binary,
// so this file adds no TestMain.

package loomcli

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCreateBeamFromParent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, hub, "the-sketch")
	location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree("the-sketch"))
	if err != nil {
		t.Fatalf("ResolveWorktree() error = %v; want nil", err)
	}

	var gotDir string
	var gotArgs []string
	err = createBeamFromParent(location, "the-project", func(dir string, args ...string) error {
		gotDir, gotArgs = dir, args
		return nil
	})
	if err != nil {
		t.Fatalf("createBeamFromParent() error = %v; want nil", err)
	}
	if want := filepath.Clean(hub.PrimeWorktree()); filepath.Clean(gotDir) != want {
		t.Errorf("run dir = %q; want the prime worktree %q, the parent of a pair forked from it", gotDir, want)
	}
	if want := []string{"beam", "create", "the-project"}; !slices.Equal(gotArgs, want) {
		t.Errorf("run args = %v; want %v", gotArgs, want)
	}

	failure := errors.New("beam create failed")
	err = createBeamFromParent(location, "the-project", func(string, ...string) error { return failure })
	if !errors.Is(err, failure) {
		t.Errorf("createBeamFromParent() error = %v; want the run's error returned", err)
	}
}
