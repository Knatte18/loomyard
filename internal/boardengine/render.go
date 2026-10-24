// render.go — turns the entry list into the wiki's output files.
//
// Render is a pure function: entries in, a map of filename → content out (a single README.md built by renderReadme, plus design-*.md for any entry with a body).
// The README reads like a roadmap: Tasks split into Running, Ready, dependency layers and Independent, Notes with one subsection per type label, then Done.
// Each subsection holds one markdown table in RenderOrder's order, each row opening with the entry's handle, and a Notes table adds the priority as H or L after it;
// the README opens with the time it was rendered and the machine that owns the hub's prime;
// the rows show the bold title with the brief, the run profile and each run setting as a bullet under it, the linked slug, the entry's age above its time since its last revision as of that time, and the labels that are not type labels;
// every Tasks table adds the recipe after the slug, loom for an empty one;
// Running adds where its run stands and the entry's recorded owner machine, and Ready and the layers add the open entries each waits on.
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
	"time"

	"github.com/Knatte18/loomyard/internal/fsx"
)

// renderManifestFile is the name of the sidecar that records the filenames the
// last render produced. It is gitignored (via ensureLockfilesIgnored) so it never
// adds commit churn, and it is never itself a member of the rendered file set.
const renderManifestFile = ".board-rendered.json"

// RenderToDisk renders the entries and the prime owner with their ages as of now, persisting the board's output files.
func RenderToDisk(boardPath string, tasks []Task, primeOwner string, out Outputs, now time.Time) error {
	files, err := Render(tasks, primeOwner, out, now)
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

// Render produces the board output files from the entry list and the machine that owns the hub's prime, "" when none is recorded, the README's ages as of now.
func Render(tasks []Task, primeOwner string, out Outputs, now time.Time) (map[string]string, error) {
	ordered, err := RenderOrder(tasks)
	if err != nil {
		return nil, err
	}

	result := map[string]string{
		out.Readme: renderReadme(ordered, primeOwner, out.DesignPrefix, out.Types, now),
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
	projectsSection = readmeSection{"Projects", "Groups of entries; a beam spawns its members from its own worktree."}
	tasksSection    = readmeSection{"Tasks", "Concrete and claimable; only a task can run."}
	runningSection  = readmeSection{"Running", "Held by a run; its scope is locked until the run ends."}
	readySection    = readmeSection{"Ready", "Waits on nothing open; can start now."}
	notesSection    = readmeSection{"Notes", "Not tasks: ideas and observations, merged into a task when one is promoted."}
	doneSection     = readmeSection{"Done", "Finished, awaiting `lyx board prune --apply`."}
)

// otherNotesHeading is the Notes subsection for a note whose type label is no longer configured.
const otherNotesHeading = "Other"

// emptySectionLine stands in for the table of a subsection that is always rendered and has no entry.
const emptySectionLine = "_None._"

// The README table columns: each table opens with the handle, the entry and the linked slug, and closes with the entry's age above the time since its last revision, and its labels.
const (
	columnHandle     = "#"
	columnPriority   = "P"
	columnSlug       = "Slug"
	columnRecipe     = "Recipe"
	columnTask       = "Task"
	columnNote       = "Note"
	columnEntry      = "Entry"
	columnAt         = "At"
	columnOwner      = "Owner"
	columnAfter      = "After"
	columnMember     = "Member"
	columnKind       = "Kind"
	columnStanding   = "Standing"
	columnAgeRevised = "Age" + cellBreak + "Revised"
	columnLabels     = "Labels"
)

// The lines under a project's heading that say whether it is a beam or only a grouping.
const (
	projectBeamText     = "beam"
	projectGroupingText = "grouping"
)

// cellBreak stacks two lines inside one table cell, where a newline would end the row.
const cellBreak = "<br>"

// renderedTimeLayout is the UTC date and time the README states it was rendered at.
const renderedTimeLayout = "2006-01-02 15:04 UTC"

// defaultRecipe is the recipe the Recipe cell shows for a task with no recipe, since an empty recipe runs loom.
const defaultRecipe = "loom"

// runningState is the run state the At cell leaves out, since the Running section already says it.
const runningState = "running"

// cellBullet starts one detail line under the title inside one table cell; a markdown list does not render in a table cell.
const cellBullet = cellBreak + "• "

// countedHeading is a README heading of the given markdown level, its name followed by the number of entries the section lists.
func countedHeading(level, name string, count int) string {
	return fmt.Sprintf("%s %s (%d)", level, name, count)
}

// kindName is the capitalised display name of an entry kind.
func kindName(kind string) string {
	switch kind {
	case KindTask:
		return "Task"
	case KindProject:
		return "Project"
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

// readmeTable is one README subsection's table: its middle column headers, the entry column first, and the cells under them per entry.
// priority adds the priority column after the handle.
type readmeTable struct {
	columns  []string
	cells    func(t Task) []string
	priority bool
}

// renderReadme builds the README: a title, the render time and prime owner, an intro, Projects when one is open, Tasks split into Running, Ready, dependency layers and Independent, Notes split by type label, then Done when any entry is done.
func renderReadme(ordered []TaskWithLayer, primeOwner, designPrefix string, types []string, now time.Time) string {
	lines := []string{
		"# Board",
		"",
		"Rendered " + now.UTC().Format(renderedTimeLayout) + "; every age below is as of then.",
		"Prime owner: " + primeOwnerText(primeOwner) + ".",
		"",
		"Tasks are grouped by dependency layer and notes by type.",
		"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.",
		"",
	}

	finished := make(map[string]bool)
	for _, twl := range ordered {
		finished[twl.Slug] = isDone(twl.Task)
	}
	// writeTable writes one table, a row per entry in the order given.
	writeTable := func(entries []TaskWithLayer, table readmeTable) {
		lead := []string{columnHandle}
		if table.priority {
			lead = append(lead, columnPriority)
		}
		// The slug goes second among the middle columns, right after the entry.
		header := append(append(lead, slices.Insert(slices.Clone(table.columns), 1, columnSlug)...), columnAgeRevised, columnLabels)
		lines = append(lines, tableRow(header), tableRow(slices.Repeat([]string{"---"}, len(header))))
		for _, twl := range entries {
			row := []string{cellText(twl.Handle)}
			if table.priority {
				row = append(row, priorityCell(twl.Priority))
			}
			row = append(row, slices.Insert(table.cells(twl.Task), 1, slugCell(twl.Task, designPrefix))...)
			row = append(row, ageCell(twl.Task, now), labelsCell(twl.Task, types))
			lines = append(lines, tableRow(row))
		}
		lines = append(lines, "")
	}
	waitTable := readmeTable{columns: []string{columnTask, columnRecipe, columnAfter}, cells: func(t Task) []string {
		var after []string
		for _, dep := range t.DependsOn {
			if !finished[dep] {
				after = append(after, dep)
			}
		}
		return []string{entryCell(t), recipeCell(t), codeList(after)}
	}}

	lines = append(lines, renderProjects(ordered, finished, primeOwner, designPrefix, types, now)...)

	var tasks []TaskWithLayer
	for _, twl := range ordered {
		if !isDone(twl.Task) && twl.Kind == KindTask {
			tasks = append(tasks, twl)
		}
	}
	lines = append(lines, countedHeading("##", tasksSection.name, len(tasks)), "", tasksSection.meaning, "")
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
		lines = append(lines, countedHeading("###", runningSection.name, len(running)), "", runningSection.meaning, "")
		writeTable(running, readmeTable{columns: []string{columnTask, columnRecipe, columnAt, columnOwner}, cells: func(t Task) []string {
			return []string{entryCell(t), recipeCell(t), atCell(*t.Status), cellText(t.OwnerMachine)}
		}})
	}
	ready := byLayer(readyLayer)
	lines = append(lines, countedHeading("###", readySection.name, len(ready)), "", readySection.meaning, "")
	if len(ready) > 0 {
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
			lines = append(lines, countedHeading("###", section.name, end-start), "", section.meaning, "")
			writeTable(tasks[start:end], waitTable)
		}
		start = end
	}

	byType := make(map[string][]TaskWithLayer)
	noteCount := 0
	for _, twl := range ordered {
		if !isDone(twl.Task) && twl.Kind == KindNote {
			typeLabel := noteType(twl.Task, types)
			byType[typeLabel] = append(byType[typeLabel], twl)
			noteCount++
		}
	}
	lines = append(lines, countedHeading("##", notesSection.name, noteCount), "", notesSection.meaning, "")
	noteTable := readmeTable{columns: []string{columnNote}, cells: func(t Task) []string { return []string{entryCell(t)} }, priority: true}
	for _, typeLabel := range append(slices.Clone(types), "") {
		entries := byType[typeLabel]
		if len(entries) == 0 {
			continue
		}
		heading := otherNotesHeading
		if typeLabel != "" {
			heading = typeHeading(typeLabel)
		}
		lines = append(lines, countedHeading("###", heading, len(entries)), "")
		writeTable(entries, noteTable)
	}

	var done []TaskWithLayer
	for _, twl := range ordered {
		if isDone(twl.Task) {
			done = append(done, twl)
		}
	}
	if len(done) > 0 {
		lines = append(lines, countedHeading("##", doneSection.name, len(done)), "", doneSection.meaning, "")
		writeTable(done, readmeTable{columns: []string{columnEntry}, cells: func(t Task) []string { return []string{entryCell(t)} }})
	}

	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// renderProjects builds the Projects section: a heading counting the open projects, then per project its handle and title, brief, whether it is a beam or a grouping, a beam's effective owner and a table of its members.
// A member is an entry whose project names the project; the project's order lists them first, and the members it leaves out follow by handle.
// It returns no lines when no project is open.
func renderProjects(ordered []TaskWithLayer, finished map[string]bool, primeOwner, designPrefix string, types []string, now time.Time) []string {
	var projects []TaskWithLayer
	for _, twl := range ordered {
		if !isDone(twl.Task) && twl.Kind == KindProject {
			projects = append(projects, twl)
		}
	}
	if len(projects) == 0 {
		return nil
	}

	lines := []string{countedHeading("##", projectsSection.name, len(projects)), "", projectsSection.meaning, ""}
	for _, project := range projects {
		lines = append(lines, "### "+cellText(project.Handle)+" "+cellText(project.Title), "")
		if project.Brief != "" {
			lines = append(lines, project.Brief, "")
		}
		if !project.Beam {
			lines = append(lines, projectGroupingText, "")
		} else {
			owner := project.OwnerMachine
			if owner == "" {
				owner = primeOwner
			}
			lines = append(lines, projectBeamText, "", "Owner: "+primeOwnerText(owner)+".", "")
		}

		members := projectMembers(ordered, project.Task)
		if len(members) == 0 {
			lines = append(lines, emptySectionLine, "")
			continue
		}
		header := []string{columnHandle, columnMember, columnKind, columnStanding}
		lines = append(lines, tableRow(header), tableRow(slices.Repeat([]string{"---"}, len(header))))
		for _, member := range members {
			lines = append(lines, tableRow([]string{
				cellText(member.Handle),
				"**" + cellText(member.Title) + "**",
				kindName(member.Kind),
				standingCell(member, finished[member.Slug]),
			}))
		}
		lines = append(lines, "")
	}
	return lines
}

// projectMembers is the entries of ordered whose project is project, the slugs in project's order first and the rest by handle.
func projectMembers(ordered []TaskWithLayer, project Task) []TaskWithLayer {
	var members []TaskWithLayer
	for _, twl := range ordered {
		if twl.Project == project.Slug {
			members = append(members, twl)
		}
	}
	position := func(slug string) int {
		if index := slices.Index(project.Order, slug); index >= 0 {
			return index
		}
		return len(project.Order)
	}
	slices.SortStableFunc(members, func(a, b TaskWithLayer) int {
		if c := position(a.Slug) - position(b.Slug); c != 0 {
			return c
		}
		if c := compareHandles(a.Handle, b.Handle); c != 0 {
			return c
		}
		return a.ID - b.ID
	})
	return members
}

// standingCell is where a project member stands: Done once finished, the run's place while a run holds it, Ready, Note, or the name of its dependency layer.
func standingCell(member TaskWithLayer, finished bool) string {
	switch {
	case finished:
		return "Done"
	case member.Kind == KindNote:
		return "Note"
	case member.Layer == runningLayer:
		return atCell(*member.Status)
	case member.Layer == readyLayer:
		return readySection.name
	case member.Layer == projectLayer:
		return kindName(KindProject)
	}
	return layerSection(member.Layer).name
}

// primeOwnerText is the README's statement of the prime owner: the machine in a code span, or none recorded.
func primeOwnerText(primeOwner string) string {
	if primeOwner == "" {
		return "none recorded"
	}
	return codeSpan(primeOwner)
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

// priorityCell is H for a high priority, L for a low one and empty for normal.
func priorityCell(priority string) string {
	switch priority {
	case PriorityHigh:
		return "H"
	case PriorityLow:
		return "L"
	}
	return ""
}

// ageCell is the entry's age as of now above the time since its last revision, empty when both dates are unknown.
func ageCell(t Task, now time.Time) string {
	age, revised := FormatAge(t.Created, now), FormatAge(t.Revised, now)
	if age == "" && revised == "" {
		return ""
	}
	return age + cellBreak + revised
}

// entryCell is the bold title, with the brief as one bullet under it when there is one, then a code-span bullet naming the run profile when there is one and one per run setting.
func entryCell(t Task) string {
	cell := "**" + cellText(t.Title) + "**"
	if t.Brief != "" {
		cell += cellBullet + cellText(t.Brief)
	}
	if t.Profile != "" {
		cell += cellBullet + codeSpan(cellText("profile: "+t.Profile))
	}
	for _, setting := range settingLines(t.RunSettings) {
		cell += cellBullet + codeSpan(cellText(setting))
	}
	return cell
}

// settingLines is one `<module>: <key path> <value>` line per leaf of a task's run settings, nested keys joined by dots, in sorted order.
func settingLines(settings map[string]any) []string {
	var lines []string
	for module, value := range settings {
		lines = appendSettingLeaves(lines, module, "", value)
	}
	slices.Sort(lines)
	return lines
}

// appendSettingLeaves appends one setting line per leaf under value, path being the dotted key path from the module down to value.
func appendSettingLeaves(lines []string, module, path string, value any) []string {
	if nested, ok := value.(map[string]any); ok {
		for key, child := range nested {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			lines = appendSettingLeaves(lines, module, childPath, child)
		}
		return lines
	}
	line := module + ":"
	if path != "" {
		line += " " + path
	}
	return append(lines, line+" "+settingValue(value))
}

// settingValue is a string setting as itself and any other value as its compact JSON, the form the board stores it in.
func settingValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	// The value was decoded from the board's JSON, so it always encodes back.
	// HTML escaping is off so a list holding <, > or & renders as stored.
	var out strings.Builder
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
	return strings.TrimSuffix(out.String(), "\n")
}

// codeSpan wraps s in a code span whose backtick fence is longer than any backtick run in s.
// It pads s with a space on each side when s starts or ends with a backtick.
func codeSpan(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return fence + s + fence
}

// recipeCell is the recipe the task runs, loom when its recipe field is empty.
func recipeCell(t Task) string {
	if t.Recipe == "" {
		return defaultRecipe
	}
	return cellText(t.Recipe)
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
