// render.go — turns the entry list into the wiki's output files.
//
// Render is a pure function: entries in, a map of filename → content out (a single README.md built
// by renderTasksSection, plus design-*.md for any entry with a body).
// No I/O — the caller writes the files.
// The design files are built by renderDesigns.
// RenderToDisk drives the write path and maintains a manifest sidecar (.board-rendered.json) so
// that renamed or removed outputs are cleaned up on the next render.

package boardengine

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

// RenderToDisk renders the entries, persisting the board's output files.
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

	previous := readRenderManifest(boardPath)
	for _, name := range previous {
		if _, kept := files[name]; !kept {
			// Best-effort removal: a stale file left behind is harmless and cleaned up
			// on the next render, mirroring the old removeOrphanProposals contract.
			os.Remove(filepath.Join(boardPath, name))
		}
	}

	writeRenderManifest(boardPath, files)
	return nil
}

// readRenderManifest returns the list of filenames from the previous render's manifest.
func readRenderManifest(boardPath string) []string {
	data, err := os.ReadFile(filepath.Join(boardPath, renderManifestFile))
	if err != nil {
		return nil
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil
	}
	return names
}

// writeRenderManifest persists the current render's file set to the manifest sidecar.
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

// Render produces the board output files from the entry list.
func Render(tasks []Task, out Outputs) (map[string]string, error) {
	ordered, err := RenderOrder(tasks)
	if err != nil {
		return nil, err
	}

	taskMap := make(map[string]Task, len(tasks))
	for _, t := range tasks {
		taskMap[t.Slug] = t
	}

	result := map[string]string{
		out.Readme: renderTasksSection(ordered, taskMap, out.DesignPrefix),
	}

	for name, content := range renderDesigns(tasks, out.DesignPrefix) {
		result[name] = content
	}
	return result, nil
}

// renderTasksSection builds the "# Tasks" section of the README.
func renderTasksSection(ordered []TaskWithLayer, taskMap map[string]Task, designPrefix string) string {
	lines := []string{"# Tasks", ""}

	currentBucket := ""
	for _, twl := range ordered {
		if twl.Layer != currentBucket {
			currentBucket = twl.Layer
			lines = append(lines, bucketHeader(twl.Layer), "")
		}

		// Heading: "## **#NNN:** Title [Layer]" (no layer suffix for done).
		displayTitle := fmt.Sprintf("**#%03d:** %s", twl.ID, twl.Title)
		if !isSpecialBucket(twl.Layer) {
			displayTitle += " [" + twl.Layer + "]"
		}
		lines = append(lines, "## "+displayTitle)

		// Slug line: a design-doc link if the task has a body, else a bare slug.
		slugLine := fmt.Sprintf("[%s]", twl.Slug)
		if twl.Body != "" {
			slugLine = fmt.Sprintf("[%s](%s%s.md)", twl.Slug, designPrefix, twl.Slug)
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

// renderDesigns returns one design-doc file entry per task or note with a
// non-empty body, using the configured design prefix. The file content is the
// body verbatim.
func renderDesigns(entries []Task, designPrefix string) map[string]string {
	designs := make(map[string]string)
	for _, e := range entries {
		if e.Body != "" {
			designs[fmt.Sprintf("%s%s.md", designPrefix, e.Slug)] = e.Body
		}
	}
	return designs
}

// bucketHeader is the README section heading for a bucket.
func bucketHeader(layer string) string {
	switch layer {
	case "__done__":
		return "# Done"
	default:
		return "# Layer " + layer
	}
}

// isSpecialBucket reports whether a layer is one of the non-letter buckets that
// suppress the "[Layer]" title suffix.
func isSpecialBucket(layer string) bool {
	return layer == "__done__"
}
