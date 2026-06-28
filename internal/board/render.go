// render.go — turns the task list into the wiki's output files.
//
// Render is a pure function: tasks in, a map of filename → content out
// (Home.md, _Sidebar.md, and proposal-*.md for tasks with a body). No I/O — the
// caller writes the files. The three outputs are built by renderHome,
// renderSidebar, and renderProposals respectively. RenderToDisk drives the write
// path and maintains a manifest sidecar (.board-rendered.json) so that renamed or
// removed outputs are cleaned up on the next render.

package board

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/fsx"
)

// renderManifestFile is the name of the sidecar that records the filenames the
// last render produced. It is gitignored (via ensureLockfilesIgnored) so it never
// adds commit churn, and it is never itself a member of the rendered file set.
const renderManifestFile = ".board-rendered.json"

// RenderToDisk renders the tasks and persists the board's readable representation.
// It writes every rendered file atomically, then removes any file the previous render
// produced that the current render no longer produces (covering Home/Sidebar renames
// and ProposalPrefix changes, not just orphaned proposals), and finally updates the
// manifest sidecar so the next render knows what to clean. render.go owns all .md
// output; board.go owns only tasks.json. This is the single call the write path makes
// for rendering.
func RenderToDisk(boardPath string, tasks []Task, out Outputs) error {
	files, err := Render(tasks, out)
	if err != nil {
		return err
	}
	for relPath, content := range files {
		if err := fsx.AtomicWrite(boardPath, relPath, content); err != nil {
			return fmt.Errorf("write %s: %w", relPath, err)
		}
	}

	// Read the previous manifest to discover files no longer produced by this render.
	// A missing or corrupt manifest is treated as an empty set — nothing to clean on
	// the first pass after an upgrade; the manifest is seeded from the current output.
	previous := readRenderManifest(boardPath)
	for _, name := range previous {
		if _, kept := files[name]; !kept {
			// Best-effort removal: a stale file left behind is harmless and cleaned up
			// on the next render, mirroring the old removeOrphanProposals contract.
			os.Remove(filepath.Join(boardPath, name))
		}
	}

	// Persist the current file set so the next render knows what to clean.
	writeRenderManifest(boardPath, files)
	return nil
}

// readRenderManifest returns the list of filenames recorded in the manifest sidecar
// from the previous render. It returns nil when the manifest is absent or unreadable
// (corrupt JSON, permission error, etc.) so the caller treats "nothing known" as a
// graceful no-op rather than an error.
func readRenderManifest(boardPath string) []string {
	data, err := os.ReadFile(filepath.Join(boardPath, renderManifestFile))
	if err != nil {
		return nil
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		// Corrupt manifest: treat as absent; it will be overwritten by the current set.
		return nil
	}
	return names
}

// writeRenderManifest persists the current render's file set to the manifest sidecar
// as a sorted JSON array for stable, diff-friendly output. Best-effort: errors are
// silently discarded so a manifest write failure never blocks a successful render.
func writeRenderManifest(boardPath string, files map[string]string) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	data, err := json.Marshal(names)
	if err != nil {
		return
	}
	_ = fsx.AtomicWriteBytes(filepath.Join(boardPath, renderManifestFile), data)
}

// Render produces the board output files from the task list.
// Returns a map of relative filename → content: the configured home and sidebar
// filenames, plus proposal files (using the configured prefix and slug) for every
// task with a non-empty body.
func Render(tasks []Task, out Outputs) (map[string]string, error) {
	ordered, err := RenderOrder(tasks)
	if err != nil {
		return nil, err
	}

	// Slug → task, for resolving dependency IDs in home file.
	taskMap := make(map[string]Task, len(tasks))
	for _, t := range tasks {
		taskMap[t.Slug] = t
	}

	result := map[string]string{
		out.Home:    renderHome(ordered, taskMap, out.ProposalPrefix),
		out.Sidebar: renderSidebar(ordered, out.ProposalPrefix),
	}
	for name, content := range renderProposals(tasks, out.ProposalPrefix) {
		result[name] = content
	}
	return result, nil
}

// renderHome builds the home file: a "# Tasks" page sectioned per bucket, with a block
// per task (heading, slug line, optional dependencies, optional brief). The proposal
// prefix is used in task links to proposals.
func renderHome(ordered []TaskWithLayer, taskMap map[string]Task, proposalPrefix string) string {
	lines := []string{"# Tasks", ""}

	currentBucket := ""
	for _, twl := range ordered {
		if twl.Layer != currentBucket {
			currentBucket = twl.Layer
			lines = append(lines, bucketHeader(twl.Layer), "")
		}

		// Heading: "## **#NNN:** Title [Layer]" (no layer suffix for done/deferred).
		displayTitle := fmt.Sprintf("**#%03d:** %s", twl.ID, twl.Title)
		if !isSpecialBucket(twl.Layer) {
			displayTitle += " [" + twl.Layer + "]"
		}
		lines = append(lines, "## "+displayTitle)

		// Slug line: a proposal link if the task has a body, else a bare slug.
		slugLine := fmt.Sprintf("[%s]", twl.Slug)
		if twl.Body != "" {
			slugLine = fmt.Sprintf("[%s](%s%s.md)", twl.Slug, proposalPrefix, twl.Slug)
		}
		if twl.Status != nil {
			switch *twl.Status {
			case "active", "done", "pr-pending", "ready-to-merge", "abandoned":
				slugLine += " [" + *twl.Status + "]"
			}
		}
		lines = append(lines, slugLine)

		if len(twl.DependsOn) > 0 {
			depParts := make([]string, 0, len(twl.DependsOn))
			for _, depSlug := range twl.DependsOn {
				if depTask, ok := taskMap[depSlug]; ok {
					depParts = append(depParts, fmt.Sprintf("#%03d", depTask.ID))
				} else {
					depParts = append(depParts, fmt.Sprintf("#???: %s (missing)", depSlug))
				}
			}
			lines = append(lines, "Depends on: "+strings.Join(depParts, ", "))
		}

		if twl.Brief != "" {
			lines = append(lines, "", twl.Brief)
		}

		lines = append(lines, "") // trailing blank line after the task block
	}

	return strings.Join(lines, "\n")
}

// renderSidebar builds the sidebar file: one line per task, grouped per bucket with a
// blank line between groups. The proposal prefix is used in task links to proposals.
// No trailing newline.
func renderSidebar(ordered []TaskWithLayer, proposalPrefix string) string {
	var lines []string

	currentBucket := ""
	for _, twl := range ordered {
		if twl.Layer != currentBucket {
			if currentBucket != "" && len(lines) > 0 {
				lines = append(lines, "")
			}
			currentBucket = twl.Layer
		}

		extTitle := ExtendedTitle(twl.Task, twl.Layer)
		line := "- " + extTitle
		if twl.Body != "" {
			line = fmt.Sprintf("- [%s](%s%s.md)", extTitle, proposalPrefix, twl.Slug)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// renderProposals returns one proposal file entry per task with a non-empty body,
// using the configured proposal prefix. The file content is the body verbatim.
func renderProposals(tasks []Task, proposalPrefix string) map[string]string {
	proposals := make(map[string]string)
	for _, t := range tasks {
		if t.Body != "" {
			proposals[fmt.Sprintf("%s%s.md", proposalPrefix, t.Slug)] = t.Body
		}
	}
	return proposals
}

// bucketHeader is the Home.md section heading for a bucket.
func bucketHeader(layer string) string {
	switch layer {
	case "__done__":
		return "# Done"
	case "__deferred__":
		return "# Someday"
	default:
		return "# Layer " + layer
	}
}

// isSpecialBucket reports whether a layer is one of the non-letter buckets that
// suppress the "[Layer]" title suffix.
func isSpecialBucket(layer string) bool {
	return layer == "__done__" || layer == "__deferred__"
}
