package wiki

import (
	"fmt"
	"sort"
)

const maxLayer = 24 // A-Y is 0-24

// ComputeLayers assigns each task a layer bucket.
// Returns a map from task slug to layer string.
// Special buckets: "__done__" for done tasks, "__deferred__" for deferred tasks,
// "Z" for isolated tasks, and "A"-"Y" for normal tasks based on topological depth.
func ComputeLayers(tasks []Task) (map[string]string, error) {
	// Build a map from slug to task for quick lookup
	taskBySlug := make(map[string]*Task)
	for i := range tasks {
		taskBySlug[tasks[i].Slug] = &tasks[i]
	}

	// Phase 1: Detect cycles using DFS
	colors := make(map[string]int) // 0=white, 1=gray, 2=black
	for _, task := range tasks {
		if colors[task.Slug] == 0 {
			if err := detectCycle(task, taskBySlug, colors); err != nil {
				return nil, err
			}
		}
	}

	// Phase 2: Calculate depths
	depths := make(map[string]int)
	for _, task := range tasks {
		if _, ok := depths[task.Slug]; !ok {
			calculateDepth(task, taskBySlug, depths)
		}
	}

	// Assign layers
	layers := make(map[string]string)
	for _, task := range tasks {
		if task.Status != nil && *task.Status == "done" {
			layers[task.Slug] = "__done__"
		} else if task.Deferred {
			layers[task.Slug] = "__deferred__"
		} else if task.Isolated {
			layers[task.Slug] = "Z"
		} else {
			depth := depths[task.Slug]
			if depth > maxLayer {
				return nil, fmt.Errorf("layer depth exceeds A..Y cap")
			}
			layers[task.Slug] = string(rune('A' + depth))
		}
	}

	return layers, nil
}

// detectCycle performs DFS to detect cycles, skipping done tasks' dependencies.
func detectCycle(task Task, taskBySlug map[string]*Task, colors map[string]int) error {
	colors[task.Slug] = 1 // gray
	for _, depSlug := range task.DependsOn {
		depTask, ok := taskBySlug[depSlug]
		if !ok {
			continue // dependency not found, skip
		}
		// Skip done tasks
		if depTask.Status != nil && *depTask.Status == "done" {
			continue
		}
		if colors[depSlug] == 1 {
			return fmt.Errorf("cycle detected")
		}
		if colors[depSlug] == 0 {
			if err := detectCycle(*depTask, taskBySlug, colors); err != nil {
				return err
			}
		}
	}
	colors[task.Slug] = 2 // black
	return nil
}

// calculateDepth computes the topological depth of a task recursively.
// Depth = 0 if no effective dependencies, otherwise 1 + max(depth of effective dependencies).
// Effective dependencies exclude done tasks.
func calculateDepth(task Task, taskBySlug map[string]*Task, depths map[string]int) int {
	if depth, ok := depths[task.Slug]; ok {
		return depth
	}

	maxDepth := -1 // -1 means no effective dependencies
	for _, depSlug := range task.DependsOn {
		depTask, ok := taskBySlug[depSlug]
		if !ok {
			continue // dependency not found, skip
		}
		// Skip done tasks
		if depTask.Status != nil && *depTask.Status == "done" {
			continue
		}
		depDepth := calculateDepth(*depTask, taskBySlug, depths)
		if depDepth > maxDepth {
			maxDepth = depDepth
		}
	}

	depth := maxDepth + 1
	depths[task.Slug] = depth
	return depth
}

// TaskWithLayer embeds a Task with its layer information.
type TaskWithLayer struct {
	Task
	Layer string
}

// RenderOrder returns tasks sorted by bucket order.
// Bucket order: letter buckets A-Y (alphabetical), then Z, then __deferred__, then __done__.
// Within each bucket, tasks are sorted by ID.
func RenderOrder(tasks []Task) ([]TaskWithLayer, error) {
	layers, err := ComputeLayers(tasks)
	if err != nil {
		return nil, err
	}

	// Create TaskWithLayer instances
	tasksWithLayers := make([]TaskWithLayer, len(tasks))
	for i, task := range tasks {
		tasksWithLayers[i] = TaskWithLayer{
			Task:  task,
			Layer: layers[task.Slug],
		}
	}

	// Sort by bucket order
	sort.Slice(tasksWithLayers, func(i, j int) bool {
		layerI := tasksWithLayers[i].Layer
		layerJ := tasksWithLayers[j].Layer

		// Define bucket order
		bucketOrder := func(layer string) int {
			if len(layer) == 1 && layer >= "A" && layer <= "Y" {
				return int(layer[0] - 'A') // 0-23 for A-Y
			}
			if layer == "Z" {
				return 25
			}
			if layer == "__deferred__" {
				return 26
			}
			if layer == "__done__" {
				return 27
			}
			return 28 // unknown
		}

		bucketI := bucketOrder(layerI)
		bucketJ := bucketOrder(layerJ)

		if bucketI != bucketJ {
			return bucketI < bucketJ
		}

		// Within same bucket, sort by ID
		return tasksWithLayers[i].ID < tasksWithLayers[j].ID
	})

	return tasksWithLayers, nil
}

// ExtendedTitle returns the task title annotated with layer if applicable.
// For letter buckets (A-Y) and Z, appends " [layer]" to the title.
// For __done__ and __deferred__ buckets, returns the plain title.
func ExtendedTitle(t Task, layer string) string {
	if (len(layer) == 1 && layer >= "A" && layer <= "Y") || layer == "Z" {
		return t.Title + " [" + layer + "]"
	}
	return t.Title
}
