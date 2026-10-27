// land.go is the "lym beam land" sequence: it judges the named beam, applies the refusal that stops a landing while work remains under it, and writes the land request the beam's pawl run consumes at Beam-Await.

package beamcli

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// landDeps holds the seam of every land step, which hubLandDeps fills over the hub and TestLand fakes.
// ParentOf names the slug of the pair a slug was forked from, "" for the prime.
// ChildStatus reads the status of the beam pair's own pawl run, found false when it has none.
// RunID returns the seed id of that pawl run, which a land request names.
// Refusal is the landing's refusal of a beam with work remaining under it.
// WriteRequest writes the land request into the beam worktree and returns its path.
type landDeps struct {
	Machine      func() (string, error)
	ReadOwners   func() (boardengine.Owners, error)
	ParentOf     func(slug string) (string, error)
	PairPresent  func(slug string) bool
	OriginRecord func(slug string) (fabricengine.Origin, bool, error)
	ChildStatus  func(slug string) (status shedengine.Status, found bool, err error)
	RunID        func(slug string) (string, error)
	Refusal      func(slug string) error
	WriteRequest func(slug string, req pawlengine.LandRequest) (string, error)
	Now          func() time.Time
}

// land asks the pawl run of the beam slug to land the beam, and returns the request's path and the command that follows the landing.
// The request is only a file: Beam-Await consumes it, and the beam's pair is removed at the run's teardown, so the verb runs from any worktree, the beam itself included.
func land(deps landDeps, slug string) (map[string]any, error) {
	here, err := deps.Machine()
	if err != nil {
		return nil, fmt.Errorf("lym beam land: read this machine's name: %w", err)
	}
	owners, err := deps.ReadOwners()
	if err != nil {
		return nil, fmt.Errorf("lym beam land: read the board's owners: %w", err)
	}
	if err := judgeLandEntry(deps, owners, slug); err != nil {
		return nil, err
	}
	if err := judgeLandRun(deps, slug); err != nil {
		return nil, err
	}
	if err := judgeLandOwners(deps, owners, here, slug); err != nil {
		return nil, err
	}
	if err := deps.Refusal(slug); err != nil {
		return nil, err
	}

	runID, err := deps.RunID(slug)
	if err != nil {
		return nil, fmt.Errorf("lym beam land: read the pawl run id of %s: %w; way forward: fix the cause it names, then run `lym beam land %s` again", slug, err, slug)
	}
	path, err := deps.WriteRequest(slug, pawlengine.LandRequest{RunID: runID, RequestedAt: deps.Now().UTC()})
	if err != nil {
		return nil, fmt.Errorf("lym beam land: write the land request of %s: %w; way forward: fix the cause it names, then run `lym beam land %s` again", slug, err, slug)
	}
	return map[string]any{
		"slug":        slug,
		"request":     path,
		"run_id":      runID,
		"way_forward": fmt.Sprintf("run `lym batten status %s` to follow the landing", slug),
	}, nil
}

// judgeLandEntry refuses the entry slug unless it is a beam project that is not done and whose pair is here, marked.
func judgeLandEntry(deps landDeps, owners boardengine.Owners, slug string) error {
	entry, held := owners.Entries[slug]
	switch {
	case !held || (!entry.IsTask && !entry.IsProject):
		return fmt.Errorf("lym beam land: %q is not a beam, as an absent entry or a note is not; way forward: `lym board list` shows the entries", slug)
	case entry.IsTask:
		return fmt.Errorf("lym beam land: %q is a task, not a beam; way forward: land a task with `lym batten run %s`, or make a project entry with `lym board upsert` under a slug of its own, then run `lym beam create <that slug>`", slug, slug)
	case entry.Done:
		return fmt.Errorf("lym beam land: the beam %q is done and already landed", slug)
	case !entry.Beam:
		return fmt.Errorf("lym beam land: %q is a grouping project, not a beam; way forward: run `lym beam create %s` to make it a beam", slug, slug)
	case !deps.PairPresent(slug):
		return fmt.Errorf("lym beam land: the beam %s has no pair on this machine; way forward: run `lym beam create %s` to finish the create, or `lym adopt %s` when the beam's branch is on origin", slug, slug, slug)
	}
	origin, found, err := deps.OriginRecord(slug)
	if err != nil {
		return fmt.Errorf("lym beam land: read the origin record of %s: %w", slug, err)
	}
	if !found {
		return fmt.Errorf("lym beam land: the pair of the beam %s has no readable origin record; way forward: run `lym fabric reconcile` to restore its records worktree, then `lym beam land %s`", slug, slug)
	}
	if !origin.Beam {
		return fmt.Errorf("lym beam land: the pair of %s does not carry the beam mark; way forward: run `lym beam create %s`, which marks it", slug, slug)
	}
	return nil
}

// judgeLandRun refuses a beam whose pawl run is absent, as a beam made before the pawl recipe is, or is not running at Beam-Await, the one row that reads a land request.
func judgeLandRun(deps landDeps, slug string) error {
	status, found, err := deps.ChildStatus(slug)
	if err != nil {
		return fmt.Errorf("lym beam land: read the pawl run of %s: %w; way forward: fix the cause it names, then run `lym beam land %s` again", slug, err, slug)
	}
	if !found {
		return fmt.Errorf("lym beam land: the beam %s has no pawl run on this machine; way forward: run `lym beam create %s`, which starts it", slug, slug)
	}
	if status.State != shedengine.StateRunning || status.CurrentProducer != pawlengine.NameBeamAwait {
		return fmt.Errorf("lym beam land: the pawl run of %s is %s at %s, and only a run waiting at %s reads a land request; way forward: run `lym batten status %s`, then `lym beam land %s` once the run waits", slug, status.State, status.CurrentProducer, pawlengine.NameBeamAwait, slug, slug)
	}
	return nil
}

// judgeLandOwners refuses a beam another machine owns and a beam whose parent another machine owns, each naming the adopt that takes it.
// A beam with no owner recorded reads its parent's, so only the parent check judges it.
func judgeLandOwners(deps landDeps, owners boardengine.Owners, here, slug string) error {
	if owner := owners.Entries[slug].Owner; boardengine.OwnedElsewhere(owner, here) {
		return fmt.Errorf("lym beam land: the beam %s is owned by machine %s; way forward: run `lym beam land %s` on %s, or run `lym adopt %s` here to take the beam over", slug, owner, slug, owner, slug)
	}
	parent, err := deps.ParentOf(slug)
	if err != nil {
		return fmt.Errorf("lym beam land: read the parent of %s: %w", slug, err)
	}
	owner, subject := owners.Prime, boardengine.RootSubject
	if parent != "" {
		if owner, err = owners.Effective(parent, deps.ParentOf); err != nil {
			return fmt.Errorf("lym beam land: read the owner of %s: %w", parent, err)
		}
		if subject, err = owners.AdoptSubject(parent, deps.ParentOf); err != nil {
			return fmt.Errorf("lym beam land: read what adopting %s takes: %w", parent, err)
		}
	}
	if boardengine.OwnedElsewhere(owner, here) {
		return fmt.Errorf("lym beam land: the parent of %s is owned by machine %s, and a landing merges into it; way forward: run `lym adopt %s` here to take it over, then `lym beam land %s`", slug, owner, subject, slug)
	}
	return nil
}
