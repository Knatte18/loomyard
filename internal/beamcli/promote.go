// promote.go is the "lyx beam promote" sequence: it judges the named entry, then ends a task's run, marks its pair as a beam, clears the run's records and claim, resets the pair's records to its parent's, turns the entry into a beam project and regenerates the workspace files.

package beamcli

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// promoteDeps holds the seam of every promote step beside create's, which hubPromoteDeps fills over the hub and TestPromote fakes.
// OriginRecord reads the origin record of the slug's pair, false when there is none.
// EndRun ends the slug's batten run and keeps its pair, and ClearRun removes the run's records and clears its claim.
type promoteDeps struct {
	createDeps
	ReadTasks    func() ([]boardengine.Task, error)
	OriginRecord func(slug string) (fabricengine.Origin, bool, error)
	EndRun       func(ctx context.Context, slug string) error
	ClearRun     func(ctx context.Context, slug string) error
	ResetRecords func(rec *fabricengine.Mutations, slug string) (fabricengine.ResetRecordsResult, error)
}

// promoteResult is the verb's envelope: create's fields for a project it makes a beam of, and for a task what each step did.
type promoteResult struct {
	createResult
	RunEnded     bool `json:"run_ended,omitempty"`
	RunCleared   bool `json:"run_cleared,omitempty"`
	RecordsReset bool `json:"records_reset,omitempty"`
	Promoted     bool `json:"promoted,omitempty"`
}

// promote turns the entry slug into a beam on this machine, and returns what each step did.
// A project entry is made a beam by create's steps, or by the workspace step alone when its beam stands with a marked pair.
// A task goes through the six steps in order, each skipped when its result is already there; the end of the run repeats while the claim stands, since ending an ended run changes nothing.
func promote(ctx context.Context, deps promoteDeps, slug string) (promoteResult, error) {
	result := promoteResult{createResult: createResult{Slug: slug, Worktree: deps.WorktreePath(slug)}}

	here, err := deps.Machine()
	if err != nil {
		return result, fmt.Errorf("lyx beam promote: read this machine's name: %w", err)
	}
	owners, err := deps.ReadOwners()
	if err != nil {
		return result, fmt.Errorf("lyx beam promote: read the board's owners: %w", err)
	}
	entry := owners.Entries[slug]

	if entry.IsProject {
		return promoteProject(deps, owners, here, slug, result)
	}
	if err := judgePromoteTask(ctx, deps, owners, here, slug); err != nil {
		return result, err
	}

	rec := fabricengine.NewMutations(deps.HubPath)
	finish := func() promoteResult {
		result.Mutations = rec.Entries()
		result.Partial = rec.Len() > 0
		return result
	}
	if err := promoteSteps(ctx, deps, rec, entry, here, slug, &result); err != nil {
		return finish(), err
	}
	return finish(), nil
}

// promoteProject finishes a promote named at a project entry: the workspace step alone for a beam whose pair is marked, create's steps otherwise.
func promoteProject(deps promoteDeps, owners boardengine.Owners, here, slug string, result promoteResult) (promoteResult, error) {
	if err := judgeCreateEntry(deps.createDeps, owners, here, slug); err != nil {
		return result, err
	}
	if entry := owners.Entries[slug]; entry.Beam {
		origin, found, err := deps.OriginRecord(slug)
		if err != nil {
			return result, fmt.Errorf("lyx beam promote: read the origin record of %s: %w", slug, err)
		}
		if found && origin.Beam {
			rec := fabricengine.NewMutations(deps.HubPath)
			workspace, err := deps.RegenerateWorkspace(rec)
			result.Mutations, result.Partial = rec.Entries(), rec.Len() > 0
			if err != nil {
				return result, fmt.Errorf("lyx beam promote: regenerate the workspace files: %w; way forward: fix the cause it names, then run `lyx beam promote %s` again", err, slug)
			}
			result.Workspace = workspace
			result.Promoted = true
			result.WayForward = orchWayForward(result.Worktree)
			return result, nil
		}
	}
	created, err := create(deps.createDeps, slug)
	result.createResult = created
	return result, err
}

// judgePromoteTask refuses the entry slug unless it is a started task of this machine, with a pair here, that no open task depends on and whose pair has a parent to reset its records to.
func judgePromoteTask(ctx context.Context, deps promoteDeps, owners boardengine.Owners, here, slug string) error {
	entry, held := owners.Entries[slug]
	switch {
	case !held:
		return fmt.Errorf("lyx beam promote: %q is not on the board; way forward: `lyx board list` shows the entries", slug)
	case !entry.IsTask:
		return fmt.Errorf("lyx beam promote: %q is a note, and a beam is made from a project entry; way forward: make one with `lyx board upsert` as a project under a slug of its own, then run `lyx beam create` on it", slug)
	case entry.Done:
		return fmt.Errorf("lyx beam promote: %q is done and already landed; way forward: make a new project entry with `lyx board upsert` under a slug of its own, then run `lyx beam create` on it", slug)
	case entry.Owner == "" && !entry.RunHeld:
		return fmt.Errorf("lyx beam promote: %q was never started, so there is no pair to keep; way forward: make a project entry with `lyx board upsert` under a slug of its own, then run `lyx beam create` on it", slug)
	}
	owner, err := owners.Effective(slug, deps.ParentOf)
	if err != nil {
		return fmt.Errorf("lyx beam promote: read the owner of %s: %w", slug, err)
	}
	if boardengine.OwnedElsewhere(owner, here) {
		return fmt.Errorf("lyx beam promote: %s is owned by machine %s; way forward: run `lyx adopt %s` to take it over, then `lyx beam promote %s`", slug, owner, slug, slug)
	}
	if !deps.PairPresent(slug) {
		return fmt.Errorf("lyx beam promote: %s has no pair on this machine; way forward: run `lyx adopt %s` to add it, then `lyx beam promote %s`", slug, slug, slug)
	}

	tasks, err := deps.ReadTasks()
	if err != nil {
		return fmt.Errorf("lyx beam promote: read the board for the tasks that depend on %s: %w", slug, err)
	}
	if dependents := openDependents(tasks, slug); len(dependents) > 0 {
		return fmt.Errorf("lyx beam promote: the open tasks %s depend on %s, and a project has nothing depending on it; way forward: drop each dependency with `lyx board set-deps`, then run `lyx beam promote %s`", strings.Join(dependents, ", "), slug, slug)
	}

	origin, found, err := deps.OriginRecord(slug)
	if err != nil {
		return fmt.Errorf("lyx beam promote: read the origin record of %s: %w", slug, err)
	}
	if !found {
		return fmt.Errorf("lyx beam promote: pair %q has no origin record; way forward: run `lyx fabric reconcile` to restore the records worktree, then `lyx beam promote %s`", slug, slug)
	}
	if origin.ParentBranch == "" {
		return fmt.Errorf("lyx beam promote: the pair of %s is a root pair, made by `lyx fabric add --adopt-local` over the default branch, and cannot become a beam; way forward: make a project entry with `lyx board upsert` under a slug of its own, then run `lyx beam create` on it", slug)
	}
	return nil
}

// openDependents lists, sorted, the slugs of the open tasks that depend on slug.
func openDependents(tasks []boardengine.Task, slug string) []string {
	var dependents []string
	for _, task := range tasks {
		done := task.Status != nil && *task.Status == "done"
		if task.Kind == boardengine.KindTask && !done && slices.Contains(task.DependsOn, slug) {
			dependents = append(dependents, task.Slug)
		}
	}
	slices.Sort(dependents)
	return dependents
}

// promoteSteps runs the six steps in order, each skipped when its result stands, and records what it did on result.
func promoteSteps(ctx context.Context, deps promoteDeps, rec *fabricengine.Mutations, entry boardengine.OwnerEntry, here, slug string, result *promoteResult) error {
	rerun := "run `lyx beam promote " + slug + "` again"
	fail := func(step string, err error) error {
		return fmt.Errorf("lyx beam promote: %s of %s: %w; way forward: fix the cause it names, then %s", step, slug, err, rerun)
	}

	origin, _, err := deps.OriginRecord(slug)
	if err != nil {
		return fail("read the origin record", err)
	}

	if entry.RunHeld && !origin.Beam {
		if err := deps.EndRun(ctx, slug); err != nil {
			return err
		}
		result.RunEnded = true
	}

	if !origin.Beam {
		marked, err := deps.MarkBeam(rec, slug)
		if err != nil {
			return fail("mark the pair as a beam", err)
		}
		result.BeamMarked = marked
		if err := deps.SpawnPush(slug); err != nil {
			return fail("spawn the push", err)
		}
		result.PushSpawned = true
	}

	if entry.RunHeld {
		if err := deps.ClearRun(ctx, slug); err != nil {
			return err
		}
		result.RunCleared = true
	}

	reset, err := deps.ResetRecords(rec, slug)
	if err != nil {
		return err
	}
	result.RecordsReset = reset.Committed
	if reset.Committed {
		if err := deps.SpawnPush(slug); err != nil {
			return fail("spawn the push", err)
		}
		result.PushSpawned = true
	}

	write, err := deps.SetOwners(boardengine.OwnerRequest{Machine: here, Slugs: []string{slug}, Beam: true, ToProject: true, ParentOf: deps.ParentOf})
	if err != nil {
		return fail("turn the entry into a beam project", err)
	}
	if owner, lost := write.Lost[slug]; lost {
		return fmt.Errorf("lyx beam promote: another machine recorded %s as the owner of %s while this write was in flight; way forward: run `lyx adopt %s` to take it over, then `lyx beam promote %s`", owner, slug, slug, slug)
	}
	result.OwnerRecorded = true
	result.Promoted = true

	workspace, err := deps.RegenerateWorkspace(rec)
	if err != nil {
		return fail("regenerate the workspace files", err)
	}
	result.Workspace = workspace
	result.WayForward = orchWayForward(result.Worktree)
	return nil
}

// orchWayForward names the step that gives a beam its orch, which no verb of the module starts.
func orchWayForward(worktree string) string {
	return fmt.Sprintf("run `lyx orch start` in %s to give the beam an orch", worktree)
}
