// render.go — turns the entry list into the wiki's output files.
//
// Render is a pure function: entries in, a map of filename → content out (README.md for the root project and project-<slug>.md for each project, both built by renderBoardFile, plus design-*.md for any entry with a body).
// Each board file reads like a roadmap of its project's own entries: the projects directly under it, Tasks split into Running, Ready, dependency layers, Independent and Draft, Notes with one subsection per type label, research first, then Done.
// Each subsection holds one markdown table in RenderOrder's order, each row opening with the entry's handle, and a Notes table adds the priority as H or L after it;
// README.md opens with the time it was rendered and the machine that owns the hub's prime, and a project file with the time, a link up to its parent's file and the project's brief, kind and owner;
// the rows show the bold title with the brief, the run profile and each run setting as a bullet under it, the linked slug, the entry's age above its time since its last revision as of that time, and the labels that are not type labels;
// every Tasks table adds the recipe after the slug, loom for an empty one;
// Running adds where its run stands and the entry's recorded owner machine, and Ready and the layers add the handles of the open entries each waits on.
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

// Render produces the board output files from the entry list and the machine that owns the hub's prime, "" when none is recorded, every board file's ages as of now.
// The configured README is the root project's file, and every project, open or done, gets a file of its own named by projectFileName.
func Render(tasks []Task, primeOwner string, out Outputs, now time.Time) (map[string]string, error) {
	ordered, err := RenderOrder(tasks)
	if err != nil {
		return nil, err
	}

	board := newBoardView(ordered, primeOwner, out, now)
	result := map[string]string{
		out.Readme: board.renderBoardFile(Task{}),
	}
	for _, twl := range ordered {
		if twl.Kind == KindProject {
			result[projectFileName(twl.Slug)] = board.renderBoardFile(twl.Task)
		}
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

// projectsHeading names the section listing the projects directly under a board file's project; it has no meaning line, since most projects only group entries.
const projectsHeading = "Projects"

// The board files' Tasks, Notes and Done sections and the Running and Ready subsections of Tasks, declared here alone.
var (
	tasksSection   = readmeSection{"Tasks", "Concrete and claimable; only a task can run."}
	runningSection = readmeSection{"Running", "Held by a run; its scope is locked until the run ends."}
	readySection   = readmeSection{"Ready", "Waits on nothing open; can start now."}
	notesSection   = readmeSection{"Notes", "Not tasks: ideas and observations, merged into a task when one is promoted."}
	doneSection    = readmeSection{"Done", "Finished, awaiting `lym board prune --apply`."}
)

// otherNotesHeading is the Notes subsection for a note whose type label is no longer configured.
const otherNotesHeading = "Other"

// researchType is the type label whose Notes subsection leads the others, headed without a plural s.
const researchType = "research"

// researchHeading is the Notes subsection heading of researchType.
const researchHeading = "Research"

// projectFilePrefix starts the file name of a project's own board file, which its slug and .md complete.
// It must equal the project file prefix the records replay in internal/fabricengine recognises.
const projectFilePrefix = "project-"

// rootTitle is the H1 of the root project's board file and the text of a link up to it.
const rootTitle = "Board"

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

// typeHeading is the Notes subsection heading of a type label: Research for research, else the name capitalised, with an s appended.
func typeHeading(typeLabel string) string {
	if typeLabel == researchType {
		return researchHeading
	}
	if typeLabel == "" {
		return "s"
	}
	return strings.ToUpper(typeLabel[:1]) + typeLabel[1:] + "s"
}

// notesTypeOrder is the order of the Notes subsections: research first when it is configured, then the other types in configured order, then "" for Other.
func notesTypeOrder(types []string) []string {
	var order []string
	if slices.Contains(types, researchType) {
		order = append(order, researchType)
	}
	for _, typeLabel := range types {
		if typeLabel != researchType {
			order = append(order, typeLabel)
		}
	}
	return append(order, "")
}

// projectFileName is the name of the board file that renders the project slug and the entries under it.
func projectFileName(slug string) string {
	return projectFilePrefix + slug + ".md"
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

// readmeOwners returns the effective owner of each rendered entry by slug.
// A parent resolves through ParentBeam over the rendered entries, and primeOwner is the root's.
func readmeOwners(ordered []TaskWithLayer, primeOwner string) func(slug string) string {
	tasks := make([]Task, len(ordered))
	for i, twl := range ordered {
		tasks[i] = twl.Task
	}
	owners := ownersOf(primeOwner, tasks)
	parentOf := func(slug string) (string, error) { return ParentBeam(tasks, slug), nil }
	return func(slug string) string {
		owner, _ := owners.Effective(slug, parentOf)
		return owner
	}
}

// boardView is the whole board as every board file reads it: the entries in RenderOrder's order, with the facts each file looks up across projects.
type boardView struct {
	ordered    []TaskWithLayer
	bySlug     map[string]TaskWithLayer
	primeOwner string
	ownerOf    func(slug string) string
	out        Outputs
	now        time.Time
}

// newBoardView indexes ordered for rendering the board files with the given prime owner and outputs, every age as of now.
func newBoardView(ordered []TaskWithLayer, primeOwner string, out Outputs, now time.Time) boardView {
	bySlug := make(map[string]TaskWithLayer, len(ordered))
	for _, twl := range ordered {
		bySlug[twl.Slug] = twl
	}
	return boardView{
		ordered:    ordered,
		bySlug:     bySlug,
		primeOwner: primeOwner,
		ownerOf:    readmeOwners(ordered, primeOwner),
		out:        out,
		now:        now,
	}
}

// fileLink is a markdown link to the board file of project, the zero Task naming the root, its text the project's handle and title.
func (b boardView) fileLink(project Task) string {
	if project.Slug == "" {
		return "[" + rootTitle + "](" + b.out.Readme + ")"
	}
	return "[" + cellText(project.Handle+" "+project.Title) + "](" + projectFileName(project.Slug) + ")"
}

// fileHeader is the opening of project's board file, the zero Task naming the root: the title, the render time, then the prime owner for the root, or for a project a link up to its parent's file, its brief, whether it is a beam or a grouping and a beam's effective owner.
func (b boardView) fileHeader(project Task) []string {
	rendered := "Rendered " + b.now.UTC().Format(renderedTimeLayout) + "; every age below is as of then."
	if project.Slug == "" {
		return []string{"# " + rootTitle, "", rendered, "Prime owner: " + primeOwnerText(b.primeOwner) + ".", ""}
	}
	lines := []string{"# " + cellText(project.Handle+" "+project.Title), "", rendered, "Under: " + b.fileLink(b.bySlug[project.Project].Task) + ".", ""}
	if project.Brief != "" {
		lines = append(lines, project.Brief, "")
	}
	if !project.Beam {
		return append(lines, projectGroupingText, "")
	}
	return append(lines, projectBeamText, "", "Owner: "+primeOwnerText(b.ownerOf(project.Slug))+".", "")
}

// renderBoardFile builds the board file of project, the zero Task naming the root: its header, an intro, Projects when an open project sits directly under it, Tasks split into Running, Ready, dependency layers, Independent and Draft, Notes split by type label, then Done when any of its entries is done.
// Every section holds only the entries whose project is this file's project, while layers, After cells and owners read the whole board.
func (b boardView) renderBoardFile(project Task) string {
	lines := append(b.fileHeader(project),
		"Tasks are grouped by dependency layer and notes by type.",
		"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.",
		"",
	)

	var own []TaskWithLayer
	for _, twl := range b.ordered {
		if twl.Project == project.Slug {
			own = append(own, twl)
		}
	}
	designPrefix, types, now := b.out.DesignPrefix, b.out.Types, b.now
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
		return []string{entryCell(t), recipeCell(t), b.afterCell(t)}
	}}

	lines = append(lines, b.projectsSection(own, project)...)

	var tasks []TaskWithLayer
	for _, twl := range own {
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
			return []string{entryCell(t), recipeCell(t), atCell(*t.Status), cellText(b.ownerOf(t.Slug))}
		}})
	}
	ready := byLayer(readyLayer)
	lines = append(lines, countedHeading("###", readySection.name, len(ready)), "", readySection.meaning, "")
	if len(ready) > 0 {
		writeTable(ready, waitTable)
	} else {
		lines = append(lines, emptySectionLine, "")
	}
	// RenderOrder sorts tasks by layer, so each dependency layer, Independent and Draft is a contiguous run.
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
	for _, twl := range own {
		if !isDone(twl.Task) && twl.Kind == KindNote {
			typeLabel := noteType(twl.Task, types)
			byType[typeLabel] = append(byType[typeLabel], twl)
			noteCount++
		}
	}
	lines = append(lines, countedHeading("##", notesSection.name, noteCount), "", notesSection.meaning, "")
	noteTable := readmeTable{columns: []string{columnNote}, cells: func(t Task) []string { return []string{entryCell(t)} }, priority: true}
	for _, typeLabel := range notesTypeOrder(types) {
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
	for _, twl := range own {
		if isDone(twl.Task) {
			done = append(done, twl)
		}
	}
	if len(done) > 0 {
		lines = append(lines, countedHeading("##", doneSection.name, len(done)), "", doneSection.meaning, "")
		writeTable(done, readmeTable{columns: []string{columnEntry}, cells: func(t Task) []string {
			if t.Kind == KindProject {
				return []string{"**[" + cellText(t.Title) + "](" + projectFileName(t.Slug) + ")**"}
			}
			return []string{entryCell(t)}
		}})
	}

	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// projectsSection builds the Projects section of project's board file from own, the entries directly under it: a heading counting its open projects, then one bullet per open project linking to its file, followed by its brief.
// The projects follow project's order, then by handle.
// It returns no lines when no open project sits under it.
func (b boardView) projectsSection(own []TaskWithLayer, project Task) []string {
	var children []TaskWithLayer
	for _, twl := range own {
		if !isDone(twl.Task) && twl.Kind == KindProject {
			children = append(children, twl)
		}
	}
	if len(children) == 0 {
		return nil
	}
	position := func(slug string) int {
		if index := slices.Index(project.Order, slug); index >= 0 {
			return index
		}
		return len(project.Order)
	}
	slices.SortStableFunc(children, func(x, y TaskWithLayer) int {
		if c := position(x.Slug) - position(y.Slug); c != 0 {
			return c
		}
		if c := compareHandles(x.Handle, y.Handle); c != 0 {
			return c
		}
		return x.ID - y.ID
	})

	lines := []string{countedHeading("##", projectsHeading, len(children)), ""}
	for _, child := range children {
		line := "- " + b.fileLink(child.Task)
		if child.Brief != "" {
			line += " — " + cellText(child.Brief)
		}
		lines = append(lines, line)
	}
	return append(lines, "")
}

// afterCell is the handles of the open entries t depends on, one per line in depends_on order, a dependency the board holds no handle for shown by its slug.
func (b boardView) afterCell(t Task) string {
	var after []string
	for _, dep := range t.DependsOn {
		entry, found := b.bySlug[dep]
		switch {
		case found && isDone(entry.Task):
			// A done dependency no longer holds t back.
		case found && entry.Handle != "":
			after = append(after, cellText(entry.Handle))
		default:
			after = append(after, codeSpan(dep))
		}
	}
	return strings.Join(after, cellBreak)
}

// primeOwnerText is a board file's statement of the prime owner: the machine in a code span, or none recorded.
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
		return readmeSection{"Layer A", "Waits only on Running, Ready or Draft entries."}
	case isolatedLayer:
		return readmeSection{"Independent", "Depends on nothing and nothing depends on it, by design."}
	case draftLayer:
		return readmeSection{"Draft", "Not released; nothing starts it until its status is cleared."}
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

// entryCell is the bold title, led by a `draft` code span for a draft task, with the brief as one bullet under it when there is one, then a code-span bullet naming the run profile when there is one and one per run setting.
func entryCell(t Task) string {
	cell := "**" + cellText(t.Title) + "**"
	if isDraft(t) {
		cell = codeSpan(StatusDraft) + " " + cell
	}
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
