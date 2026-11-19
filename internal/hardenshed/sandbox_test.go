// sandbox_test.go covers NewSandbox and ListSandbox over fake seams.

package hardenshed

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeSandboxDeps returns deps over root whose seams make a bare directory pair and a hub directory in the container.
// The .dev-bin lym is built from head and unmodified, so Deploy is never called until a test changes the revision.
func fakeSandboxDeps(root string, deployed *int) SandboxDeps {
	return SandboxDeps{
		Root: root,
		Seed: func(dir string) (string, string, error) {
			code, records := filepath.Join(dir, "code-bare"), filepath.Join(dir, "records-bare")
			for _, bare := range []string{code, records} {
				if err := os.Mkdir(bare, 0o755); err != nil {
					return "", "", err
				}
			}
			return code, records, nil
		},
		Clone: func(_, _, container string) (string, error) {
			hub := filepath.Join(container, "tst-LYMHUB")
			return hub, os.Mkdir(hub, 0o755)
		},
		CommitOverride: func(string) error { return nil },
		DevBinLym:      "/worktree/.dev-bin/lym",
		Head:           func() (string, error) { return "head", nil },
		Revision:       func(string) (buildvcs.Identity, error) { return buildvcs.Identity{Revision: "head"}, nil },
		Deploy:         func() error { *deployed++; return nil },
	}
}

func TestNewSandbox(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("MakesHubsUnderTheRootAndListsThem", func(t *testing.T) {
		t.Parallel()
		parent := t.TempDir()
		root := SandboxRoot(filepath.Join(parent, "worktree"))
		var deployed int
		deps := fakeSandboxDeps(root, &deployed)

		first, err := NewSandbox(deps)
		if err != nil {
			t.Fatalf("NewSandbox() error = %v", err)
		}
		second, err := NewSandbox(deps)
		if err != nil {
			t.Fatalf("NewSandbox() error = %v", err)
		}

		if first.Hub == second.Hub {
			t.Errorf("both sandboxes are the hub %s; want distinct hubs", first.Hub)
		}
		if first.Lym != deps.DevBinLym {
			t.Errorf("SandboxHub.Lym = %q; want %q", first.Lym, deps.DevBinLym)
		}
		for _, hub := range []string{first.Hub, second.Hub} {
			if !strings.HasPrefix(hub, root+string(filepath.Separator)) {
				t.Errorf("hub %s lies outside the root %s", hub, root)
			}
		}
		listed, err := ListSandbox(root)
		if err != nil {
			t.Fatalf("ListSandbox() error = %v", err)
		}
		if want := []string{first.Hub, second.Hub}; !reflect.DeepEqual(listed, want) {
			t.Errorf("ListSandbox() = %v; want %v", listed, want)
		}
		if entries, err := os.ReadDir(filepath.Join(parent, "worktree")); err != nil || len(entries) != 1 {
			t.Errorf("the worktree directory holds %v, %v; want only the .lym directory", entries, err)
		}
	})

	t.Run("ListsNothingBeforeAnyHubIsMade", func(t *testing.T) {
		t.Parallel()
		listed, err := ListSandbox(SandboxRoot(t.TempDir()))
		if err != nil || len(listed) != 0 {
			t.Errorf("ListSandbox() = %v, %v; want no hub and no error", listed, err)
		}
	})

	deployTests := []struct {
		name         string
		built        buildvcs.Identity
		wantDeployed int
	}{
		{"DeploysWhenTheRevisionDiffers", buildvcs.Identity{Revision: "older"}, 1},
		{"DeploysWhenTheBuildIsModified", buildvcs.Identity{Revision: "head", Modified: true}, 1},
		{"SkipsDeployWhenTheBuildIsHeads", buildvcs.Identity{Revision: "head"}, 0},
	}
	for _, tt := range deployTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var deployed int
			deps := fakeSandboxDeps(SandboxRoot(t.TempDir()), &deployed)
			deps.Revision = func(string) (buildvcs.Identity, error) { return tt.built, nil }
			if _, err := NewSandbox(deps); err != nil {
				t.Fatalf("NewSandbox() error = %v", err)
			}
			if deployed != tt.wantDeployed {
				t.Errorf("Deploy calls = %d; want %d", deployed, tt.wantDeployed)
			}
		})
	}

	failureTests := []struct {
		name   string
		mutate func(*SandboxDeps)
	}{
		{"FailingSeed", func(d *SandboxDeps) {
			d.Seed = func(string) (string, string, error) { return "", "", errors.New("no seed") }
		}},
		{"FailingClone", func(d *SandboxDeps) {
			d.Clone = func(string, string, string) (string, error) { return "", errors.New("no clone") }
		}},
		{"FailingCommitOverride", func(d *SandboxDeps) { d.CommitOverride = func(string) error { return errors.New("no commit") } }},
		{"FailingDeploy", func(d *SandboxDeps) {
			d.Revision = func(string) (buildvcs.Identity, error) { return buildvcs.Identity{}, nil }
			d.Deploy = func() error { return errors.New("no deploy") }
		}},
	}
	for _, tt := range failureTests {
		t.Run(tt.name+"RecordsNoHubAndLeavesNoContainer", func(t *testing.T) {
			t.Parallel()
			root := SandboxRoot(t.TempDir())
			var deployed int
			deps := fakeSandboxDeps(root, &deployed)
			tt.mutate(&deps)

			if _, err := NewSandbox(deps); err == nil {
				t.Fatal("NewSandbox() error = nil; want the seam's failure")
			}
			// The root is read before the listing, which creates the hub list's lock file there.
			if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
				t.Errorf("the root holds %v, %v; want it empty", entries, err)
			}
			if listed, err := ListSandbox(root); err != nil || len(listed) != 0 {
				t.Errorf("ListSandbox() = %v, %v; want no hub", listed, err)
			}
		})
	}
}
