// render.go turns the entry list into the board's output files:
// README.md, a board file per project and the root, a design file per entry with a body, board-data.js and board.html.
// RenderToDisk writes them and removes the files the previous render produced and this one does not.

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

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/fsx"
)

// renderManifestFile is the name of the sidecar that records the filenames the last render produced.
// `ensureBoardIgnores` gitignores it, so it adds no commit churn, and it is never a member of the rendered file set.
const renderManifestFile = fabriclayout.BoardRenderManifestFile

// RenderToDisk renders the board as of now and writes its output files under boardPath.
// It rewrites an existing output file in place, keeping its inode.
// VS Code's file watcher reports a file replaced by rename as created, not changed.
// VS Code's markdown preview refreshes only on a change.
// It removes the files the previous render recorded in the manifest that this render does not produce.
// An entry flagged discussed whose decision record file is missing from boardPath renders as undiscussed,
// so no board file links a missing record.
// The board files carry the beam modes out's reader reads for the hub of boardPath, and none when it has no reader.
func RenderToDisk(boardPath string, tasks []Task, primeOwner string, out Outputs, now time.Time) error {
	modes := readBeamModes(boardPath, tasks, out)
	files, err := renderFiles(tasks, withMissingRecordsUndiscussed(boardPath, tasks), primeOwner, out, now, modes)
	if err != nil {
		return err
	}
	for relPath, content := range files {
		if err := fsx.PathGuard(relPath); err != nil {
			return fmt.Errorf("write %s: %w", relPath, err)
		}
		fullPath := filepath.Join(boardPath, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("write %s: %w", relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", relPath, err)
		}
	}

	previous := readRenderManifest(boardPath)
	for _, name := range previous {
		if _, kept := files[name]; !kept {
			// Best-effort removal: a stale file left behind is harmless and is cleaned up on the next render.
			os.Remove(filepath.Join(boardPath, name))
		}
	}

	writeRenderManifest(boardPath, files)
	return nil
}

// withMissingRecordsUndiscussed returns a copy of tasks.
// Each discussed entry whose decision record file is missing from boardPath has its discussed flag cleared in the copy.
func withMissingRecordsUndiscussed(boardPath string, tasks []Task) []Task {
	rendered := slices.Clone(tasks)
	for i, task := range rendered {
		if !task.Discussed {
			continue
		}
		if _, err := os.Stat(filepath.Join(boardPath, DecisionFileName(task.Slug))); err != nil {
			rendered[i].Discussed = false
		}
	}
	return rendered
}

// readRenderManifest returns the filenames the previous render recorded.
// It returns nil when the manifest is missing or unreadable.
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

// Render produces the board output files from the entries, measuring every age as of now.
// primeOwner is the machine that owns the hub's prime, or "" when none is recorded.
// Every project, open or done, gets a file named by projectFileName, and the root project gets rootProjectFile.
// Every entry flagged discussed is linked to its decision record, which Render takes to exist.
// BoardDataFile holds the board's view projection beside the README, and BoardPageFile the viewer page.
// The files carry no beam mode line, since Render reads no hub.
func Render(tasks []Task, primeOwner string, out Outputs, now time.Time) (map[string]string, error) {
	return renderFiles(tasks, tasks, primeOwner, out, now, nil)
}

// renderFiles renders every board file from rendered, and the view projection's revisions from stored.
// The two differ only where RenderToDisk clears the discussed flag of an entry whose decision record is missing,
// so an entry's revision covers its stored copy while its files follow the copy rendered.
// modes are the beam modes the files show, nil for none.
func renderFiles(stored, rendered []Task, primeOwner string, out Outputs, now time.Time, modes *BeamModes) (map[string]string, error) {
	ordered, err := RenderOrder(rendered)
	if err != nil {
		return nil, err
	}

	board := newBoardView(ordered, primeOwner, out, now)
	board.modes = modes
	result := map[string]string{
		out.Readme:      board.renderReadme(),
		rootProjectFile: board.renderProjectFile(rootProject),
	}
	for _, twl := range ordered {
		if twl.Kind == KindProject {
			result[projectFileName(twl.Slug)] = board.renderProjectFile(twl.Task)
		}
	}

	for name, content := range renderDesigns(rendered, out.DesignPrefix) {
		result[name] = content
	}

	view, err := buildView(stored, rendered, primeOwner, out, now)
	if err != nil {
		return nil, err
	}
	script, err := boardDataScript(view)
	if err != nil {
		return nil, err
	}
	result[BoardDataFile] = script
	result[BoardPageFile] = string(ViewerPage())
	return result, nil
}

// boardIntro is the paragraph every board file holds under its header.
var boardIntro = []string{
	"Tasks are grouped by dependency layer and notes by type.",
	"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.",
	"",
}

// The fixed section headings of the board files.
const (
	ownEntriesHeading = "Own entries"
	runningHeading    = "Running"
	readyHeading      = "Ready"
	doneHeading       = "Done"
)

// otherNotesHeading is the section heading for a note whose type label is no longer configured.
const otherNotesHeading = "Other"

// researchType is the type label of a measurement, bench or study.
// Its section follows the leading note types.
const researchType = "research"

// leadingNoteTypes are the type labels whose sections lead the other note sections, in this order:
// Sketches wait for the operator's triage, and research follows them.
var leadingNoteTypes = []string{SketchType, researchType}

// typeHeadings gives the section heading of each note type label that does not follow the default.
// The default is the label capitalised with an s appended.
var typeHeadings = map[string]string{researchType: "Research", SketchType: "Sketches"}

// projectFilePrefix starts the file name of a project's own board file, which its slug and .md complete.
// The replayer in replay.go recognises the same prefix.
const projectFilePrefix = "project-"

// rootTitle is the H1 of README.md, the overview of every project, and the text of a link up to it.
const rootTitle = "Board"

// rootProject is the root project as the board files render it, with handle P0 and the entries under no other project.
// No board project takes P0, since their handles start at 1.
// Its empty slug names the root wherever a project is looked up, and it has no board entry, so no design file.
var rootProject = Task{Handle: projectHandleLetter + "0", Title: "Root: entries under no project"}

// RootProjectBrief returns the root project as one listing row.
// The row has handle P0, slug RootSubject and the title the board files give the root.
// The root is no board entry, so the row carries no labels, dates or owner.
func RootProjectBrief() BriefTask {
	return BriefTask{Handle: rootProject.Handle, Slug: RootSubject, Title: rootProject.Title, Kind: KindProject, Layer: projectLayer}
}

// rootProjectFile is the name of the root project's own board file.
var rootProjectFile = projectFileName(RootSubject)

// The column headings of the board's tables, one constant per heading.
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

// beamMarker ends a beam project's heading and its Done row, so a reader tells a beam from a plain project.
const beamMarker = " · BEAM"

// beamSuffix is the marker that ends a beam's heading and Done row, and empty for any other entry.
func beamSuffix(t Task) string {
	if t.Beam {
		return beamMarker
	}
	return ""
}

// cellBreak stacks two lines inside one table cell, where a newline would end the row.
const cellBreak = "<br>"

// renderedTimeLayout is the UTC date and time the README states it was rendered at.
const renderedTimeLayout = "2006-01-02 15:04 UTC"

// recipeProfileSeparator stands between the recipe and the run profile in one Recipe cell.
const recipeProfileSeparator = " · "

// runningState is the run state the At cell leaves out, since the Running section already says it.
const runningState = "running"

// cellBullet starts one detail line under the title inside one table cell.
// A markdown list does not render in a table cell.
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

// typeHeading is the section heading of a note type label.
// It is the label's entry in `typeHeadings` when it has one.
// Otherwise it is the label capitalised with an s appended, and an empty label gives "s".
func typeHeading(typeLabel string) string {
	if heading, irregular := typeHeadings[typeLabel]; irregular {
		return heading
	}
	if typeLabel == "" {
		return "s"
	}
	return strings.ToUpper(typeLabel[:1]) + typeLabel[1:] + "s"
}

// notesTypeOrder is the order of the note sections.
// The leading note types come first, in their configured order.
// The other types follow in configured order, and "" for Other comes last.
func notesTypeOrder(types []string) []string {
	var order []string
	for _, lead := range leadingNoteTypes {
		if slices.Contains(types, lead) {
			order = append(order, lead)
		}
	}
	for _, typeLabel := range types {
		if !slices.Contains(leadingNoteTypes, typeLabel) {
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

// readmeTable is one README subsection's table.
// It has the middle column headers and the cells under them for each entry, with the entry column first.
// When priority is set, the table adds a priority column after the handle.
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

// boardView is the whole board as every board file reads it.
// It holds the entries in render order and the facts each file looks up across projects.
type boardView struct {
	ordered      []TaskWithLayer
	bySlug       map[string]TaskWithLayer
	dependencies Dependencies
	primeOwner   string
	ownerOf      func(slug string) string
	out          Outputs
	now          time.Time
	// modes are the beam modes the board files show, nil for none.
	modes *BeamModes
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

// upLink is a markdown link to the board file that project's file links up to.
// It is the parent's project file, or the README titled `rootTitle` when the parent is not on the board.
func (b boardView) upLink(project Task) string {
	parent, onBoard := b.bySlug[project.Project]
	if !onBoard {
		return "[" + rootTitle + "](" + b.out.Readme + ")"
	}
	return projectLink(parent.Task)
}

// projectLink is a markdown link to project's own board file, its text the project's handle and title.
func projectLink(project Task) string {
	return "[" + cellText(project.Handle+" "+project.Title) + "](" + projectFile(project) + ")"
}

// projectFile is the name of project's own board file, the root's included.
func projectFile(project Task) string {
	if project.Slug == "" {
		return rootProjectFile
	}
	return projectFileName(project.Slug)
}

// renderedLine is the line every board file states its render time on.
func (b boardView) renderedLine() string {
	return "Rendered " + b.now.UTC().Format(renderedTimeLayout) + "; every age below is as of then."
}

// fileHeader is the opening of project's board file.
// It holds the heading, the render time and a link up to the file above.
// The facts line, the mode line, the brief and the body follow.
func (b boardView) fileHeader(project Task) []string {
	title := "# " + cellText(project.Handle+" "+project.Title) + beamSuffix(project)
	lines := []string{title, "", b.renderedLine(), "Under: " + b.upLink(project) + ".", "", b.projectFacts(project), ""}
	lines = append(lines, b.modeLines(project)...)
	if project.Brief != "" {
		lines = append(lines, project.Brief, "")
	}
	if strings.TrimSpace(project.Body) == "" {
		return lines
	}
	return append(append(lines, projectBodyLines(project.Body)...), "")
}

// bodyTopHeadingLevel is the level the shallowest heading of a project's body renders at in its board file.
// It sits one below the file's own heading.
const bodyTopHeadingLevel = 2

// projectBodyLines is a project's body as lines of its board file, trailing newlines dropped.
// Its ATX headings shift down together until the shallowest sits at `bodyTopHeadingLevel`.
// The shift stops at `maxHeadingLevel`, so the body never adds a heading beside the file's own.
// A body whose headings already sit that deep keeps them.
// A line inside a fenced code block is never read as a heading.
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

// forEachBodyHeading calls visit for every ATX heading line of lines outside a fenced code block.
// It passes the line's index, its level and the text after its hashes.
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

// markdownMaxIndent is the deepest indent, in spaces, at which a heading or code fence still starts a line.
// A deeper line is an indented code block.
const markdownMaxIndent = 3

// codeFence reports the run of three or more backticks or tildes that opens line as a code fence.
// It also returns the text after the run, and both results are empty when line is no fence.
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

// projectFacts is the line naming project's slug, its age as of the render time and its effective owner.
// The slug links to its design doc when the project has a body.
// The age is left out when the created date is unknown.
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

// renderReadme builds README.md, the overview of every project.
// Its header names the render time, the prime owner and, with beam modes, the hub-wide defaults; then comes the intro.
// Each open project directly under the root gets a section, and the root's own section comes last.
func (b boardView) renderReadme() string {
	lines := []string{"# " + rootTitle, "", b.renderedLine(), "Prime owner: " + primeOwnerText(b.primeOwner) + ".", ""}
	lines = append(lines, b.modeHeader()...)
	lines = append(lines, boardIntro...)
	for _, child := range b.childProjects(rootProject) {
		lines = append(lines, b.projectSection(child.Task, readmeSectionLevel)...)
	}
	lines = append(lines, b.projectSection(rootProject, readmeSectionLevel)...)
	return joinLines(lines)
}

// readmeSectionLevel is the heading level of each project section of README.md.
const readmeSectionLevel = 2

// renderProjectFile builds the board file of project.
// Its header stands as the project's level-one heading; the intro follows, then the project's sub-projects and own entries.
func (b boardView) renderProjectFile(project Task) string {
	lines := append(b.fileHeader(project), boardIntro...)
	lines = append(lines, b.projectBody(project, 1)...)
	return joinLines(lines)
}

// projectSection renders project as one section at the heading level given.
// Its heading links the project's handle and title to its file and counts its open members.
func (b boardView) projectSection(project Task, level int) []string {
	heading := countedHeading(level, projectLink(project), b.openMembers(project)) + beamSuffix(project)
	return append([]string{heading, ""}, b.projectSectionContent(project, level)...)
}

// openMembers counts the entries directly under project that are not done.
func (b boardView) openMembers(project Task) int {
	members := 0
	for _, twl := range b.ordered {
		// The root's projects stand beside it, not under it, so they are not its members.
		member := twl.Project == project.Slug && (project.Slug != "" || twl.Kind != KindProject)
		if member && !isDone(twl.Task) {
			members++
		}
	}
	return members
}

// projectSectionContent is what a section of project at heading level level holds under its heading:
// its facts line, its mode line for the root or a beam, its brief when it has one, and its body.
func (b boardView) projectSectionContent(project Task, level int) []string {
	lines := []string{b.projectFacts(project), ""}
	lines = append(lines, b.modeLines(project)...)
	if project.Brief != "" {
		lines = append(lines, project.Brief, "")
	}
	return append(lines, b.projectBody(project, level)...)
}

// projectBody renders what sits under the heading of project at heading level level.
// Each open sub-project directly under project gets a section one level down;
// with any, project's own entries follow under an Own entries heading, so a reader sees where the last sub-project ends.
// The root's body is its own entries alone, since its projects stand beside it in README.md.
func (b boardView) projectBody(project Task, level int) []string {
	children := b.childProjects(project)
	if project.Slug == "" || len(children) == 0 {
		return b.entrySections(project, level+1)
	}
	var lines []string
	for _, child := range children {
		lines = append(lines, b.projectSection(child.Task, level+1)...)
	}
	if own := b.entrySections(project, level+2); len(own) > 0 {
		lines = append(lines, heading(level+1, ownEntriesHeading), "")
		lines = append(lines, own...)
	}
	return lines
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

// entrySections renders the entries whose project is project as sections at heading level level,
// leaving out a section with no entry.
// Layers, After cells and owners read the whole board.
func (b boardView) entrySections(project Task, level int) []string {
	var own []TaskWithLayer
	for _, twl := range b.ordered {
		if twl.Project == project.Slug {
			own = append(own, twl)
		}
	}
	var lines []string
	section := func(name string, entries []TaskWithLayer, table readmeTable) {
		if len(entries) == 0 {
			return
		}
		lines = append(lines, countedHeading(level, name, len(entries)), "")
		lines = append(lines, b.table(entries, table)...)
	}

	var tasks []TaskWithLayer
	for _, twl := range own {
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
	waitTable := readmeTable{columns: []string{columnTask, columnRecipe, columnAfter}, cells: func(t Task) []string {
		return []string{entryCell(t), recipeCell(t, b.out), b.afterCell(t)}
	}}
	section(runningHeading, byLayer(runningLayer), readmeTable{columns: []string{columnTask, columnRecipe, columnAt, columnOwner}, cells: func(t Task) []string {
		return []string{entryCell(t), recipeCell(t, b.out), atCell(*t.Status), cellText(b.ownerOf(t.Slug))}
	}})
	section(readyHeading, byLayer(readyLayer), waitTable)
	// Tasks are ordered by layer, so each dependency layer, Independent and Draft, is one contiguous run.
	for start := 0; start < len(tasks); {
		end := start
		for end < len(tasks) && tasks[end].Layer == tasks[start].Layer {
			end++
		}
		if layer := tasks[start].Layer; layer != runningLayer && layer != readyLayer {
			section(layerHeading(layer), tasks[start:end], waitTable)
		}
		start = end
	}

	types := b.out.Types
	byType := make(map[string][]TaskWithLayer)
	for _, twl := range own {
		if !isDone(twl.Task) && twl.Kind == KindNote {
			typeLabel := noteType(twl.Task, types)
			byType[typeLabel] = append(byType[typeLabel], twl)
		}
	}
	for _, typeLabel := range notesTypeOrder(types) {
		section(noteSectionName(typeLabel), byType[typeLabel], b.noteTable(byType[typeLabel]))
	}

	var done []TaskWithLayer
	for _, twl := range own {
		if isDone(twl.Task) {
			done = append(done, twl)
		}
	}
	section(doneHeading, done, readmeTable{columns: []string{columnEntry}, cells: func(t Task) []string {
		if t.Kind == KindProject {
			return []string{"**[" + cellText(t.Title) + "](" + projectFileName(t.Slug) + ")**" + beamSuffix(t)}
		}
		return []string{entryCell(t)}
	}})
	return lines
}

// noteTable returns the table of one note section holding notes.
// It carries the After column only when one of the notes waits on a dependency that is not met.
func (b boardView) noteTable(notes []TaskWithLayer) readmeTable {
	waits := slices.ContainsFunc(notes, func(twl TaskWithLayer) bool { return len(b.unmetDependencies(twl.Task)) > 0 })
	if !waits {
		return readmeTable{columns: []string{columnNote}, cells: func(t Task) []string { return []string{entryCell(t)} }, priority: true}
	}
	return readmeTable{columns: []string{columnNote, columnAfter}, cells: func(t Task) []string {
		return []string{entryCell(t), b.afterCell(t)}
	}, priority: true}
}

// childProjects returns the open projects directly under project, in project's order, then by handle.
// The root has no order, so its children run by handle.
// An open project under a done project counts as directly under the root.
// A done project has no README section to hold it.
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

// unmetDependencies returns the slugs of t's dependencies that are not met, in depends_on order.
// A project dependency is judged by its landing members too.
// A dependency the board holds no entry for counts as unmet.
func (b boardView) unmetDependencies(t Task) []string {
	var unmet []string
	for _, dep := range t.DependsOn {
		if _, found := b.bySlug[dep]; found && b.dependencies.met(dep) {
			continue
		}
		unmet = append(unmet, dep)
	}
	return unmet
}

// afterCell returns the After cell of t.
// The cell holds the handles of its dependencies that are not met, one per line in depends_on order.
// A dependency the board holds no handle for is shown by its slug.
func (b boardView) afterCell(t Task) string {
	var after []string
	for _, dep := range b.unmetDependencies(t) {
		if entry, found := b.bySlug[dep]; found && entry.Handle != "" {
			after = append(after, cellText(entry.Handle))
		} else {
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

// layerHeading is the section heading of a computed dependency layer.
func layerHeading(layer string) string {
	switch layer {
	case isolatedLayer:
		return "Independent"
	case draftLayer:
		return "Draft"
	}
	return "Layer " + layer
}

// tableRow joins cells into one markdown table row.
func tableRow(cells []string) string {
	return "| " + strings.Join(cells, " | ") + " |"
}

// cellText makes s safe inside one table cell: a pipe is escaped,
// and a line break becomes a space so the row stays on one line.
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

// recordLinePrefix opens the design doc line that links a discussed entry's decision record.
const recordLinePrefix = "Decision record: "

// sketchLink is a markdown link to the decision record of the entry slug.
func sketchLink(slug string) string {
	return "[" + sketchLinkText + "](" + DecisionFileName(slug) + ")"
}

// slugColumnCell is a table's Slug cell: slugCell, with a Sketch link to the decision record for a discussed entry.
// The link stacks under the slug, so the mark adds no column.
func slugColumnCell(t Task, designPrefix string) string {
	cell := slugCell(t, designPrefix)
	if !t.Discussed {
		return cell
	}
	return cell + cellBreak + sketchLink(t.Slug)
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

// entryCell is the bold title, led by a `draft` code span for a draft task.
// The brief follows as one bullet when there is one.
// Then come one code-span bullet per run field the entry sets and one per run setting.
func entryCell(t Task) string {
	cell := "**" + cellText(t.Title) + "**"
	if isDraft(t) {
		cell = codeSpan(StatusDraft) + " " + cell
	}
	if t.Brief != "" {
		cell += cellBullet + cellText(t.Brief)
	}
	for _, line := range append(runFieldLines(t), settingLines(t.RunSettings)...) {
		cell += cellBullet + codeSpan(cellText(line))
	}
	return cell
}

// runFieldLines is one `<key>: <value>` line per field of t that changes how its run goes, in a fixed order.
// It leaves out the run settings and the Recipe cell.
// on_land names the action that runs once the task lands.
// A field left empty gives no line.
func runFieldLines(t Task) []string {
	var lines []string
	if t.OnLand != "" {
		lines = append(lines, onLandKey+": "+t.OnLand)
	}
	return lines
}

// settingLines is one `<module>: <key path> <value>` line per leaf of a task's run settings.
// Nested keys join with dots, and the lines come back sorted.
func settingLines(settings map[string]any) []string {
	var lines []string
	for module, value := range settings {
		lines = appendSettingLeaves(lines, module, "", value)
	}
	slices.Sort(lines)
	return lines
}

// appendSettingLeaves appends one setting line per leaf under value.
// path is the dotted key path from the module down to value.
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

// recipeCell is the recipe the task runs, followed by the run profile it names, if any.
// An empty recipe field falls back to the default recipe of out.
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
// The doc is a header (title, metadata line, decision record link, dependencies) followed by the body byte-for-byte.
// The decision record link stands only in a discussed entry's header.
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
		if e.Discussed {
			header = append(header, recordLinePrefix+sketchLink(e.Slug), "")
		}
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
