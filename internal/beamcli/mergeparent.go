// mergeparent.go is the "lym beam merge-parent" sequence: it judges the named beam and files the darn task, in the beam's project, whose run merges the beam's parent branch into the beam with its Code-Review perch on.

package beamcli

import (
	"fmt"
	"strconv"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/reviewbudget"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// mergeParentMinRounds is the fewest Code-Review rounds a merge task runs, so its perch is on whatever the hub's budget.
const mergeParentMinRounds = 1

// mergeParent files the merge task of the beam slug and returns the task's slug, the parent branch it merges and how the run follows.
// It refuses what `land`'s entry check refuses, a beam whose origin record names no parent branch, and a beam that holds an open merge task already.
// The task is a released darn task in the beam's project, carrying the beam's labels, so the beam's dobby starts it as any member.
// Its run settings set the Code-Review budget to the hub's, or to mergeParentMinRounds when the hub's is lower, with a hard cap above it.
// Its pull_request field lands it with no pull request, by a true merge, so the beam keeps the parent's commit as an ancestor.
func mergeParent(deps landDeps, slug string) (map[string]any, error) {
	const verb = "lym beam merge-parent"
	owners, err := deps.ReadOwners()
	if err != nil {
		return nil, fmt.Errorf("%s: read the board's owners: %w", verb, err)
	}
	if err := judgeLandEntry(deps, owners, verb, slug); err != nil {
		return nil, err
	}
	origin, _, err := deps.OriginRecord(slug)
	if err != nil {
		return nil, fmt.Errorf("%s: read the origin record of %s: %w", verb, slug, err)
	}
	if origin.ParentBranch == "" {
		return nil, fmt.Errorf("%s: the origin record of the beam %s names no parent branch; way forward: run `lym fabric reconcile` to restore its records worktree, then `%s %s`", verb, slug, verb, slug)
	}

	tasks, err := deps.Tasks()
	if err != nil {
		return nil, fmt.Errorf("%s: read the board: %w", verb, err)
	}
	if open, found := openMergeTask(tasks, slug); found {
		return nil, fmt.Errorf("%s: the merge task %s stands open in the beam %s; way forward: run `lym batten status %s` and let it land, or remove it with `lym batten cancel %s --remove`, then `%s %s`", verb, open, slug, open, open, verb, slug)
	}
	var labels []string
	for _, task := range tasks {
		if task.Slug == slug {
			labels = task.Labels
		}
	}
	caps, err := deps.CodeReviewCaps()
	if err != nil {
		return nil, fmt.Errorf("%s: read the hub's Code-Review budget: %w", verb, err)
	}
	rounds := max(caps.Soft, mergeParentMinRounds)
	perch := string(loomengine.PerchCode)

	taskSlug := freeMergeTaskSlug(tasks, slug)
	fields := map[string]any{
		"slug":         taskSlug,
		"title":        fmt.Sprintf("Merge %s into the beam %s", origin.ParentBranch, slug),
		"kind":         boardengine.KindTask,
		"project":      slug,
		"recipe":       shedrun.RecipeDarn,
		"labels":       labels,
		"body":         mergeParentBody(slug, origin.ParentBranch),
		"pull_request": boardengine.PullRequestMergeIn,
		"run_settings": map[string]any{
			"loom": map[string]any{
				reviewbudget.SoftKey(perch): rounds,
				reviewbudget.HardKey(perch): max(caps.Hard, rounds+1),
			},
		},
	}
	if err := deps.FileTask(fields); err != nil {
		return nil, fmt.Errorf("%s: file the merge task %s: %w; way forward: fix the cause it names, then run `%s %s` again", verb, taskSlug, err, verb, slug)
	}
	return map[string]any{
		"slug":          slug,
		"task":          taskSlug,
		"parent_branch": origin.ParentBranch,
		"way_forward":   fmt.Sprintf("the beam's dobby starts %s as any member; run `lym batten status %s` to follow it", taskSlug, taskSlug),
	}, nil
}

// openMergeTask returns the slug of a merge task of beam that is not done, found by its project and its slug's suffix.
func openMergeTask(tasks []boardengine.Task, beam string) (string, bool) {
	for _, task := range tasks {
		if task.Kind == boardengine.KindTask && task.Project == beam && !boardengine.IsDoneStatus(task.Status) && darnengine.IsMergeTaskSlug(task.Slug) {
			return task.Slug, true
		}
	}
	return "", false
}

// freeMergeTaskSlug returns the first merge task slug of beam no board entry holds: the beam's slug and the suffix, then the same with -2, -3 and so on.
// The beam's slug is cut so the whole fits the board's slug length.
func freeMergeTaskSlug(tasks []boardengine.Task, beam string) string {
	taken := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		taken[task.Slug] = true
	}
	for number := 1; ; number++ {
		tail := darnengine.MergeTaskSuffix
		if number > 1 {
			tail += "-" + strconv.Itoa(number)
		}
		head := beam
		if room := boardengine.MaxSlugLength - len(tail); len(head) > room {
			head = head[:room]
		}
		if candidate := head + tail; !taken[candidate] {
			return candidate
		}
	}
}

// mergeParentBody returns the body of the merge task of beam, which merges parentBranch.
// The decision record carries it as the task's goal, so it instructs both the darn writer and the Code-Review perch.
func mergeParentBody(beam, parentBranch string) string {
	return fmt.Sprintf(`Merge in from parent: bring the parent branch %[2]s into the beam %[1]s.
This task's branch forks from the beam, so its landing carries the merge into the beam, as a true merge with no pull request.
Filed by `+"`lym beam merge-parent %[1]s`"+`.

- Run `+"`lym fabric merge-in %[2]s`"+` in this task's worktree.
  On a conflict, settle each path it lists, stage them with `+"`lym fabric merge-stage <paths>`"+` and conclude with `+"`lym fabric merge --continue`"+`.
- Make the gate pass on the merged tree: fix what the merge broke, on either side, and nothing else.
- Make no change the merge does not need: no refactor, no feature and no fix of an older fault.
- When the merge is already up to date, change nothing and say so in the change description.
- The Code-Review perch reviews the merge alone: each merge commit's conflict resolution and every commit after the merge, never the parent's own commits the merge brings in.
  Checking the project's goal is no part of this task.
`, beam, parentBranch)
}
