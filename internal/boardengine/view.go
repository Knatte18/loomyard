// view.go builds the board's view projection, the data behind board-data.js and the viewer server.
// BoardView is the projection.
// buildView computes it from the same boardView the README renders from.
// EntryRevision and BoardRevision are the revisions a page's write is checked against.

package boardengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// BoardDataFile is the name of the render output that holds the view projection as a script.
const BoardDataFile = "board-data.js"

// boardDataPrefix and boardDataSuffix wrap the projection's JSON into the script a page loads.
const (
	boardDataPrefix = "window.BOARD = "
	boardDataSuffix = ";\n"
)

// BoardView is the projection of the whole board a viewer reads.
// Every placement in it, a section, a parent project, an owner or an unmet dependency, is the README's own.
type BoardView struct {
	// Rendered is the render time in RFC 3339 UTC.
	Rendered   string `json:"rendered"`
	PrimeOwner string `json:"prime_owner"`
	// Types and Areas are the configured type and area labels in board.yaml order.
	Types []string `json:"types"`
	Areas []string `json:"areas"`
	// Sections are the README section names the entries occupy, in README order.
	Sections []string `json:"sections"`
	// Projects are the root and every open project, each after the project it stands under, then every done project.
	// A done project stays listed until it is pruned, so its members stand under it and not under the root.
	Projects []ViewProject `json:"projects"`
	// Entries are every task and note, done ones included, in render order.
	Entries []ViewEntry `json:"entries"`
}

// ViewProject is one project of a BoardView.
type ViewProject struct {
	Handle string   `json:"handle"`
	Slug   string   `json:"slug"`
	Title  string   `json:"title"`
	Brief  string   `json:"brief"`
	Labels []string `json:"labels"`
	// Parent is the handle of the project this one stands under in the README, empty for the root.
	Parent string `json:"parent"`
	// DesignFile is the name of the project's design file, empty when it has none.
	DesignFile string `json:"design_file"`
	// Done reports whether the project is done, so a page can keep it out of the choices of a project to write.
	Done bool `json:"done"`
}

// ViewEntry is one task or note of a BoardView.
type ViewEntry struct {
	Handle string `json:"handle"`
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Kind   string `json:"kind"`
	// Types and Areas split the entry's labels by the configured type labels.
	Types []string `json:"types"`
	Areas []string `json:"areas"`
	// Status is empty for an entry with none.
	Status string `json:"status"`
	// Project is the handle of the entry's project, the root's for none.
	Project string `json:"project"`
	// Section is the README section the entry stands under.
	Section string `json:"section"`
	// WaitsOn lists the handles of the entry's unmet dependencies, the slug for one the board holds no handle for.
	WaitsOn []string `json:"waits_on"`
	Brief   string   `json:"brief"`
	Body    string   `json:"body"`
	// Owner is the effective owner the README shows.
	Owner string `json:"owner"`
	// Recipe is a task's recipe with the default applied, empty for a note.
	Recipe  string `json:"recipe"`
	Created string `json:"created"`
	Revised string `json:"revised"`
	// DesignFile and DecisionFile are the file names that exist for the entry, empty when it has none.
	DesignFile   string `json:"design_file"`
	DecisionFile string `json:"decision_file"`
	// Revision is EntryRevision of the stored entry.
	Revision string `json:"revision"`
}

// EntryRevision returns the revision of t: the lowercase hex SHA-256 of its JSON encoding.
// The encoding holds every stored field, status and the unknown keys included,
// so any change to the stored entry changes the revision.
func EntryRevision(t Task) string {
	// A Task always encodes, so the error is unreachable.
	data, _ := json.Marshal(t)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// BoardRevision returns the lowercase hex SHA-256 of the board.json bytes under boardPath.
// An absent file gives "" and no error.
func BoardRevision(boardPath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(boardPath, boardFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read the board revision: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// buildView projects the board for a viewer.
// Every entry's revision is taken over its stored copy in stored, while its decision file follows the copy in rendered.
func buildView(stored, rendered []Task, primeOwner string, out Outputs, now time.Time) (BoardView, error) {
	ordered, err := RenderOrder(rendered)
	if err != nil {
		return BoardView{}, err
	}
	board := newBoardView(ordered, primeOwner, out, now)
	storedBySlug := make(map[string]Task, len(stored))
	for _, task := range stored {
		storedBySlug[task.Slug] = task
	}

	entries := []ViewEntry{}
	for _, twl := range ordered {
		if twl.Kind == KindProject {
			continue
		}
		entries = append(entries, board.viewEntry(twl, storedBySlug[twl.Slug]))
	}
	return BoardView{
		Rendered:   now.UTC().Format(time.RFC3339),
		PrimeOwner: primeOwner,
		Types:      nonNil(out.Types),
		Areas:      nonNil(out.Labels),
		Sections:   board.sectionsOccupiedBy(entries),
		Projects:   board.viewProjects(),
		Entries:    entries,
	}, nil
}

// boardDataScript returns the script a page loads to read view: an assignment of its JSON to window.BOARD.
// The JSON keeps Go's HTML escaping, so a body holding a closing script tag cannot end the script.
func boardDataScript(view BoardView) (string, error) {
	data, err := json.Marshal(view)
	if err != nil {
		return "", fmt.Errorf("encode the board view: %w", err)
	}
	return boardDataPrefix + string(data) + boardDataSuffix, nil
}

// nonNil returns values, or an empty slice for nil, so the projection encodes a list as [] and never null.
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// viewProjects lists the root, then every open project in README order, then every done project in README order.
// Each carries the handle it stands under: an open one its README section's parent,
// and a done one the project whose Done section lists it.
func (b boardView) viewProjects() []ViewProject {
	projects := []ViewProject{{Handle: rootProject.Handle, Slug: RootSubject, Title: rootProject.Title, Labels: []string{}}}
	add := func(project Task, parentHandle string, done bool) {
		projects = append(projects, ViewProject{
			Handle:     project.Handle,
			Slug:       project.Slug,
			Title:      project.Title,
			Brief:      project.Brief,
			Labels:     nonNil(project.Labels),
			Parent:     parentHandle,
			DesignFile: b.designFile(project),
			Done:       done,
		})
	}
	var walk func(parent Task)
	walk = func(parent Task) {
		for _, child := range b.childProjects(parent) {
			add(child.Task, parent.Handle, false)
			walk(child.Task)
		}
	}
	walk(rootProject)
	for _, twl := range b.ordered {
		if twl.Kind == KindProject && isDone(twl.Task) {
			add(twl.Task, b.projectHandle(twl.Task), true)
		}
	}
	return projects
}

// viewEntry projects one task or note.
// storedEntry is the entry as stored, which its revision covers.
func (b boardView) viewEntry(twl TaskWithLayer, storedEntry Task) ViewEntry {
	entry := ViewEntry{
		Handle:     twl.Handle,
		Slug:       twl.Slug,
		Title:      twl.Title,
		Kind:       twl.Kind,
		Types:      []string{},
		Areas:      []string{},
		Project:    b.projectHandle(twl.Task),
		Section:    b.sectionOf(twl),
		WaitsOn:    []string{},
		Brief:      twl.Brief,
		Body:       twl.Body,
		Owner:      b.ownerOf(twl.Slug),
		Created:    timeText(twl.Created),
		Revised:    timeText(twl.Revised),
		DesignFile: b.designFile(twl.Task),
		Revision:   EntryRevision(storedEntry),
	}
	for _, label := range twl.Labels {
		if slices.Contains(b.out.Types, label) {
			entry.Types = append(entry.Types, label)
		} else {
			entry.Areas = append(entry.Areas, label)
		}
	}
	if twl.Status != nil {
		entry.Status = *twl.Status
	}
	if twl.Kind == KindTask {
		entry.Recipe = b.out.recipeOf(twl.Task)
	}
	if twl.Discussed {
		entry.DecisionFile = DecisionFileName(twl.Slug)
	}
	for _, slug := range b.unmetDependencies(twl.Task) {
		if dependency, found := b.bySlug[slug]; found && dependency.Handle != "" {
			entry.WaitsOn = append(entry.WaitsOn, dependency.Handle)
		} else {
			entry.WaitsOn = append(entry.WaitsOn, slug)
		}
	}
	return entry
}

// projectHandle is the handle of the project t belongs to, the root's for none or for one the board does not hold.
func (b boardView) projectHandle(t Task) string {
	if project, found := b.bySlug[t.Project]; found && project.Handle != "" {
		return project.Handle
	}
	return rootProject.Handle
}

// designFile is the name of t's design file, empty when t has no body and so no file.
func (b boardView) designFile(t Task) string {
	if t.Body == "" {
		return ""
	}
	return b.out.DesignPrefix + t.Slug + designFileSuffix
}

// timeText is t in RFC 3339 UTC, empty when t is unknown.
func timeText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// sectionOf is the name of the README section twl stands under.
func (b boardView) sectionOf(twl TaskWithLayer) string {
	switch {
	case isDone(twl.Task):
		return doneHeading
	case twl.Kind == KindNote:
		return noteSectionName(noteType(twl.Task, b.out.Types))
	case twl.Layer == runningLayer:
		return runningHeading
	case twl.Layer == readyLayer:
		return readyHeading
	}
	return layerHeading(twl.Layer)
}

// noteSectionName is the section heading of a note type label, Other for none.
func noteSectionName(typeLabel string) string {
	if typeLabel == "" {
		return otherNotesHeading
	}
	return typeHeading(typeLabel)
}

// sectionsOccupiedBy lists the section names entries stand under, in the README's order:
// Running, Ready, the dependency layers, Independent, Draft, the note sections, Done.
func (b boardView) sectionsOccupiedBy(entries []ViewEntry) []string {
	order := []string{runningHeading, readyHeading}
	for letter := 'A'; letter <= 'Y'; letter++ {
		order = append(order, layerHeading(string(letter)))
	}
	order = append(order, layerHeading(isolatedLayer), layerHeading(draftLayer))
	for _, typeLabel := range notesTypeOrder(b.out.Types) {
		order = append(order, noteSectionName(typeLabel))
	}
	order = append(order, doneHeading)

	occupied := make(map[string]bool, len(entries))
	for _, entry := range entries {
		occupied[entry.Section] = true
	}
	sections := []string{}
	for _, name := range order {
		if occupied[name] {
			sections = append(sections, name)
			// A name is listed once, whichever slot of the order it fills.
			occupied[name] = false
		}
	}
	return sections
}
