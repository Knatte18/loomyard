// render.go — turns the entry list into the wiki's output files.
//
// Render is a pure function: entries in, a map of filename → content out (README.md, the overview of every project, built by renderReadme, and project-<slug>.md for each project and project-root.md for the root project P0, built by renderProjectFile, plus design-*.md for any entry with a body).
// Every project, the root P0 included, renders through one project section: its heading linked to its file, facts and brief, then a section per open project directly under it one heading level down, nested to any depth with headings capped at level six, then its own entries;
// with no open sub-project the own entries sit one level down, and with any they sit under an Own entries heading at the sub-projects' level, two levels down;
// the root's projects stand beside it rather than under it, so its section holds its own entries alone.
// README.md holds every top-level project's section under its header and the root's last, and a project file holds its project's sub-projects and own entries under its header.
// A project's own entries split into Tasks, by Running, Ready, dependency layers, Independent, Discussed and Draft, Notes with one subsection per type label, research first, then Done.
// Each subsection holds one markdown table in RenderOrder's order, each row opening with the entry's handle, and a Notes table adds the priority as H or L after it, under Prio;
// README.md opens with the time it was rendered and the machine that owns the hub's prime, and a project file with the time, a link up to its parent's file, the project's brief, for a beam its owner, and the project's body with its headings shifted below the file's own;
// the rows show the bold title with the brief, the run profile and each run setting as a bullet under it, the linked slug with a Sketch link to the decision record under it for a discussed entry, the entry's age above its time since its last revision as of that time, and the labels that are not type labels;
// every Tasks table adds the recipe after the slug, the configured default recipe for an empty one;
// Running adds where its run stands and the entry's recorded owner machine, and Ready and the layers add the handles of the entries each waits on that are not done, a project judged by its landing members.
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
		out.Readme:      board.renderReadme(),
		rootProjectFile: board.renderProjectFile(rootProject),
	}
	for _, twl := range ordered {
		if twl.Kind == KindProject {
			result[projectFileName(twl.Slug)] = board.renderProjectFile(twl.Task)
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

// boardIntro is the paragraph every board file holds under its header.
var boardIntro = []string{
	"Tasks are grouped by dependency layer and notes by type.",
	"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.",
	"",
}

// The heading over a project's own entries beside its sub-projects, the board files' Tasks, Notes and Done sections and the Running and Ready subsections of Tasks, declared here alone.
var (
	ownEntriesSection = readmeSection{"Own entries", "The entries directly under this project and under none of its sub-projects."}
	tasksSection      = readmeSection{"Tasks", "Concrete and claimable; only a task can run."}
	runningSection    = readmeSection{"Running", "Held by a run; its scope is locked until the run ends."}
	readySection      = readmeSection{"Ready", "Waits on nothing open; can start now."}
	notesSection      = readmeSection{"Notes", "Not tasks: ideas and observations, merged into a task when one is promoted."}
	doneSection       = readmeSection{"Done", "Finished, awaiting `lym board prune --apply`."}
)

// otherNotesHeading is the Notes subsection for a note whose type label is no longer configured.
const otherNotesHeading = "Other"

// researchType is the type label whose Notes subsection leads the others, headed without a plural s.
const researchType = "research"

// researchHeading is the Notes subsection heading of researchType.
const researchHeading = "Research"

// projectFilePrefix starts the file name of a project's own board file, which its slug and .md complete.
// The board's Bolt replayer recognises the same prefix as its own.
const projectFilePrefix = "project-"

// rootTitle is the H1 of README.md, the overview of every project, and the text of a link up to it.
const rootTitle = "Board"

// rootProject is the root project as the board files render it: handle P0, which no board project takes since their handles start at 1, and the entries under no other project.
// Its empty slug names the root wherever a project is looked up, and it has no board entry, so no design file.
var rootProject = Task{Handle: projectHandleLetter + "0", Title: "Root: entries under no project"}

// rootProjectFile is the name of the root project's own board file.
var rootProjectFile = projectFileName(RootSubject)

// emptySectionLine stands in for the table of a subsection that is always rendered and has no entry.
const emptySectionLine = "_None._"

// The README table columns: each table opens with the handle, the entry and the linked slug, and closes with the entry's age above the time since its last revision, and its labels.
const (
	columnHandle     = "#"
	columnPriority   = "Prio"
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

// projectBeamText is the line under a beam project's brief that says it is a beam.
const projectBeamText = "beam"

// cellBreak stacks two lines inside one table cell, where a newline would end the row.
const cellBreak = "<br>"

// renderedTimeLayout is the UTC date and time the README states it was rendered at.
const renderedTimeLayout = "2006-01-02 15:04 UTC"

// recipeProfileSeparator stands between the recipe and the run profile in one Recipe cell.
const recipeProfileSeparator = " · "

// runningState is the run state the At cell leaves out, since the Running section already says it.
const runningState = "running"

// cellBullet starts one detail line under the title inside one table cell; a markdown list does not render in a table cell.
const cellBullet = cellBreak + "• "

// maxHeadingLevel is the deepest markdown heading level; a section nested deeper renders at this level.
const maxHeadingLevel = 6

// heading is a markdown heading of the given level, capped at maxHeadingLevel, holding text.
func heading(level int, text string) string {
	return strings.Repeat("#", min(level, maxHeadingLevel)) + " " + text
}

// countedHeading is a board heading of the given level, its name followed by the number of entries the section lists.
func countedHeading(level int, name string, count int) string {
	return heading(level, fmt.Sprintf("%s (%d)", name, count))
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
	ordered      []TaskWithLayer
	bySlug       map[string]TaskWithLayer
	dependencies Dependencies
	primeOwner   string
	ownerOf      func(slug string) string
	out          Outputs
	now          time.Time
}

// newBoardView indexes ordered for rendering the board files with the given prime owner and outputs, every age as of now.
func newBoardView(ordered []TaskWithLayer, primeOwner string, out Outputs, now time.Time) boardView {
	bySlug := make(map[string]TaskWithLayer, len(ordered))
	entries := make([]Task, 0, len(ordered))
	for _, twl := range ordered {
		bySlug[twl.Slug] = twl
		entries = append(entries, twl.Task)
	}
	return boardView{
		ordered:      ordered,
		bySlug:       bySlug,
		dependencies: NewDependencies(entries),
		primeOwner:   primeOwner,
		ownerOf:      readmeOwners(ordered, primeOwner),
		out:          out,
		now:          now,
	}
}

// upLink is a markdown link to the board file that project's file links up to: its parent's project file, or README.md, titled Board, for a project whose parent is not on the board, the root and every project under no other included.
func (b boardView) upLink(project Task) string {
	parent, onBoard := b.bySlug[project.Project]
	if !onBoard {
		return "[" + rootTitle + "](" + b.out.Readme + ")"
	}
	return projectLink(parent.Task)
}

// projectLink is a markdown link to project's own board file, its text the project's handle and title.
func projectLink(project Task) string {
	file := rootProjectFile
	if project.Slug != "" {
		file = projectFileName(project.Slug)
	}
	return "[" + cellText(project.Handle+" "+project.Title) + "](" + file + ")"
}

// renderedLine is the line every board file states its render time on.
func (b boardView) renderedLine() string {
	return "Rendered " + b.now.UTC().Format(renderedTimeLayout) + "; every age below is as of then."
}

// fileHeader is the opening of project's board file: its handle and title, the render time, a link up to the file above it, its facts line, its brief, for a beam the word beam, and its body when it has one.
func (b boardView) fileHeader(project Task) []string {
	lines := []string{"# " + cellText(project.Handle+" "+project.Title), "", b.renderedLine(), "Under: " + b.upLink(project) + ".", "", b.projectFacts(project), ""}
	if project.Brief != "" {
		lines = append(lines, project.Brief, "")
	}
	if project.Beam {
		lines = append(lines, projectBeamText, "")
	}
	if strings.TrimSpace(project.Body) == "" {
		return lines
	}
	return append(append(lines, projectBodyLines(project.Body)...), "")
}

// bodyTopHeadingLevel is the level the shallowest heading of a project's body renders at in its board file, one below the file's own heading.
const bodyTopHeadingLevel = 2

// projectBodyLines is a project's body as lines of its board file, trailing newlines dropped.
// Its ATX headings shift down together until the shallowest sits at bodyTopHeadingLevel, capped at maxHeadingLevel, so the body never adds a heading beside the file's own;
// a body whose headings already sit that deep keeps them, and a line inside a fenced code block is never read as a heading.
func projectBodyLines(body string) []string {
	lines := strings.Split(strings.TrimRight(body, "\r\n"), "\n")
	shallowest := maxHeadingLevel
	forEachBodyHeading(lines, func(_, level int, _ string) {
		shallowest = min(shallowest, level)
	})
	shift := max(0, bodyTopHeadingLevel-shallowest)
	if shift == 0 {
		return lines
	}
	forEachBodyHeading(lines, func(index, level int, rest string) {
		lines[index] = strings.Repeat("#", min(level+shift, maxHeadingLevel)) + rest
	})
	return lines
}

// forEachBodyHeading calls visit with the index, level and text after the hashes of every ATX heading line of lines outside a fenced code block.
func forEachBodyHeading(lines []string, visit func(index, level int, rest string)) {
	openFence := ""
	for index, line := range lines {
		fence, info := codeFence(line)
		if openFence != "" {
			// A closing fence repeats the opening character at least as many times and carries no info string.
			if fence != "" && fence[0] == openFence[0] && len(fence) >= len(openFence) && strings.TrimSpace(info) == "" {
				openFence = ""
			}
			continue
		}
		if fence != "" {
			openFence = fence
			continue
		}
		if level, rest := atxHeading(line); level > 0 {
			visit(index, level, rest)
		}
	}
}

// markdownMaxIndent is the deepest indent, in spaces, at which a markdown heading or code fence still starts a line; a deeper one is an indented code block.
const markdownMaxIndent = 3

// codeFence is the run of three or more backticks or tildes that opens line as a code fence, and the text after it, or "" when line is no fence.
func codeFence(line string) (fence, info string) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > markdownMaxIndent || trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return "", ""
	}
	info = strings.TrimLeft(trimmed, trimmed[:1])
	fence = trimmed[:len(trimmed)-len(info)]
	if len(fence) < 3 || (fence[0] == '`' && strings.Contains(info, "`")) {
		return "", ""
	}
	return fence, info
}

// atxHeading is the level of line as an ATX heading and the text after its hashes, or level 0 when line is no heading.
func atxHeading(line string) (level int, rest string) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > markdownMaxIndent {
		return 0, ""
	}
	rest = strings.TrimLeft(trimmed, "#")
	level = len(trimmed) - len(rest)
	if level == 0 || level > maxHeadingLevel || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
		return 0, ""
	}
	return level, rest
}

// projectFacts is the line naming project's slug, linked to its design doc when the project has a body, its age as of the render time and its effective owner, the age left out when its created date is unknown.
// The root's line names only the prime owner, since the root has no board entry to give it a slug or an age.
func (b boardView) projectFacts(project Task) string {
	if project.Slug == "" {
		return "Owner " + primeOwnerText(b.primeOwner)
	}
	parts := []string{slugCell(project, b.out.DesignPrefix)}
	if age := FormatAge(project.Created, b.now); age != "" {
		parts = append(parts, "Age "+age)
	}
	return strings.Join(append(parts, "Owner "+primeOwnerText(b.ownerOf(project.Slug))), " · ")
}

// renderReadme builds README.md, the overview of every project: its header naming the render time and the prime owner, the intro, then a section at heading level two per open project under no other, in childProjects' order, and the root project's section last.
func (b boardView) renderReadme() string {
	lines := []string{"# " + rootTitle, "", b.renderedLine(), "Prime owner: " + primeOwnerText(b.primeOwner) + ".", ""}
	lines = append(lines, boardIntro...)
	for _, child := range b.childProjects(rootProject) {
		lines = append(lines, b.projectSection(child.Task, 2)...)
	}
	lines = append(lines, b.projectSection(rootProject, 2)...)
	return joinLines(lines)
}

// renderProjectFile builds project's board file, rootProject naming the root's: its header, which stands as the project's level-one heading, the intro, then the project's sub-projects and own entries.
func (b boardView) renderProjectFile(project Task) string {
	lines := append(b.fileHeader(project), boardIntro...)
	lines = append(lines, b.projectBody(project, 1)...)
	return joinLines(lines)
}

// projectSection renders project, rootProject naming the root, as one section at heading level level: its heading linking its handle and title to its file and counting its open members, its facts line, its brief, then its body.
func (b boardView) projectSection(project Task, level int) []string {
	members := 0
	for _, twl := range b.ordered {
		// The root's projects stand beside it, not under it, so they are not its members.
		member := twl.Project == project.Slug && (project.Slug != "" || twl.Kind != KindProject)
		if member && !isDone(twl.Task) {
			members++
		}
	}
	lines := []string{countedHeading(level, projectLink(project), members), "", b.projectFacts(project), ""}
	if project.Brief != "" {
		lines = append(lines, project.Brief, "")
	}
	return append(lines, b.projectBody(project, level)...)
}

// projectBody renders what sits under the heading of project, rootProject naming the root, at heading level level: a section per open project directly under it, in childProjects' order, each one level down and holding its own sub-projects in turn, then project's own Tasks, Notes and Done.
// With no open sub-project the own entries sit one level down;
// with any, they sit under an Own entries heading at the sub-projects' level, so a reader sees where the last sub-project ends.
// The root's body is its own entries alone, since its projects stand beside it in README.md.
func (b boardView) projectBody(project Task, level int) []string {
	if project.Slug == "" {
		return b.entrySections(project, level+1)
	}
	children := b.childProjects(project)
	if len(children) == 0 {
		return b.entrySections(project, level+1)
	}
	var lines []string
	for _, child := range children {
		lines = append(lines, b.projectSection(child.Task, level+1)...)
	}
	lines = append(lines, heading(level+1, ownEntriesSection.name), "", ownEntriesSection.meaning, "")
	return append(lines, b.entrySections(project, level+2)...)
}

// joinLines joins a board file's lines into its content, ending in exactly one newline.
func joinLines(lines []string) string {
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// table is one markdown table, a row per entry in the order given, followed by a blank line.
func (b boardView) table(entries []TaskWithLayer, table readmeTable) []string {
	lead := []string{columnHandle}
	if table.priority {
		lead = append(lead, columnPriority)
	}
	// The slug goes second among the middle columns, right after the entry.
	header := append(append(lead, slices.Insert(slices.Clone(table.columns), 1, columnSlug)...), columnAgeRevised, columnLabels)
	lines := []string{tableRow(header), tableRow(slices.Repeat([]string{"---"}, len(header)))}
	for _, twl := range entries {
		row := []string{cellText(twl.Handle)}
		if table.priority {
			row = append(row, priorityCell(twl.Priority))
		}
		row = append(row, slices.Insert(table.cells(twl.Task), 1, slugColumnCell(twl.Task, b.out.DesignPrefix))...)
		row = append(row, ageCell(twl.Task, b.now), labelsCell(twl.Task, b.out.Types))
		lines = append(lines, tableRow(row))
	}
	return append(lines, "")
}

// entrySections builds the Tasks, Notes and Done sections of project, rootProject naming the root, with level the heading level of their headings and their subsections one level down: Tasks split into Running, Ready, dependency layers, Independent, Discussed and Draft, Notes split by type label, then Done when any of its entries is done.
// Every section holds only the entries whose project is project, while layers, After cells and owners read the whole board.
func (b boardView) entrySections(project Task, level int) []string {
	sublevel := level + 1
	var own []TaskWithLayer
	for _, twl := range b.ordered {
		if twl.Project == project.Slug {
			own = append(own, twl)
		}
	}
	types := b.out.Types
	waitTable := readmeTable{columns: []string{columnTask, columnRecipe, columnAfter}, cells: func(t Task) []string {
		return []string{entryCell(t), recipeCell(t, b.out), b.afterCell(t)}
	}}

	var tasks []TaskWithLayer
	for _, twl := range own {
		if !isDone(twl.Task) && twl.Kind == KindTask {
			tasks = append(tasks, twl)
		}
	}
	lines := []string{countedHeading(level, tasksSection.name, len(tasks)), "", tasksSection.meaning, ""}
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
		lines = append(lines, countedHeading(sublevel, runningSection.name, len(running)), "", runningSection.meaning, "")
		lines = append(lines, b.table(running, readmeTable{columns: []string{columnTask, columnRecipe, columnAt, columnOwner}, cells: func(t Task) []string {
			return []string{entryCell(t), recipeCell(t, b.out), atCell(*t.Status), cellText(b.ownerOf(t.Slug))}
		}})...)
	}
	ready := byLayer(readyLayer)
	lines = append(lines, countedHeading(sublevel, readySection.name, len(ready)), "", readySection.meaning, "")
	if len(ready) > 0 {
		lines = append(lines, b.table(ready, waitTable)...)
	} else {
		lines = append(lines, emptySectionLine, "")
	}
	// RenderOrder sorts tasks by layer, so each dependency layer, Independent, Discussed and Draft is a contiguous run.
	for start := 0; start < len(tasks); {
		end := start
		for end < len(tasks) && tasks[end].Layer == tasks[start].Layer {
			end++
		}
		if layer := tasks[start].Layer; layer != runningLayer && layer != readyLayer {
			section := layerSection(layer)
			lines = append(lines, countedHeading(sublevel, section.name, end-start), "", section.meaning, "")
			lines = append(lines, b.table(tasks[start:end], waitTable)...)
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
	lines = append(lines, countedHeading(level, notesSection.name, noteCount), "", notesSection.meaning, "")
	noteTable := readmeTable{columns: []string{columnNote}, cells: func(t Task) []string { return []string{entryCell(t)} }, priority: true}
	for _, typeLabel := range notesTypeOrder(types) {
		entries := byType[typeLabel]
		if len(entries) == 0 {
			continue
		}
		sectionName := otherNotesHeading
		if typeLabel != "" {
			sectionName = typeHeading(typeLabel)
		}
		lines = append(lines, countedHeading(sublevel, sectionName, len(entries)), "")
		lines = append(lines, b.table(entries, noteTable)...)
	}

	var done []TaskWithLayer
	for _, twl := range own {
		if isDone(twl.Task) {
			done = append(done, twl)
		}
	}
	if len(done) > 0 {
		lines = append(lines, countedHeading(level, doneSection.name, len(done)), "", doneSection.meaning, "")
		lines = append(lines, b.table(done, readmeTable{columns: []string{columnEntry}, cells: func(t Task) []string {
			if t.Kind == KindProject {
				return []string{"**[" + cellText(t.Title) + "](" + projectFileName(t.Slug) + ")**"}
			}
			return []string{entryCell(t)}
		}})...)
	}
	return lines
}

// childProjects returns the open projects directly under project, in project's order, then by handle.
// rootProject names the root, which has no order, so its children run by handle;
// an open project under a done project counts as directly under the root, since a done project has no README section to hold it.
func (b boardView) childProjects(project Task) []TaskWithLayer {
	var children []TaskWithLayer
	for _, twl := range b.ordered {
		parent, parentOnBoard := b.bySlug[twl.Project]
		under := twl.Project == project.Slug || (project.Slug == "" && parentOnBoard && isDone(parent.Task))
		if under && !isDone(twl.Task) && twl.Kind == KindProject {
			children = append(children, twl)
		}
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
	return children
}

// afterCell is the handles of the entries t depends on that are not done, a project judged by its landing members too, one per line in depends_on order, a dependency the board holds no handle for shown by its slug.
func (b boardView) afterCell(t Task) string {
	var after []string
	for _, dep := range t.DependsOn {
		entry, found := b.bySlug[dep]
		switch {
		case found && b.dependencies.met(dep):
			// A met dependency no longer holds t back.
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
	case discussedLayer:
		return readmeSection{"Discussed", "Discussion approved and its decision record stored; nothing starts it until its status is cleared."}
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

// sketchLinkText is the text of a discussed entry's link to its decision record.
const sketchLinkText = "Sketch"

// slugColumnCell is a table's Slug cell: slugCell, with a Sketch link to the decision record stacked under it for a discussed entry, so the mark adds no column.
func slugColumnCell(t Task, designPrefix string) string {
	cell := slugCell(t, designPrefix)
	if !t.Discussed {
		return cell
	}
	return cell + cellBreak + "[" + sketchLinkText + "](" + DecisionFileName(t.Slug) + ")"
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

// entryCell is the bold title, led by a `draft` or `discussed` code span for a task carrying that status, with the brief as one bullet under it when there is one, then one code-span bullet per run setting.
func entryCell(t Task) string {
	cell := "**" + cellText(t.Title) + "**"
	switch {
	case isDraft(t):
		cell = codeSpan(StatusDraft) + " " + cell
	case isDiscussedStatus(t):
		cell = codeSpan(StatusDiscussed) + " " + cell
	}
	if t.Brief != "" {
		cell += cellBullet + cellText(t.Brief)
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

// recipeCell is the recipe the task runs, the default recipe of out when its recipe field is empty, followed by the run profile it names, if any.
// A task naming no profile takes its recipe's default profile, so the cell shows the recipe alone.
func recipeCell(t Task, out Outputs) string {
	recipe := cellText(out.recipeOf(t))
	if t.Profile == "" {
		return recipe
	}
	return recipe + recipeProfileSeparator + cellText(t.Profile)
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
