// layer.go — derived task fields.
//
// ComputeLayers assigns each task its README subsection: Running, Ready, a dependency layer or Independent, and RenderOrder orders tasks for output.
// All computed at read time;
// never stored.

package boardengine

import (
	"fmt"
	"sort"
)

// The non-letter layers: a running task, an open task waiting on nothing open, an isolated task, an open project and a done entry.
const (
	runningLayer  = "Running"
	readyLayer    = "Ready"
	isolatedLayer = "Z"
	projectLayer  = "Project"
	doneLayer     = "__done__"
)

// ComputeLayers assigns each task a bucket based on topological depth.
// A task a run holds is runningLayer, by RunHolds as the run lock decides it.
// An open project a run does not hold, a beam whose pawl run has not reached Parent-Align included, is projectLayer, never Ready or a letter, and counts as no entry's dependency depth.
// An open task with no open dependency is readyLayer, and every other task is the letter of its depth:
// A waits only on running or ready tasks, B on something in A, and so on.
// A dependency on a done task adds no depth, while one on a running task counts as one on a ready task, and cycle detection still follows running tasks.
func ComputeLayers(tasks []Task) (map[string]string, error) {
	layerMap := make(map[string]string)

	for _, t := range tasks {
		if isDone(t) {
			layerMap[t.Slug] = doneLayer
		} else if RunHolds(t) {
			layerMap[t.Slug] = runningLayer
		} else if t.Kind == KindProject {
			layerMap[t.Slug] = projectLayer
		} else if t.Isolated {
			layerMap[t.Slug] = isolatedLayer
		}
	}

	taskMap := make(map[string]*Task)
	for i := range tasks {
		taskMap[tasks[i].Slug] = &tasks[i]
	}

	color := make(map[string]string)
	for slug := range taskMap {
		color[slug] = "white"
	}

	var detectCycleDFS func(slug string) error
	detectCycleDFS = func(slug string) error {
		if color[slug] == "black" {
			return nil // Already processed.
		}
		if color[slug] == "gray" {
			return fmt.Errorf("cycle detected involving %s", slug)
		}

		color[slug] = "gray"
		t := taskMap[slug]
		for _, dep := range t.DependsOn {
			depTask, ok := taskMap[dep]
			if !ok {
				continue // Skip missing deps.
			}
			// Skip done tasks in cycle detection.
			if isDone(*depTask) {
				continue
			}
			if err := detectCycleDFS(dep); err != nil {
				return err
			}
		}
		color[slug] = "black"
		return nil
	}

	for slug := range taskMap {
		if color[slug] == "white" {
			if err := detectCycleDFS(slug); err != nil {
				return nil, err
			}
		}
	}

	depth := make(map[string]int)

	var getDepth func(slug string) (int, error)
	getDepth = func(slug string) (int, error) {
		if d, ok := depth[slug]; ok {
			return d, nil
		}

		t := taskMap[slug]
		if t == nil {
			return 0, nil
		}

		if layerMap[slug] != "" {
			depth[slug] = 0 // Special tasks don't contribute to depth.
			return 0, nil
		}

		maxDepth := -1
		for _, dep := range t.DependsOn {
			depTask, ok := taskMap[dep]
			if !ok {
				continue
			}
			// A done dependency adds no depth; a running one weighs as a ready one, at depth 0; a project adds none.
			if isDone(*depTask) || depTask.Kind == KindProject {
				continue
			}
			d, err := getDepth(dep)
			if err != nil {
				return 0, err
			}
			if d > maxDepth {
				maxDepth = d
			}
		}

		// Depth 0 is ready, and depth n from 1 on is the nth letter, so the letters stop at Y.
		d := maxDepth + 1
		if d > 25 {
			return 0, fmt.Errorf("layer depth exceeds A..Y cap")
		}

		depth[slug] = d
		return d, nil
	}

	// Compute depths for all tasks.
	for slug := range taskMap {
		if _, ok := layerMap[slug]; ok {
			continue // Skip already assigned.
		}
		d, err := getDepth(slug)
		if err != nil {
			return nil, err
		}
		if d == 0 {
			layerMap[slug] = readyLayer
		} else {
			layerMap[slug] = string(rune('A' + d - 1))
		}
	}

	return layerMap, nil
}

// TaskWithLayer wraps a Task with its computed layer string.
// It is never JSON-encoded: the embedded Task's MarshalJSON is promoted onto it and would drop Layer.
type TaskWithLayer struct {
	Task
	Layer string
}

// kindRank orders the kinds in README order: tasks, then projects, then notes.
func kindRank(kind string) int {
	switch kind {
	case KindProject:
		return 1
	case KindNote:
		return 2
	}
	return 0
}

// RenderOrder returns tasks in README order.
// Open tasks come first, then open projects, then open notes, then done entries.
// Open tasks order by dependency layer, then by handle;
// open projects and open notes order by handle, so each type's notes run by number;
// done entries put tasks before projects before notes, each by handle.
// A handle orders by letter, then by number, and entries with equal handles by ID.
func RenderOrder(tasks []Task) ([]TaskWithLayer, error) {
	layerMap, err := ComputeLayers(tasks)
	if err != nil {
		return nil, err
	}

	// Wrap tasks with their layers.
	var result []TaskWithLayer
	for _, t := range tasks {
		result = append(result, TaskWithLayer{
			Task:  t,
			Layer: layerMap[t.Slug],
		})
	}

	// Running, then ready, then the letters A..Y, isolated and done.
	bucketOrder := map[string]int{runningLayer: 0, readyLayer: 1, isolatedLayer: 27, doneLayer: 28}
	for letter := 'A'; letter <= 'Y'; letter++ {
		bucketOrder[string(letter)] = 2 + int(letter-'A')
	}

	sort.Slice(result, func(i, j int) bool {
		doneI := result[i].Layer == doneLayer
		doneJ := result[j].Layer == doneLayer
		if doneI != doneJ {
			return doneJ
		}
		if result[i].Kind != result[j].Kind {
			return kindRank(result[i].Kind) < kindRank(result[j].Kind)
		}
		// An open note's README group is its type label, which its handle letter already groups, and a project has no layer to order by, so only a task's layer plays a part.
		if result[i].Kind != KindNote && result[i].Kind != KindProject {
			bucketI := bucketOrder[result[i].Layer]
			bucketJ := bucketOrder[result[j].Layer]
			if bucketI != bucketJ {
				return bucketI < bucketJ
			}
		}
		if c := compareHandles(result[i].Handle, result[j].Handle); c != 0 {
			return c < 0
		}
		return result[i].ID < result[j].ID
	})

	return result, nil
}
