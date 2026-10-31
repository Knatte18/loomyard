// review.go holds the target resolver the slug-taking loom groups share.
//
// Each such group carries its own PersistentPreRunE, and the loom parent's pre-run skips the group, so arm never runs for it and never reads a slug as a run-id.
// The skip is what holds under cmd/lym's cobra.EnableTraverseRunHooks, which runs every ancestor's pre-run.
// circling.go, decision.go and peers.go point at this file for that reason.

package loomcli

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const reviewWaySlugForm = `pass the task's slug as listed by "lym board list"`

// reviewTargetDeps is every side effect resolveReviewTarget performs, injected so tests supply fakes.
type reviewTargetDeps struct {
	// primeName resolves the base name of the hub's main worktree.
	primeName func(*lymcwd.Location) (string, error)
	// dirExists reports whether a directory exists on disk.
	dirExists func(path string) bool
	// resolveWorktree resolves a worktree root into a Location with no cwd gate.
	resolveWorktree func(worktreeRoot string) (*lymcwd.Location, error)
	// seedRecipe returns the recipe name the target worktree's self run records, tapestry for a run with no seed.
	// A nil seam reads the target as tapestry.
	seedRecipe func(target *lymcwd.Location) (string, error)
}

// defaultReviewTargetDeps are the production seams.
func defaultReviewTargetDeps() reviewTargetDeps {
	return reviewTargetDeps{
		primeName: fabricengine.PrimeName,
		dirExists: func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && info.IsDir()
		},
		resolveWorktree: lymcwd.ResolveWorktree,
		seedRecipe:      seedRecipe,
	}
}

// resolveReviewTarget returns the worktree a verb of the named group (`circling` or `decision`) acts on.
// A given slug resolves to its sibling worktree of the hub; with no slug a task worktree addresses itself and the prime refuses.
// A target whose run records a recipe `lym loom` does not serve is refused, since it has no decision record or review perch the verb acts on.
func resolveReviewTarget(group, verb string, location *lymcwd.Location, slug string, d reviewTargetDeps) (*lymcwd.Location, error) {
	target, err := locateReviewTarget(group, verb, location, slug, d)
	if err != nil {
		return nil, err
	}
	if d.seedRecipe == nil {
		return target, nil
	}
	recipe, err := d.seedRecipe(target)
	if err != nil {
		return nil, fmt.Errorf("loom: %s %s: read the run's seed: %w; way forward: restore the run's seed, or begin the run again with `lym loom start`", group, verb, err)
	}
	switch serviceOfRecipe(recipe) {
	case recipeNotLoom:
		return nil, fmt.Errorf("loom: %s %s: run recipe %q is not a loom recipe, so it has no decision record or review perch this verb acts on; way forward: %s", group, verb, recipe, shedVerbsWayForward)
	case recipeUnknown:
		return nil, fmt.Errorf("loom: %s %s: run recipe %q names no recipe this lym knows; way forward: %s", group, verb, recipe, reseedWayForward)
	}
	return target, nil
}

// locateReviewTarget resolves the worktree a verb of the named group acts on, before the recipe of its run is judged.
func locateReviewTarget(group, verb string, location *lymcwd.Location, slug string, d reviewTargetDeps) (*lymcwd.Location, error) {
	if slug == "" {
		prime, err := d.primeName(location)
		if err != nil {
			return nil, fmt.Errorf("loom: %s %s: %w", group, verb, err)
		}
		if location.WorktreeName == prime {
			return nil, fmt.Errorf("loom: %s %s: slug required from the prime; way forward: %s, e.g. \"lym loom %s %s <slug>\"", group, verb, reviewWaySlugForm, group, verb)
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
