// layer.go — derived task fields.
//
// ComputeLayers assigns each task its README subsection: Running, Ready, a dependency layer or Independent,
// and RenderOrder orders tasks for output.
// All computed at read time;
// never stored.

package boardengine

import (
	"fmt"
	"sort"
)

// The non-letter layers: a running task, an open task waiting on nothing open, an isolated task and a done entry.
const (
	runningLayer  = "Running"
	readyLayer    = "Ready"
	isolatedLayer = "Z"
	doneLayer     = "__done__"
)

// ComputeLayers assigns each task a bucket based on topological depth.
// A task a run holds is runningLayer, by IsRunStatus as the run lock decides it.
// An open task with no open dependency is readyLayer, and every other task is the letter of its depth:
// A waits only on running or ready tasks, B on something in A, and so on.
// A dependency on a done task adds no depth, while one on a running task counts as one on a ready task,
// and cycle detection still follows running tasks.
func ComputeLayers(tasks []Task) (map[string]string, error) {
	layerMap := make(map[string]string)

	for _, t := range tasks {
		if isDone(t) {
			layerMap[t.Slug] = doneLayer
		} else if IsRunStatus(t.Status) {
			layerMap[t.Slug] = runningLayer
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
			// A done dependency adds no depth; a running one weighs as a ready one, at depth 0.
			if isDone(*depTask) {
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
type TaskWithLayer struct {
	Task
	Layer string
}

// RenderOrder returns tasks in README order.
// Open tasks come first, then open notes, then done entries.
// Within each section, entries order by dependency layer, then by ID.
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
		if !doneI && result[i].Kind != result[j].Kind {
			return result[i].Kind == KindTask
		}
		bucketI := bucketOrder[result[i].Layer]
		bucketJ := bucketOrder[result[j].Layer]
		if bucketI != bucketJ {
			return bucketI < bucketJ
		}
		return result[i].ID < result[j].ID
	})

	return result, nil
}
