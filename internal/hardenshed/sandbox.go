// sandbox.go builds and lists the throwaway sandbox hubs a campaign's live tests run against.

package hardenshed

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// hubListFileName is the file under the sandbox root that records the hubs made there.
const hubListFileName = "hubs.json"

// hubListLockSuffix names the lock of the hub list beside it, under the root.
const hubListLockSuffix = ".lock"

// SandboxRoot returns the directory a run's sandbox hubs are made under.
// It lies below the told worktree's never-tracked directory.
// Every process of a run derives the same root, and the root goes with the worktree.
func SandboxRoot(worktreeRoot string) string {
	return filepath.Join(worktreeRoot, lymdirs.DotLymDirName, lymdirs.HardenSandboxDirName)
}

// SandboxDeps is what NewSandbox needs told.
type SandboxDeps struct {
	// Root is the sandbox root; NewSandbox writes only under it.
	Root string
	// Seed creates the code bare and the records bare a hub is cloned from under dir.
	Seed func(dir string) (codeBare, recordsBare string, err error)
	// Clone clones a hub from the two bares into container, with no GitHub remote, and returns the hub's path.
	Clone func(codeBare, recordsBare, container string) (hubPath string, err error)
	// CommitOverride commits the hub's sandbox config onto its prime records before any pair exists.
	CommitOverride func(hubPath string) error
	// DevBinLym is the absolute path of the worktree's `.dev-bin` lym, the binary the sandbox hubs run.
	DevBinLym string
	// Head returns the revision of the worktree's HEAD.
	Head func() (string, error)
	// Revision returns the build identity of the binary at path.
	Revision func(path string) (buildvcs.Identity, error)
	// Deploy rebuilds the `.dev-bin` lym from the worktree.
	Deploy func() error
}

// SandboxHub is a hub NewSandbox made and the lym binary to drive it with.
type SandboxHub struct {
	// Hub is the absolute path of the sandbox hub.
	Hub string
	// Lym is the absolute path of the `.dev-bin` lym that runs in it.
	Lym string
}

// hubList is the sandbox root's record of the hubs made under it.
type hubList struct {
	Hubs []string `json:"hubs"`
}

// hubListPaths returns the hub list file under root and the lock that guards it.
// The lock sits under root too, so a sandbox writes nothing outside its root.
func hubListPaths(root string) (path, lockPath string) {
	path = filepath.Join(root, hubListFileName)
	return path, path + hubListLockSuffix
}

// NewSandbox makes a fresh throwaway hub under deps.Root, records it in the root's hub list, and returns it with its lym.
// It seeds the two bares and clones the hub in a container of its own, then has the sandbox config committed.
// It rebuilds the `.dev-bin` lym only when its revision is not the worktree HEAD's or it is marked modified.
// The hub list is written under a lock, so another process lists the hub.
// A failure after the container is made removes the container, the seeded bares included, before the error returns.
func NewSandbox(deps SandboxDeps) (SandboxHub, error) {
	if err := os.MkdirAll(deps.Root, 0o755); err != nil {
		return SandboxHub{}, fmt.Errorf("create the sandbox root: %w", err)
	}
	container, err := os.MkdirTemp(deps.Root, "hub-*")
	if err != nil {
		return SandboxHub{}, fmt.Errorf("create a sandbox hub container: %w", err)
	}
	hub, err := buildSandboxHub(deps, container)
	if err != nil {
		if removeErr := os.RemoveAll(container); removeErr != nil {
			return SandboxHub{}, fmt.Errorf("%w; the container %s could not be removed: %v", err, container, removeErr)
		}
		return SandboxHub{}, err
	}
	return hub, nil
}

// buildSandboxHub seeds, clones and records the hub inside container, and settles the lym that runs it.
func buildSandboxHub(deps SandboxDeps, container string) (SandboxHub, error) {
	codeBare, recordsBare, err := deps.Seed(container)
	if err != nil {
		return SandboxHub{}, fmt.Errorf("seed the sandbox bares: %w", err)
	}
	hubPath, err := deps.Clone(codeBare, recordsBare, container)
	if err != nil {
		return SandboxHub{}, fmt.Errorf("clone the sandbox hub: %w", err)
	}
	if err := deps.CommitOverride(hubPath); err != nil {
		return SandboxHub{}, fmt.Errorf("commit the sandbox config override: %w", err)
	}
	if err := deployWhenStale(deps); err != nil {
		return SandboxHub{}, err
	}
	path, lockPath := hubListPaths(deps.Root)
	err = state.UpdateJSON(path, lockPath, func(cur hubList, _ bool) (hubList, error) {
		cur.Hubs = append(cur.Hubs, hubPath)
		return cur, nil
	})
	if err != nil {
		return SandboxHub{}, fmt.Errorf("record the sandbox hub: %w", err)
	}
	return SandboxHub{Hub: hubPath, Lym: deps.DevBinLym}, nil
}

// deployWhenStale rebuilds the `.dev-bin` lym unless it was built from the worktree's HEAD and is unmodified.
func deployWhenStale(deps SandboxDeps) error {
	head, err := deps.Head()
	if err != nil {
		return fmt.Errorf("read the worktree HEAD: %w", err)
	}
	built, err := deps.Revision(deps.DevBinLym)
	if err != nil {
		return fmt.Errorf("read the revision of %s: %w", deps.DevBinLym, err)
	}
	if built.Revision == head && !built.Modified {
		return nil
	}
	if err := deps.Deploy(); err != nil {
		return fmt.Errorf("deploy the .dev-bin lym: %w", err)
	}
	return nil
}

// ListSandbox returns the hubs recorded under root, in the order they were made.
// It reads the hub list alone, so an entry outlives its directory until a reap prunes it.
func ListSandbox(root string) ([]string, error) {
	path, lockPath := hubListPaths(root)
	list, _, err := state.ReadJSON[hubList](path, lockPath)
	if err != nil {
		return nil, fmt.Errorf("read the sandbox hub list: %w", err)
	}
	return list.Hubs, nil
}
