// layer.go — derived task fields.
//
// ComputeLayers assigns each task a dependency depth (its render bucket),
// and RenderOrder orders tasks for output.
// All computed at read time;
// never stored.

package boardengine

import (
	"fmt"
	"sort"
)

// ComputeLayers assigns each task a bucket based on topological depth.
func ComputeLayers(tasks []Task) (map[string]string, error) {
	layerMap := make(map[string]string)

	for _, t := range tasks {
		if t.Status != nil && *t.Status == "done" {
			layerMap[t.Slug] = "__done__"
		} else if t.Deferred {
			layerMap[t.Slug] = "__deferred__"
		} else if t.Isolated {
			layerMap[t.Slug] = "Z"
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
			if depTask.Status != nil && *depTask.Status == "done" {
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
			if depTask.Status != nil && *depTask.Status == "done" {
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

		d := maxDepth + 1
		if d >= 25 {
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
		layerMap[slug] = string(rune('A' + d))
	}

	return layerMap, nil
}

// TaskWithLayer wraps a Task with its computed layer string.
type TaskWithLayer struct {
	Task
	Layer string
}

// RenderOrder returns tasks sorted by bucket order, then by ID.
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

	// Define bucket order.
	bucketOrder := map[string]int{
		"A": 0, "B": 1, "C": 2, "D": 3, "E": 4,
		"F": 5, "G": 6, "H": 7, "I": 8, "J": 9,
		"K": 10, "L": 11, "M": 12, "N": 13, "O": 14,
		"P": 15, "Q": 16, "R": 17, "S": 18, "T": 19,
		"U": 20, "V": 21, "W": 22, "X": 23, "Y": 24,
		"Z":            25,
		"__deferred__": 26,
		"__done__":     27,
	}

	sort.Slice(result, func(i, j int) bool {
		bucketI := bucketOrder[result[i].Layer]
		bucketJ := bucketOrder[result[j].Layer]
		if bucketI != bucketJ {
			return bucketI < bucketJ
		}
		return result[i].ID < result[j].ID
	})

	return result, nil
}
