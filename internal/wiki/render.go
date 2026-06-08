// render.go — turns the task list into the wiki's output files.
//
// Render is a pure function: tasks in, a map of filename → content out
// (Home.md, _Sidebar.md, and proposal-*.md for tasks with a body). No I/O — the
// caller writes the files.

package wiki

import (
	"fmt"
	"strings"
)

// Render produces the wiki output files from the task list.
// Returns a map of relative filename → content: always "Home.md" and "_Sidebar.md",
// plus "proposal-<slug>.md" for every task with a non-empty body.
func Render(tasks []Task) (map[string]string, error) {
	result := make(map[string]string)

	tasksWithLayer, err := RenderOrder(tasks)
	if err != nil {
		return nil, err
	}

	// Build task slug map for dependency resolution
	taskMap := make(map[string]Task)
	for _, t := range tasks {
		taskMap[t.Slug] = t
	}

	// Build Home.md
	homeLines := []string{"# Tasks", ""}

	// Build _Sidebar.md
	var sidebarLines []string

	// Group tasks by layer
	var currentBucket string
	for _, twl := range tasksWithLayer {
		if twl.Layer != currentBucket {
			// Bucket boundary: add blank line to sidebar before new bucket (except first)
			if currentBucket != "" && len(sidebarLines) > 0 {
				sidebarLines = append(sidebarLines, "")
			}
			currentBucket = twl.Layer

			// Add bucket header to Home.md
			bucketHeader := ""
			switch twl.Layer {
			case "__done__":
				bucketHeader = "# Done"
			case "__deferred__":
				bucketHeader = "# Someday"
			default:
				bucketHeader = "# Layer " + twl.Layer
			}
			homeLines = append(homeLines, bucketHeader, "")
		}

		// Build display title for heading
		displayTitle := fmt.Sprintf("**#%03d:** %s", twl.ID, twl.Title)
		if twl.Layer != "__done__" && twl.Layer != "__deferred__" {
			displayTitle += " [" + twl.Layer + "]"
		}

		// Add heading to Home.md
		homeLines = append(homeLines, "## "+displayTitle)

		// Add slug line to Home.md
		slugLine := ""
		if twl.Body != "" {
			slugLine = fmt.Sprintf("[%s](proposal-%s.md)", twl.Slug, twl.Slug)
		} else {
			slugLine = fmt.Sprintf("[%s]", twl.Slug)
		}

		// Append status if present and in the allowed list
		if twl.Status != nil {
			status := *twl.Status
			switch status {
			case "active", "done", "pr-pending", "ready-to-merge", "abandoned":
				slugLine += " [" + status + "]"
			}
		}

		homeLines = append(homeLines, slugLine)

		// Add depends on line if non-empty
		if len(twl.DependsOn) > 0 {
			depParts := []string{}
			for _, depSlug := range twl.DependsOn {
				if depTask, ok := taskMap[depSlug]; ok {
					depParts = append(depParts, fmt.Sprintf("#%03d", depTask.ID))
				} else {
					depParts = append(depParts, fmt.Sprintf("#???: %s (missing)", depSlug))
				}
			}
			homeLines = append(homeLines, "Depends on: "+strings.Join(depParts, ", "))
		}

		// Add brief if non-empty
		if twl.Brief != "" {
			homeLines = append(homeLines, "")
			homeLines = append(homeLines, twl.Brief)
		}

		// Trailing blank line after task block
		homeLines = append(homeLines, "")

		// Add to sidebar
		extTitle := ExtendedTitle(twl.Task, twl.Layer)
		sidebarLine := "- " + extTitle
		if twl.Body != "" {
			sidebarLine = fmt.Sprintf("- [%s](proposal-%s.md)", extTitle, twl.Slug)
		}
		sidebarLines = append(sidebarLines, sidebarLine)
	}

	// Join Home.md lines
	homeContent := strings.Join(homeLines, "\n")
	result["Home.md"] = homeContent

	// Join Sidebar lines (no trailing newline per spec)
	sidebarContent := strings.Join(sidebarLines, "\n")
	result["_Sidebar.md"] = sidebarContent

	// Add proposal files for tasks with body
	for _, t := range tasks {
		if t.Body != "" {
			result[fmt.Sprintf("proposal-%s.md", t.Slug)] = t.Body
		}
	}

	return result, nil
}
