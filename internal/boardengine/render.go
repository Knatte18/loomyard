// render.go — turns the task and note lists into the wiki's output files.
//
// Render is a pure function: tasks and notes in, a map of filename → content
// out (a single README.md carrying both a "# Tasks" and a "# Manifest"
// section, plus design-*.md for any task or note with a body). No I/O — the
// caller writes the files. The README's two sections are built by
// renderTasksSection and renderManifestSection; the design files are built by
// renderDesigns. RenderToDisk drives the write path and maintains a manifest
// sidecar (.board-rendered.json) so that renamed or removed outputs are
// cleaned up on the next render.

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

// RenderToDisk renders the tasks and notes and persists the board's readable
// representation. It writes every rendered file atomically, then removes any
// file the previous render produced that the current render no longer
// produces (covering README renames and DesignPrefix changes, not just
// orphaned design docs), and finally updates the manifest sidecar so the next
// render knows what to clean. render.go owns all .md output; board.go owns
// only tasks.json/notes.json. This is the single call the write path makes
// for rendering.
func RenderToDisk(boardPath string, tasks, notes []Task, out Outputs) error {
	files, err := Render(tasks, notes, out)
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

// Render produces the board output files from the task and note lists.
// Returns a map of relative filename → content: the configured README
// filename (a "# Tasks" section built from tasks, followed by a "# Manifest"
// section built from notes when notes is non-empty), plus design files (using
// the configured prefix and slug) for every task or note with a non-empty
// body.
func Render(tasks, notes []Task, out Outputs) (map[string]string, error) {
	ordered, err := RenderOrder(tasks)
	if err != nil {
		return nil, err
	}

	// Slug → task/note, for resolving dependency IDs within each section.
	// Notes resolve dependencies against noteMap only — per discussion.md's
	// per-store-scoped depends_on decision, a note's dependencies are always
	// other notes, never tasks.
	taskMap := make(map[string]Task, len(tasks))
	for _, t := range tasks {
		taskMap[t.Slug] = t
	}
	noteMap := make(map[string]Task, len(notes))
	for _, n := range notes {
		noteMap[n.Slug] = n
	}

	tasksSection := renderTasksSection(ordered, taskMap, out.DesignPrefix)
	manifestSection := renderManifestSection(notes, noteMap, out.DesignPrefix)

	// Concatenate Tasks/Done then Manifest, with a blank-line separator only
	// when there is a Manifest section to separate from — a notes-free board's
	// README must carry no dangling heading or trailing blank line.
	readme := tasksSection
	if manifestSection != "" {
		readme += "\n" + manifestSection
	}

	result := map[string]string{
		out.Readme: readme,
	}

	// A design-<slug>.md file applies uniformly to any entry — task or note —
	// with a non-empty body, so union both lists before rendering designs.
	combined := append(append([]Task{}, tasks...), notes...)
	for name, content := range renderDesigns(combined, out.DesignPrefix) {
		result[name] = content
	}
	return result, nil
}

// renderTasksSection builds the README's "# Tasks" section: sectioned per
// bucket, with a block per task (heading, slug line, optional dependencies,
// optional brief, and a [status] suffix). The design prefix is used in task
// links to design docs.
func renderTasksSection(ordered []TaskWithLayer, taskMap map[string]Task, designPrefix string) string {
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

// renderManifestSection builds the README's "# Manifest" section: a flat
// block per note (no layer suffix — notes have no computed layer — and no
// [status] suffix — notes are not bucket/status-tracked), sorted by ID
// ascending. Returns "" when notes is empty so a notes-free board's README
// carries no dangling Manifest heading.
func renderManifestSection(notes []Task, noteMap map[string]Task, designPrefix string) string {
	if len(notes) == 0 {
		return ""
	}

	sorted := make([]Task, len(notes))
	copy(sorted, notes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	lines := []string{"# Manifest", ""}
	for _, n := range sorted {
		lines = append(lines, fmt.Sprintf("## **#%03d:** %s", n.ID, n.Title))

		slugLine := fmt.Sprintf("[%s]", n.Slug)
		if n.Body != "" {
			slugLine = fmt.Sprintf("[%s](%s%s.md)", n.Slug, designPrefix, n.Slug)
		}
		lines = append(lines, slugLine)

		if len(n.DependsOn) > 0 {
			depParts := make([]string, 0, len(n.DependsOn))
			for _, depSlug := range n.DependsOn {
				if depNote, ok := noteMap[depSlug]; ok {
					depParts = append(depParts, fmt.Sprintf("#%03d", depNote.ID))
				} else {
					depParts = append(depParts, fmt.Sprintf("#???: %s (missing)", depSlug))
				}
			}
			lines = append(lines, "Depends on: "+strings.Join(depParts, ", "))
		}

		if n.Brief != "" {
			lines = append(lines, "", n.Brief)
		}

		lines = append(lines, "") // trailing blank line after the note block
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
