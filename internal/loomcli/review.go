// review.go holds the target resolver the slug-taking loom groups share.
//
// Each such group carries its own PersistentPreRunE, and the loom parent's pre-run skips the group, so arm never runs for it and never reads a slug as a run-id.
// The skip is what holds under cmd/lyx's cobra.EnableTraverseRunHooks, which runs every ancestor's pre-run.
// circling.go, decision.go and peers.go point at this file for that reason.

package loomcli

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const reviewWaySlugForm = `pass the task's slug as listed by "lyx board list"`

// reviewTargetDeps is every side effect resolveReviewTarget performs, injected so tests supply fakes.
type reviewTargetDeps struct {
	// primeName resolves the base name of the hub's main worktree.
	primeName func(*lyxcwd.Location) (string, error)
	// dirExists reports whether a directory exists on disk.
	dirExists func(path string) bool
	// resolveWorktree resolves a worktree root into a Location with no cwd gate.
	resolveWorktree func(worktreeRoot string) (*lyxcwd.Location, error)
}

// defaultReviewTargetDeps are the production seams.
func defaultReviewTargetDeps() reviewTargetDeps {
	return reviewTargetDeps{
		primeName: fabricengine.PrimeName,
		dirExists: func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && info.IsDir()
		},
		resolveWorktree: lyxcwd.ResolveWorktree,
	}
}

// resolveReviewTarget returns the worktree a verb of the named group (`circling` or `decision`) acts on.
// A given slug resolves to its sibling worktree of the hub; with no slug a task worktree addresses itself and the prime refuses.
func resolveReviewTarget(group, verb string, location *lyxcwd.Location, slug string, d reviewTargetDeps) (*lyxcwd.Location, error) {
	if slug == "" {
		prime, err := d.primeName(location)
		if err != nil {
			return nil, fmt.Errorf("loom: %s %s: %w", group, verb, err)
		}
		if location.WorktreeName == prime {
			return nil, fmt.Errorf("loom: %s %s: slug required from the prime; way forward: %s, e.g. \"lyx loom %s %s <slug>\"", group, verb, reviewWaySlugForm, group, verb)
		}
		return location, nil
	}
	root := fabricengine.WorktreePath(location, slug)
	if !d.dirExists(root) {
		return nil, fmt.Errorf("loom: %s %s: unknown slug %q: no worktree at %s; way forward: %s", group, verb, slug, root, reviewWaySlugForm)
	}
	target, err := d.resolveWorktree(root)
	if err != nil {
		return nil, fmt.Errorf("loom: %s %s: resolve worktree %q: %w", group, verb, slug, err)
	}
	return target, nil
}

// reviewStatusReader reads the target worktree's default run status; found is false when the run has no status file.
type reviewStatusReader func() (shedengine.Status, bool, error)
