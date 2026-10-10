// render.go — turns the entry list into the wiki's output files.
//
// Render is a pure function: entries in, a map of filename → content out (a single README.md built by renderTasksSection, plus design-*.md for any entry with a body).
// The README reads like a roadmap: Tasks split into a Running subsection and dependency layers, Notes with one subsection per type label, then Done, each entry one numbered item showing its labels.
// The section names and their meaning lines are declared here alone;
// the data holds only the kind and the labels.
// No I/O — the caller writes the files.
// The design files are built by renderDesigns;
// each opens with a header naming the entry's slug, kind, labels and status, then the stored body unchanged.
// RenderToDisk drives the write path and maintains a manifest sidecar (.board-rendered.json) so
// that renamed or removed outputs are cleaned up on the next render.

package boardengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

	result := map[string]string{
		out.Readme: renderTasksSection(ordered, out.DesignPrefix, out.Types),
	}

	for name, content := range renderDesigns(tasks, out.DesignPrefix) {
		result[name] = content
	}
	return result, nil
}

// readmeSection is one README section: its heading name and the one-line meaning under it.
type readmeSection struct {
	name    string
	meaning string
}

// The README's Tasks, Notes and Done sections and the Running subsection of Tasks, declared here alone.
var (
	tasksSection   = readmeSection{"Tasks", "Concrete and claimable; only a task can run."}
	runningSection = readmeSection{"Running", "Held by a run; its scope is locked until the run ends."}
	notesSection   = readmeSection{"Notes", "Not tasks: ideas and observations, merged into a task when one is promoted."}
	doneSection    = readmeSection{"Done", "Finished, awaiting `lyx board prune`."}
)

// otherNotesHeading is the Notes subsection for a note whose type label is no longer configured.
const otherNotesHeading = "Other"

// kindName is the capitalised display name of an entry kind.
func kindName(kind string) string {
	if kind == KindTask {
		return "Task"
	}
	return "Note"
}

// typeHeading is the Notes subsection heading of a type label: the name capitalised, with an s appended.
func typeHeading(typeLabel string) string {
	if typeLabel == "" {
		return "s"
	}
	return strings.ToUpper(typeLabel[:1]) + typeLabel[1:] + "s"
}

// noteType is the first configured type label t carries, in types order, or "" when it carries none.
func noteType(t Task, types []string) string {
	for _, typeLabel := range types {
		if slices.Contains(t.Labels, typeLabel) {
			return typeLabel
		}
	}
	return ""
}

// metaLine is `slug` · [middle ·] [labels ·] [status], the design doc header's metadata line.
func metaLine(t Task, middle ...string) string {
	return metaLineWithSlug(t, "`"+t.Slug+"`", middle...)
}

// metaLineWithSlug is metaLine with the slug already rendered, so the README can make it a link.
func metaLineWithSlug(t Task, slug string, middle ...string) string {
	parts := append([]string{slug}, middle...)
	if len(t.Labels) > 0 {
		parts = append(parts, strings.Join(t.Labels, ", "))
	}
	if t.Status != nil {
		parts = append(parts, *t.Status)
	}
	return strings.Join(parts, " · ")
}

// renderTasksSection builds the README: a title, an intro, Tasks split into Running and dependency layers, Notes split by type label, then Done when any entry is done.
func renderTasksSection(ordered []TaskWithLayer, designPrefix string, types []string) string {
	lines := []string{
		"# Board",
		"",
		"Tasks are grouped by dependency layer and notes by type.",
		"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.",
		"",
	}

	dependents := openDependents(ordered)
	finished := make(map[string]bool)
	for _, twl := range ordered {
		finished[twl.Slug] = isDone(twl.Task)
	}
	writeEntries := func(entries []TaskWithLayer) {
		for _, twl := range entries {
			var after []string
			for _, dep := range twl.DependsOn {
				if !finished[dep] {
					after = append(after, dep)
				}
			}
			var before []string
			if !finished[twl.Slug] {
				before = dependents[twl.Slug]
			}
			lines = append(lines, renderEntry(twl.Task, after, before, designPrefix)...)
		}
		lines = append(lines, "")
	}

	lines = append(lines, "## "+tasksSection.name, "", tasksSection.meaning, "")
	// A task a run holds goes under Running, split on the run lock's own predicate so README and lock agree.
	var running, tasks []TaskWithLayer
	for _, twl := range ordered {
		if isDone(twl.Task) || twl.Kind != KindTask {
			continue
		}
		if IsRunStatus(twl.Status) {
			running = append(running, twl)
		} else {
			tasks = append(tasks, twl)
		}
	}
	if len(running) > 0 {
		lines = append(lines, "### "+runningSection.name, "", runningSection.meaning, "")
		writeEntries(running)
	}
	for start := 0; start < len(tasks); {
		end := start
		for end < len(tasks) && tasks[end].Layer == tasks[start].Layer {
			end++
		}
		layer := layerSection(tasks[start].Layer)
		lines = append(lines, "### "+layer.name, "", layer.meaning, "")
		writeEntries(tasks[start:end])
		start = end
	}

	lines = append(lines, "## "+notesSection.name, "", notesSection.meaning, "")
	byType := make(map[string][]TaskWithLayer)
	for _, twl := range ordered {
		if !isDone(twl.Task) && twl.Kind == KindNote {
			typeLabel := noteType(twl.Task, types)
			byType[typeLabel] = append(byType[typeLabel], twl)
		}
	}
	for _, typeLabel := range append(slices.Clone(types), "") {
		entries := byType[typeLabel]
		if len(entries) == 0 {
			continue
		}
		heading := otherNotesHeading
		if typeLabel != "" {
			heading = typeHeading(typeLabel)
		}
		lines = append(lines, "### "+heading, "")
		writeEntries(entries)
	}

	var done []TaskWithLayer
	for _, twl := range ordered {
		if isDone(twl.Task) {
			done = append(done, twl)
		}
	}
	if len(done) > 0 {
		lines = append(lines, "## "+doneSection.name, "", doneSection.meaning, "")
		writeEntries(done)
	}

	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// layerSection names a computed dependency layer and says when its entries can start.
func layerSection(layer string) readmeSection {
	switch layer {
	case "A":
		return readmeSection{"Layer A", "Waits on nothing that is not running; next to start."}
	case isolatedLayer:
		return readmeSection{"Independent", "Depends on nothing and nothing depends on it, by design."}
	default:
		return readmeSection{"Layer " + layer, "Starts when every entry it names under After is done."}
	}
}

// openDependents maps each slug to the slugs of the open entries that depend on it, in README order.
func openDependents(ordered []TaskWithLayer) map[string][]string {
	dependents := make(map[string][]string)
	for _, twl := range ordered {
		if isDone(twl.Task) {
			continue
		}
		for _, dep := range twl.DependsOn {
			dependents[dep] = append(dependents[dep], twl.Slug)
		}
	}
	return dependents
}

// renderEntry builds the lines of one numbered README item, whose title line links the slug to its design doc when the entry has a body.
// after and before name the open entries it waits on and that wait on it, so a finished dependency drops out of both.
func renderEntry(t Task, after, before []string, designPrefix string) []string {
	slug := "`" + t.Slug + "`"
	if t.Body != "" {
		slug = fmt.Sprintf("[`%s`](%s%s.md)", t.Slug, designPrefix, t.Slug)
	}
	lines := []string{fmt.Sprintf("1. **%s** — %s", t.Title, metaLineWithSlug(t, slug))}

	// Each detail is its own sub-item: markdown joins plain continuation lines into one paragraph.
	if t.Brief != "" {
		lines = append(lines, "   - "+t.Brief)
	}
	if len(after) > 0 {
		lines = append(lines, "   - **After:** "+codeList(after))
	}
	if len(before) > 0 {
		lines = append(lines, "   - **Before:** "+codeList(before))
	}
	return lines
}

// codeList joins slugs as comma-separated code spans.
func codeList(slugs []string) string {
	quoted := make([]string, len(slugs))
	for i, s := range slugs {
		quoted[i] = "`" + s + "`"
	}
	return strings.Join(quoted, ", ")
}

// renderDesigns returns one design-doc file entry per entry with a non-empty body, using the configured design prefix.
// The doc is a header (title, metadata line, dependencies) followed by the body byte-for-byte.
func renderDesigns(entries []Task, designPrefix string) map[string]string {
	hasBody := make(map[string]bool, len(entries))
	for _, e := range entries {
		hasBody[e.Slug] = e.Body != ""
	}

	designs := make(map[string]string)
	for _, e := range entries {
		if e.Body == "" {
			continue
		}
		header := []string{"# " + e.Title, "", metaLine(e, kindName(e.Kind)), ""}
		if len(e.DependsOn) > 0 {
			deps := make([]string, len(e.DependsOn))
			for i, d := range e.DependsOn {
				if hasBody[d] {
					deps[i] = fmt.Sprintf("[`%s`](%s%s.md)", d, designPrefix, d)
				} else {
					deps[i] = "`" + d + "`"
				}
			}
			header = append(header, "Depends on: "+strings.Join(deps, ", "), "")
		}
		designs[fmt.Sprintf("%s%s.md", designPrefix, e.Slug)] = strings.Join(header, "\n") + "\n" + e.Body
	}
	return designs
}
