// land.go is the "lyx beam land" sequence: it judges the named beam, applies the refusal that stops a landing while work remains under it, and starts the landing run in a window.

package beamcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// landDeps holds the seam of every land step, which hubLandDeps fills over the hub and TestLand fakes.
// ParentWorktree names the worktree the beam's pair was forked from, the prime's name when its record names none.
// Refusal is the landing's refusal of a beam with work remaining under it.
// StartLanding runs "lyx batten run <slug> --window" and returns the envelope that verb prints.
type landDeps struct {
	ReadOwners     func() (boardengine.Owners, error)
	PairPresent    func(slug string) bool
	OriginRecord   func(slug string) (fabricengine.Origin, bool, error)
	ParentWorktree func(slug string) (string, error)
	Refusal        func(slug string) error
	StartLanding   func(slug string) (map[string]any, error)
}

// land starts the landing of the beam slug from the worktree named acting, and returns the envelope of the landing run's start.
// The beam's pair is removed by the landing, so the verb refuses to run from inside the beam itself.
func land(deps landDeps, acting, slug string) (map[string]any, error) {
	if acting == slug {
		parent, err := deps.ParentWorktree(slug)
		if err != nil {
			return nil, fmt.Errorf("lyx beam land: read the parent worktree of %s: %w", slug, err)
		}
		return nil, fmt.Errorf("lyx beam land: %s is the beam itself, and its landing removes the pair this verb runs in; way forward: run `lyx beam land %s` from its parent worktree %s", slug, slug, parent)
	}

	owners, err := deps.ReadOwners()
	if err != nil {
		return nil, fmt.Errorf("lyx beam land: read the board's owners: %w", err)
	}
	if err := judgeLandEntry(deps, owners, slug); err != nil {
		return nil, err
	}
	if err := deps.Refusal(slug); err != nil {
		return nil, err
	}

	envelope, err := deps.StartLanding(slug)
	if err != nil {
		return nil, fmt.Errorf("lyx beam land: start the landing run of %s: %w; way forward: fix the cause it names, then run `lyx beam land %s` again", slug, err, slug)
	}
	if ok, _ := envelope["ok"].(bool); !ok {
		return envelope, fmt.Errorf("lyx beam land: the landing run of %s did not start: %v; way forward: fix the cause it names, then run `lyx beam land %s` again", slug, envelope["error"], slug)
	}
	return envelope, nil
}

// judgeLandEntry refuses the entry slug unless it is a beam project that is not done and whose pair is here, marked.
func judgeLandEntry(deps landDeps, owners boardengine.Owners, slug string) error {
	entry, held := owners.Entries[slug]
	switch {
	case !held || (!entry.IsTask && !entry.IsProject):
		return fmt.Errorf("lyx beam land: %q is not a beam, as an absent entry or a note is not; way forward: `lyx board list` shows the entries", slug)
	case entry.IsTask:
		return fmt.Errorf("lyx beam land: %q is a task, not a beam; way forward: land a task with `lyx batten run %s`, or make it a beam with `lyx beam promote %s`", slug, slug, slug)
	case entry.Done:
		return fmt.Errorf("lyx beam land: the beam %q is done and already landed", slug)
	case !entry.Beam:
		return fmt.Errorf("lyx beam land: %q is a grouping project, not a beam; way forward: run `lyx beam create %s` to make it a beam", slug, slug)
	case !deps.PairPresent(slug):
		return fmt.Errorf("lyx beam land: the beam %s has no pair on this machine; way forward: run `lyx beam create %s` to finish the create, or `lyx adopt %s` when the beam's branch is on origin", slug, slug, slug)
	}
	origin, found, err := deps.OriginRecord(slug)
	if err != nil {
		return fmt.Errorf("lyx beam land: read the origin record of %s: %w", slug, err)
	}
	if !found {
		return fmt.Errorf("lyx beam land: the pair of the beam %s has no readable origin record; way forward: run `lyx fabric reconcile` to restore its records worktree, then `lyx beam land %s`", slug, slug)
	}
	if !origin.Beam {
		return fmt.Errorf("lyx beam land: the pair of %s does not carry the beam mark; way forward: run `lyx beam create %s`, which marks it", slug, slug)
	}
	return nil
}
