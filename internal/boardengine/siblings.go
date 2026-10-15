// siblings.go — the sibling check of a depends_on write: the tasks that newly wait on a dependency another open task waits on while sharing a module label.

package boardengine

import "slices"

// Sibling is one reported pair of open tasks that wait on the same dependency and share a module label.
// Task is the slug of the written task and Sibling the slug of the other task.
// Deps are the dependencies that make the pair siblings, each once, in the written task's depends_on order.
// Labels are the module labels both carry, in the written task's label order.
type Sibling struct {
	Task    string   `json:"task"`
	Sibling string   `json:"sibling"`
	Deps    []string `json:"deps"`
	Labels  []string `json:"labels"`
}

// SharedModuleLabels returns the labels a and b both carry that v does not report as type labels, in a's order, or nil when there are none.
func SharedModuleLabels(a, b Task, v Vocabulary) []string {
	var shared []string
	for _, label := range a.Labels {
		if v.IsType(label) || !slices.Contains(b.Labels, label) || slices.Contains(shared, label) {
			continue
		}
		shared = append(shared, label)
	}
	return shared
}

// addedSiblings returns the siblings of one write from the entries as loaded (before) and as the mutation left them (after).
// A task X in after that is not done has the dependencies its depends_on gained over its baseline, minus any that are done;
// X's baseline is its own depends_on in before together with the depends_on of every before entry absent from after, so a merge warns only on dependencies none of the merged entries had.
// Every other entry in after that is neither done nor abandoned, waits on an added dependency and shares a module label with X is a sibling of X.
// A pair found from both sides is reported once, in the first orientation in after order, with both sides' dependencies.
// A write that adds no dependency returns nil.
func addedSiblings(before, after []Task, v Vocabulary) []Sibling {
	afterSlugs := make(map[string]Task, len(after))
	for _, task := range after {
		afterSlugs[task.Slug] = task
	}
	var removedDeps []string
	beforeDeps := make(map[string][]string, len(before))
	for _, task := range before {
		beforeDeps[task.Slug] = task.DependsOn
		if _, kept := afterSlugs[task.Slug]; !kept {
			removedDeps = append(removedDeps, task.DependsOn...)
		}
	}

	var siblings []Sibling
	reported := map[[2]string]int{}
	for _, x := range after {
		if isDone(x) {
			continue
		}
		var added []string
		for _, dep := range x.DependsOn {
			if slices.Contains(added, dep) || slices.Contains(beforeDeps[x.Slug], dep) || slices.Contains(removedDeps, dep) {
				continue
			}
			if target, found := afterSlugs[dep]; found && isDone(target) {
				continue
			}
			added = append(added, dep)
		}
		if len(added) == 0 {
			continue
		}

		for _, y := range after {
			if y.Slug == x.Slug || isDone(y) || (y.Status != nil && *y.Status == "abandoned") {
				continue
			}
			var shared []string
			for _, dep := range added {
				if slices.Contains(y.DependsOn, dep) {
					shared = append(shared, dep)
				}
			}
			labels := SharedModuleLabels(x, y, v)
			if len(shared) == 0 || len(labels) == 0 {
				continue
			}
			if index, seen := reported[[2]string{y.Slug, x.Slug}]; seen {
				for _, dep := range shared {
					if !slices.Contains(siblings[index].Deps, dep) {
						siblings[index].Deps = append(siblings[index].Deps, dep)
					}
				}
				continue
			}
			reported[[2]string{x.Slug, y.Slug}] = len(siblings)
			siblings = append(siblings, Sibling{Task: x.Slug, Sibling: y.Slug, Deps: shared, Labels: labels})
		}
	}
	return siblings
}

// writeReportingSiblings runs fn as a board write under the write lock and returns fn's result with the siblings the write created.
// The siblings are computed inside the locked mutation from the entries as loaded and as fn left them,
// so they describe exactly the board the write saves.
// A refused write returns its error and no siblings; the check itself never refuses.
func (b *Board) writeReportingSiblings(fn func(store *Store) (any, error)) (any, []Sibling, error) {
	var siblings []Sibling
	result, err := b.boardCriticalSection(func(store *Store) (any, error) {
		loaded := cloneTasks(store.Tasks())
		result, err := fn(store)
		if err != nil {
			return nil, err
		}
		siblings = addedSiblings(loaded, store.Tasks(), b.vocab)
		return result, nil
	}, nil)
	if err != nil {
		return nil, nil, err
	}
	return result, siblings, nil
}

// UpsertTaskSiblings creates or updates the task identified by fields["slug"] under the write lock and returns the siblings the write created beside it;
// field validation (allowlist, slug shape) is the store's job.
func (b *Board) UpsertTaskSiblings(fields map[string]any) (Task, []Sibling, error) {
	result, siblings, err := b.writeReportingSiblings(func(store *Store) (any, error) {
		return store.UpsertTask(fields)
	})
	if err != nil {
		return Task{}, nil, err
	}
	return result.(Task), siblings, nil
}
