// create.go is the "lyx beam create" sequence: it judges the named board entry, then claims the root, records the owner and the beam flag, and starts the beam's pawl run as a detached first pass, each step skipped when its result is already there.

package beamcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// createDeps holds the seam of every create step, which hubCreateDeps fills over the hub and TestCreate fakes.
// Acting names the worktree the verb runs from, and PrimeName the hub's prime.
// ParentOf names the slug of the pair a slug was forked from, "" for the prime.
// ParentBeam names the beam a board entry belongs to over its project links, "" for the prime.
// PairPresent reports whether this machine holds the slug's pair worktree.
// BranchOnOrigin reports whether the slug's task branch is on origin.
// RunStatus reads the state of the slug's batten run, found false when it has no status file.
// StartRun starts the slug's batten run as a detached first pass recording anchor, and returns the envelope of that start.
type createDeps struct {
	Acting         string
	PrimeName      string
	Machine        func() (string, error)
	ReadOwners     func() (boardengine.Owners, error)
	ParentOf       func(slug string) (string, error)
	ParentBeam     func(slug string) (string, error)
	PairPresent    func(slug string) bool
	BranchOnOrigin func(slug string) (bool, error)
	SetOwners      func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error)
	RunStatus      func(slug string) (state shedengine.State, found bool, err error)
	StartRun       func(slug, anchor string) (map[string]any, error)
	WorktreePath   func(slug string) string
}

// createResult is the verb's envelope: what each step did, the beam's worktree, the first pass's envelope and the next step.
type createResult struct {
	Slug          string         `json:"slug"`
	Worktree      string         `json:"worktree"`
	RootClaimed   bool           `json:"root_claimed,omitempty"`
	OwnerRecorded bool           `json:"owner_recorded,omitempty"`
	RunStarted    bool           `json:"run_started,omitempty"`
	RunLive       bool           `json:"run_live,omitempty"`
	FirstPass     map[string]any `json:"first_pass,omitempty"`
	WayForward    string         `json:"way_forward,omitempty"`
}

// create makes the project entry slug a beam on this machine and starts its pawl run, and returns what each step did.
// A non-empty anchor is the directory the beam's pair takes as its anchor, recorded in the run's seed; empty keeps its parent's.
// A refusal before any step runs returns the error with the result gathered so far.
func create(deps createDeps, slug, anchor string) (createResult, error) {
	result := createResult{Slug: slug, Worktree: deps.WorktreePath(slug)}

	if err := refuseCreateHere(deps, deps.Acting, slug); err != nil {
		return result, err
	}
	here, err := deps.Machine()
	if err != nil {
		return result, fmt.Errorf("lyx beam create: read this machine's name: %w", err)
	}
	owners, err := deps.ReadOwners()
	if err != nil {
		return result, fmt.Errorf("lyx beam create: read the board's owners: %w", err)
	}
	if err := judgeCreateEntry(deps, owners, here, slug); err != nil {
		return result, err
	}
	return result, createSteps(deps, owners, here, slug, anchor, &result)
}

// refuseCreateHere refuses a create run from any worktree but the beam's parent: the prime, or the parent beam ParentBeam names.
// The refusal names that worktree's path and the same command there, or, when the parent beam's pair is absent on this machine, the command that brings the pair first: its create, or its adopt when its branch is on origin.
func refuseCreateHere(deps createDeps, acting, slug string) error {
	parent, err := deps.ParentBeam(slug)
	if err != nil {
		return fmt.Errorf("lyx beam create: read the beam %s belongs to: %w; way forward: fix the cause it names, then run `lyx beam create %s` again", slug, err, slug)
	}
	want := deps.PrimeName
	if parent != "" {
		want = parent
	}
	if acting == want {
		return nil
	}
	if parent != "" && !deps.PairPresent(parent) {
		onOrigin, err := deps.BranchOnOrigin(parent)
		if err != nil {
			return fmt.Errorf("lyx beam create: read whether the branch of the parent beam %s is on origin: %w; way forward: fix the cause it names, then run `lyx beam create %s` again", parent, err, slug)
		}
		command := "lyx beam create " + parent
		if onOrigin {
			command = "lyx adopt " + parent
		}
		return fmt.Errorf("lyx beam create: %s belongs to the beam %s, whose pair is not on this machine; way forward: run `%s`, then run `lyx beam create %s` in %s", slug, parent, command, slug, deps.WorktreePath(parent))
	}
	return fmt.Errorf("lyx beam create: %s is created from its parent's worktree, %s, and not from %s; way forward: run `lyx beam create %s` in %s", slug, deps.WorktreePath(want), acting, slug, deps.WorktreePath(want))
}

// judgeCreateEntry refuses the entry slug unless it is an open project, whose beam is not another machine's and whose parent is not owned by another machine.
func judgeCreateEntry(deps createDeps, owners boardengine.Owners, here, slug string) error {
	entry, held := owners.Entries[slug]
	switch {
	case !held || (!entry.IsTask && !entry.IsProject):
		return fmt.Errorf("lyx beam create: %q is not a project entry, as an absent entry or a note is not; way forward: make one with `lyx board upsert` as a project under a slug of its own, then run `lyx beam create` on that slug", slug)
	case entry.IsTask:
		return fmt.Errorf("lyx beam create: %q is a task, and a beam is made from a project entry; way forward: make a project entry with `lyx board upsert` under a slug of its own, then run `lyx beam create <that slug>`", slug)
	case entry.Done:
		return fmt.Errorf("lyx beam create: the project %q is done and already landed; way forward: make a new project entry with `lyx board upsert` under a slug of its own, then run `lyx beam create` on it", slug)
	}
	if entry.Beam && boardengine.OwnedElsewhere(entry.Owner, here) {
		return fmt.Errorf("lyx beam create: the beam %s is owned by machine %s; way forward: run `lyx beam create %s` on %s, or run `lyx adopt %s` here to take the beam over and then `lyx beam create %s`", slug, entry.Owner, slug, entry.Owner, slug, slug)
	}
	return judgeParentOwner(deps, owners, here, slug)
}

// judgeParentOwner refuses the entry slug when its parent reads as owned by another machine, naming the adopt of the parent's subject: the nearest beam above it, else `root`.
func judgeParentOwner(deps createDeps, owners boardengine.Owners, here, slug string) error {
	parent, err := deps.ParentOf(slug)
	if err != nil {
		return fmt.Errorf("lyx beam create: read the parent of %s: %w", slug, err)
	}
	owner, subject := owners.Prime, boardengine.RootSubject
	if parent != "" {
		if owner, err = owners.Effective(parent, deps.ParentOf); err != nil {
			return fmt.Errorf("lyx beam create: read the owner of %s: %w", parent, err)
		}
		if subject, err = owners.AdoptSubject(parent, deps.ParentOf); err != nil {
			return fmt.Errorf("lyx beam create: read what adopting %s takes: %w", parent, err)
		}
	}
	if boardengine.OwnedElsewhere(owner, here) {
		return fmt.Errorf("lyx beam create: the parent of %s is owned by machine %s, which owns whatever is made under it; way forward: run `lyx adopt %s` here to take it over, then `lyx beam create %s`", slug, owner, subject, slug)
	}
	return nil
}

// createSteps runs the three steps in order, each skipped when its result stands, and records what it did on result.
// The root is claimed when the board records no root owner; a claim another machine wins is judged again, so a parent that now reads as theirs refuses.
// The owner step records this machine as the beam's owner with the beam flag and the pawl recipe in one write.
// The run is the last step, so a beam's pair is made by the run's own Worktree-Create and marked by its Beam-Up.
func createSteps(deps createDeps, owners boardengine.Owners, here, slug, anchor string, result *createResult) error {
	rerun := "run `lyx beam create " + slug + "` again"

	if owners.Prime == "" {
		write, err := deps.SetOwners(boardengine.OwnerRequest{Machine: here, Prime: true})
		if err != nil {
			return fmt.Errorf("lyx beam create: record this machine as the hub's prime owner: %w; way forward: fix the cause it names, then %s", err, rerun)
		}
		if _, lost := write.Lost[boardengine.OwnerWritePrime]; lost {
			if owners, err = deps.ReadOwners(); err != nil {
				return fmt.Errorf("lyx beam create: read the board's owners: %w", err)
			}
			if err := judgeParentOwner(deps, owners, here, slug); err != nil {
				return err
			}
		} else {
			result.RootClaimed = true
		}
	}

	entry := owners.Entries[slug]
	if !entry.Beam || entry.Owner != here {
		write, err := deps.SetOwners(boardengine.OwnerRequest{Machine: here, Slugs: []string{slug}, Beam: true, ParentOf: deps.ParentOf})
		if err != nil {
			return fmt.Errorf("lyx beam create: record this machine as the owner of %s with the beam flag: %w; way forward: fix the cause it names, then %s", slug, err, rerun)
		}
		if owner, lost := write.Lost[slug]; lost {
			return fmt.Errorf("lyx beam create: another machine recorded %s as the owner of %s while this write was in flight; way forward: run `lyx adopt %s` to take the beam over, then `lyx beam create %s`", owner, slug, slug, slug)
		}
		result.OwnerRecorded = true
	}

	state, found, err := deps.RunStatus(slug)
	if err != nil {
		return fmt.Errorf("lyx beam create: read the pawl run of %s: %w; way forward: fix the cause it names, then %s", slug, err, rerun)
	}
	if found && state != shedengine.StateDone {
		result.RunLive = true
	} else {
		envelope, err := deps.StartRun(slug, anchor)
		if err != nil {
			return fmt.Errorf("lyx beam create: start the pawl run of %s: %w; way forward: fix the cause it names, then %s", slug, err, rerun)
		}
		result.FirstPass = envelope
		if ok, _ := envelope["ok"].(bool); !ok {
			return fmt.Errorf("lyx beam create: the first pass of %s did not start: %v; way forward: fix the cause it names, then %s", slug, envelope["error"], rerun)
		}
		result.RunStarted = true
	}
	result.WayForward = fmt.Sprintf("run `lyx batten status %s` to follow the run", slug)
	return nil
}
