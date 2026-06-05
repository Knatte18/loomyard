package wiki

import (
	"fmt"
	"sort"
)

// ComputeLayers assigns each task a bucket string based on topological depth.
// Special buckets: "__done__" (Status=="done"), "__deferred__" (Deferred), "Z" (Isolated).
// Regular buckets: "A"–"Y" based on depth (depth 0 = A, depth 1 = B, ..., depth 24 = Y).
// Returns error if any non-special task has depth >= 25.
func ComputeLayers(tasks []Task) (map[string]string, error) {
	layerMap := make(map[string]string)

	// Fast path: assign special buckets.
	for _, t := range tasks {
		if t.Status != nil && *t.Status == "done" {
			layerMap[t.Slug] = "__done__"
		} else if t.Deferred {
			layerMap[t.Slug] = "__deferred__"
		} else if t.Isolated {
			layerMap[t.Slug] = "Z"
		}
	}

	// Build slug → task index.
	taskMap := make(map[string]*Task)
	for i := range tasks {
		taskMap[tasks[i].Slug] = &tasks[i]
	}

	// Phase 1: DFS to detect cycles (white/gray/black).
	color := make(map[string]string) // "white", "gray", "black"
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

	// Phase 2: Memoized depth calculation.
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

		// Skip tasks with special buckets already assigned.
		if layerMap[slug] != "" {
			depth[slug] = 0 // Special tasks don't contribute to depth.
			return 0, nil
		}

		maxDepth := -1
		for _, dep := range t.DependsOn {
			depTask, ok := taskMap[dep]
			if !ok {
				continue // Skip missing deps.
			}
			// Exclude done tasks from depth calculation.
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
		// Convert depth to letter: 0→A, 1→B, ..., 24→Y.
		layerMap[slug] = string(rune('A' + d))
	}

	return layerMap, nil
}

// TaskWithLayer wraps a Task with its computed layer string.
type TaskWithLayer struct {
	Task
	Layer string
}

// RenderOrder returns tasks sorted by bucket order then by ID.
// Bucket order: A–Y (alphabetical), then Z, then __deferred__, then __done__.
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
		"Z":             25,
		"__deferred__":  26,
		"__done__":      27,
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

// ExtendedTitle returns the task title, optionally annotated with layer.
// For letter buckets (A–Y) and Z, returns title + " [" + layer + "]".
// For __done__ and __deferred__, returns plain title.
func ExtendedTitle(t Task, layer string) string {
	if layer == "__done__" || layer == "__deferred__" {
		return t.Title
	}
	return t.Title + " [" + layer + "]"
}
