// render.go — turns the entry list into the wiki's output files.
//
// Render is a pure function: entries in, a map of filename → content out (a single README.md built by renderReadme, plus design-*.md for any entry with a body).
// The README reads like a roadmap: Tasks split into Running, Ready, dependency layers and Independent, Notes with one subsection per type label, then Done.
// Each subsection is one markdown table numbered from 1, whose rows show the linked slug, the bold title over the brief, and the labels that are not type labels;
// Running adds where its run stands, and Ready and the layers add the open entries each waits on.
// The section names, their meaning lines and the table columns are declared here alone;
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
		out.Readme: renderReadme(ordered, out.DesignPrefix, out.Types),
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

// The README's Tasks, Notes and Done sections and the Running and Ready subsections of Tasks, declared here alone.
var (
	tasksSection   = readmeSection{"Tasks", "Concrete and claimable; only a task can run."}
	runningSection = readmeSection{"Running", "Held by a run; its scope is locked until the run ends."}
	readySection   = readmeSection{"Ready", "Waits on nothing open; can start now."}
	notesSection   = readmeSection{"Notes", "Not tasks: ideas and observations, merged into a task when one is promoted."}
	doneSection    = readmeSection{"Done", "Finished, awaiting `lyx board prune`."}
)

// otherNotesHeading is the Notes subsection for a note whose type label is no longer configured.
const otherNotesHeading = "Other"

// emptySectionLine stands in for the table of a subsection that is always rendered and has no entry.
const emptySectionLine = "_None._"

// The README table columns: each table opens with the row number, the linked slug and the entry, and closes with its labels.
const (
	columnNumber = "#"
	columnSlug   = "Slug"
	columnTask   = "Task"
	columnNote   = "Note"
	columnEntry  = "Entry"
	columnAt     = "At"
	columnAfter  = "After"
	columnLabels = "Labels"
)

// runningState is the run state the At cell leaves out, since the Running section already says it.
const runningState = "running"

// cellLineBreak puts the brief under the title inside one table cell.
const cellLineBreak = "<br>"

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
	parts := append([]string{"`" + t.Slug + "`"}, middle...)
	if len(t.Labels) > 0 {
		parts = append(parts, strings.Join(t.Labels, ", "))
	}
	if t.Status != nil {
		parts = append(parts, *t.Status)
	}
	return strings.Join(parts, " · ")
}

// readmeTable is one README subsection's table: its middle column headers, and the cells under them per entry.
type readmeTable struct {
	columns []string
	cells   func(t Task) []string
}

// renderReadme builds the README: a title, an intro, Tasks split into Running, Ready, dependency layers and Independent, Notes split by type label, then Done when any entry is done.
func renderReadme(ordered []TaskWithLayer, designPrefix string, types []string) string {
	lines := []string{
		"# Board",
		"",
		"Tasks are grouped by dependency layer and notes by type.",
		"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.",
		"",
	}

	finished := make(map[string]bool)
	for _, twl := range ordered {
		finished[twl.Slug] = isDone(twl.Task)
	}
	writeTable := func(entries []TaskWithLayer, table readmeTable) {
		header := append(append([]string{columnNumber, columnSlug}, table.columns...), columnLabels)
		lines = append(lines, tableRow(header), tableRow(slices.Repeat([]string{"---"}, len(header))))
		for i, twl := range entries {
			row := append([]string{fmt.Sprint(i + 1), slugCell(twl.Task, designPrefix)}, table.cells(twl.Task)...)
			lines = append(lines, tableRow(append(row, labelsCell(twl.Task, types))))
		}
		lines = append(lines, "")
	}
	waitTable := readmeTable{[]string{columnTask, columnAfter}, func(t Task) []string {
		var after []string
		for _, dep := range t.DependsOn {
			if !finished[dep] {
				after = append(after, dep)
			}
		}
		return []string{entryCell(t), codeList(after)}
	}}

	lines = append(lines, "## "+tasksSection.name, "", tasksSection.meaning, "")
	var tasks []TaskWithLayer
	for _, twl := range ordered {
		if !isDone(twl.Task) && twl.Kind == KindTask {
			tasks = append(tasks, twl)
		}
	}
	byLayer := func(layer string) []TaskWithLayer {
		var entries []TaskWithLayer
		for _, twl := range tasks {
			if twl.Layer == layer {
				entries = append(entries, twl)
			}
		}
		return entries
	}
	if running := byLayer(runningLayer); len(running) > 0 {
		lines = append(lines, "### "+runningSection.name, "", runningSection.meaning, "")
		writeTable(running, readmeTable{[]string{columnTask, columnAt}, func(t Task) []string {
			return []string{entryCell(t), atCell(*t.Status)}
		}})
	}
	lines = append(lines, "### "+readySection.name, "", readySection.meaning, "")
	if ready := byLayer(readyLayer); len(ready) > 0 {
		writeTable(ready, waitTable)
	} else {
		lines = append(lines, emptySectionLine, "")
	}
	// RenderOrder sorts tasks by layer, so each dependency layer and Independent is a contiguous run.
	for start := 0; start < len(tasks); {
		end := start
		for end < len(tasks) && tasks[end].Layer == tasks[start].Layer {
			end++
		}
		if layer := tasks[start].Layer; layer != runningLayer && layer != readyLayer {
			section := layerSection(layer)
			lines = append(lines, "### "+section.name, "", section.meaning, "")
			writeTable(tasks[start:end], waitTable)
		}
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
	noteTable := readmeTable{[]string{columnNote}, func(t Task) []string { return []string{entryCell(t)} }}
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
		writeTable(entries, noteTable)
	}

	var done []TaskWithLayer
	for _, twl := range ordered {
		if isDone(twl.Task) {
			done = append(done, twl)
		}
	}
	if len(done) > 0 {
		lines = append(lines, "## "+doneSection.name, "", doneSection.meaning, "")
		writeTable(done, readmeTable{[]string{columnEntry}, func(t Task) []string { return []string{entryCell(t)} }})
	}

	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// layerSection names a computed dependency layer and says when its entries can start.
func layerSection(layer string) readmeSection {
	switch layer {
	case "A":
		return readmeSection{"Layer A", "Waits only on Running or Ready entries."}
	case isolatedLayer:
		return readmeSection{"Independent", "Depends on nothing and nothing depends on it, by design."}
	default:
		return readmeSection{"Layer " + layer, "Starts when every entry it names under After is done."}
	}
}

// tableRow joins cells into one markdown table row.
func tableRow(cells []string) string {
	return "| " + strings.Join(cells, " | ") + " |"
}

// cellText makes s safe inside one table cell: a pipe is escaped, and a line break becomes a space so the row stays on one line.
func cellText(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\r\n", "\n")), " ")
}

// slugCell is the slug as a code span, linked to its design doc when the entry has a body.
func slugCell(t Task, designPrefix string) string {
	if t.Body != "" {
		return fmt.Sprintf("[`%s`](%s%s.md)", t.Slug, designPrefix, t.Slug)
	}
	return "`" + t.Slug + "`"
}

// entryCell is the bold title, with the brief under it when there is one.
func entryCell(t Task) string {
	cell := "**" + cellText(t.Title) + "**"
	if t.Brief != "" {
		cell += cellLineBreak + cellText(t.Brief)
	}
	return cell
}

// atCell is where a run stands: the producer alone while it is running, else the whole run status.
func atCell(status string) string {
	if state, producer, _ := strings.Cut(status, runStatusSeparator); state == runningState {
		return cellText(producer)
	}
	return cellText(status)
}

// labelsCell lists the labels of t that are not type labels; the section shows a note's type.
func labelsCell(t Task, types []string) string {
	var labels []string
	for _, label := range t.Labels {
		if !slices.Contains(types, label) {
			labels = append(labels, label)
		}
	}
	return cellText(strings.Join(labels, ", "))
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
