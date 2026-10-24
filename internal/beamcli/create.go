// create.go is the "lyx beam create" sequence: it judges the named board entry, then records the owner and the beam flag, makes the beam's pair, spawns its push and regenerates the workspace files, each step skipped when its result is already there.

package beamcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// createDeps holds the seam of every create step, which hubCreateDeps fills over the hub and TestCreate fakes.
// ParentOf names the slug of the pair a slug was forked from, "" for the prime.
// ParentBeam names the beam a board entry belongs to over its project links, "" for the prime.
// PairPresent reports whether this machine holds the slug's pair worktree.
// BranchOnOrigin reports whether the slug's task branch is on origin.
// HubPath roots the mutation record.
type createDeps struct {
	Machine             func() (string, error)
	ReadOwners          func() (boardengine.Owners, error)
	ParentOf            func(slug string) (string, error)
	ParentBeam          func(slug string) (string, error)
	PairPresent         func(slug string) bool
	BranchOnOrigin      func(slug string) (bool, error)
	SetOwners           func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error)
	AddPair             func(slug string, opts fabricengine.AddOptions) (fabricengine.AddResult, error)
	MarkBeam            func(rec *fabricengine.Mutations, slug string) (bool, error)
	SpawnPush           func(slug string) error
	RegenerateWorkspace func(rec *fabricengine.Mutations) (string, error)
	WorktreePath        func(slug string) string
	HubPath             string
}

// createResult is the verb's envelope: what each step did, the beam's worktree, the next step and the mutation record of the fabric calls.
type createResult struct {
	Slug          string                  `json:"slug"`
	Worktree      string                  `json:"worktree"`
	OwnerRecorded bool                    `json:"owner_recorded,omitempty"`
	PairAdded     bool                    `json:"pair_added,omitempty"`
	BeamMarked    bool                    `json:"beam_marked,omitempty"`
	PushSpawned   bool                    `json:"push_spawned,omitempty"`
	Workspace     string                  `json:"workspace,omitempty"`
	WayForward    string                  `json:"way_forward,omitempty"`
	Mutations     []fabricengine.Mutation `json:"mutations,omitempty"`
	Partial       bool                    `json:"partial,omitempty"`
}

// create makes the project entry slug a beam on this machine, and returns what each step did.
// A refusal before any step runs returns the error with the result gathered so far.
// The push is the one step a re-run repeats, since the detached push carries whatever is unpushed.
func create(deps createDeps, slug string) (createResult, error) {
	result := createResult{Slug: slug, Worktree: deps.WorktreePath(slug)}

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

	rec := fabricengine.NewMutations(deps.HubPath)
	finish := func() createResult {
		result.Mutations = rec.Entries()
		result.Partial = rec.Len() > 0
		return result
	}
	if err := createSteps(deps, rec, owners.Entries[slug], here, slug, &result); err != nil {
		return finish(), err
	}
	return finish(), nil
}

// judgeCreateEntry refuses the entry slug unless it is a project that is not done, whose beam is not another machine's.
func judgeCreateEntry(deps createDeps, owners boardengine.Owners, here, slug string) error {
	entry, held := owners.Entries[slug]
	switch {
	case !held || (!entry.IsTask && !entry.IsProject):
		return fmt.Errorf("lyx beam create: %q is not a project entry, as an absent entry or a note is not; way forward: make one with `lyx board upsert` as a project under a slug of its own, then run `lyx beam create` on that slug", slug)
	case entry.IsTask:
		return fmt.Errorf("lyx beam create: %q is a task, and a beam is made from a project entry; way forward: run `lyx beam promote %s`, which turns the task into a beam and keeps its pair", slug, slug)
	case entry.Done:
		return fmt.Errorf("lyx beam create: the project %q is done and already landed; way forward: make a new project entry with `lyx board upsert` under a slug of its own, then run `lyx beam create` on it", slug)
	case !entry.Beam:
		return nil
	}
	owner, err := owners.Effective(slug, deps.ParentOf)
	if err != nil {
		return fmt.Errorf("lyx beam create: read the owner of %s: %w", slug, err)
	}
	if boardengine.OwnedElsewhere(owner, here) {
		return fmt.Errorf("lyx beam create: the beam %s is owned by machine %s; way forward: run `lyx beam create %s` on %s, or run `lyx adopt %s` here to take the beam over and then `lyx beam create %s`", slug, owner, slug, owner, slug, slug)
	}
	return nil
}

// createSteps runs the four steps in order, each skipped when its result stands, and records what it did on result.
func createSteps(deps createDeps, rec *fabricengine.Mutations, entry boardengine.OwnerEntry, here, slug string, result *createResult) error {
	rerun := "run `lyx beam create " + slug + "` again"

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

	if deps.PairPresent(slug) {
		marked, err := deps.MarkBeam(rec, slug)
		if err != nil {
			return fmt.Errorf("lyx beam create: mark the pair of %s as a beam: %w; way forward: fix the cause it names, then %s", slug, err, rerun)
		}
		result.BeamMarked = marked
	} else {
		parent, err := deps.ParentBeam(slug)
		if err != nil {
			return fmt.Errorf("lyx beam create: read the beam %s belongs to: %w; way forward: fix the cause it names, then %s", slug, err, rerun)
		}
		if err := requireParentBeamPair(deps, slug, parent); err != nil {
			return err
		}
		added, err := deps.AddPair(slug, fabricengine.AddOptions{Parent: parent, Beam: true})
		rec.Extend(added.Mutated())
		if err != nil {
			return fmt.Errorf("lyx beam create: add the pair of %s: %w; way forward: fix the cause it names, then %s", slug, err, rerun)
		}
		result.PairAdded = true
	}

	if err := deps.SpawnPush(slug); err != nil {
		return fmt.Errorf("lyx beam create: spawn the push of %s: %w; way forward: fix the cause it names, then %s", slug, err, rerun)
	}
	result.PushSpawned = true

	workspace, err := deps.RegenerateWorkspace(rec)
	if err != nil {
		return fmt.Errorf("lyx beam create: regenerate the workspace files: %w; way forward: fix the cause it names, then %s", err, rerun)
	}
	result.Workspace = workspace
	result.WayForward = orchWayForward(result.Worktree)
	return nil
}

// requireParentBeamPair refuses a create whose parent beam has no pair on this machine, naming the command that brings it.
// The parent's create is named when its branch is off origin, and its adopt when the branch is on origin.
func requireParentBeamPair(deps createDeps, slug, parent string) error {
	if parent == "" || deps.PairPresent(parent) {
		return nil
	}
	onOrigin, err := deps.BranchOnOrigin(parent)
	if err != nil {
		return fmt.Errorf("lyx beam create: read whether the branch of the parent beam %s is on origin: %w; way forward: fix the cause it names, then run `lyx beam create %s` again", parent, err, slug)
	}
	command := "lyx beam create " + parent
	if onOrigin {
		command = "lyx adopt " + parent
	}
	return fmt.Errorf("lyx beam create: %s belongs to the beam %s, whose pair is not on this machine; way forward: run `%s`, then `lyx beam create %s`", slug, parent, command, slug)
}
