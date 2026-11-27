// view.go builds the board's view projection, the JSON a viewer page shows, and the revisions of entries and boards.
// The projection is read from the same ordered view the README renders from, so a viewer recomputes nothing.

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

// BoardDataFile is the rendered file that holds the view projection as a script a page can load.
const BoardDataFile = "board-data.js"

// boardDataPrefix and boardDataSuffix wrap the projection's JSON into the script of BoardDataFile.
const (
	boardDataPrefix = "window.BOARD = "
	boardDataSuffix = ";\n"
)

// BoardView is the board as a viewer shows it.
type BoardView struct {
	// Rendered is the UTC time the view was built, in RFC 3339.
	Rendered   string   `json:"rendered"`
	PrimeOwner string   `json:"prime_owner"`
	Types      []string `json:"types"`
	Areas      []string `json:"areas"`
	// Sections are the names of the sections the entries stand in, in README order.
	Sections []string `json:"sections"`
	// Projects is the root project followed by every open project, each after its parent.
	Projects []ViewProject `json:"projects"`
	// Entries are the tasks and notes, done ones included.
	Entries []ViewEntry `json:"entries"`
}

// ViewProject is one project of a BoardView.
type ViewProject struct {
	Handle string   `json:"handle"`
	Slug   string   `json:"slug"`
	Title  string   `json:"title"`
	Brief  string   `json:"brief"`
	Labels []string `json:"labels"`
	// Parent is the handle of the project this one is listed under, empty for the root.
	Parent string `json:"parent"`
	// DesignFile is empty when the project has no design file.
	DesignFile string `json:"design_file"`
}

// ViewEntry is one task or note of a BoardView.
type ViewEntry struct {
	Handle string   `json:"handle"`
	Slug   string   `json:"slug"`
	Title  string   `json:"title"`
	Kind   string   `json:"kind"`
	Types  []string `json:"types"`
	Areas  []string `json:"areas"`
	// Status is empty for an entry with none.
	Status string `json:"status"`
	// Project is the handle of the entry's project, the root's for an entry under none.
	Project string `json:"project"`
	// Section is the name of the README section the entry stands in.
	Section string `json:"section"`
	// WaitsOn holds the unmet dependencies by handle, by slug for one with no handle.
	WaitsOn []string `json:"waits_on"`
	Brief   string   `json:"brief"`
	Body    string   `json:"body"`
	// Owner is the effective owner machine.
	Owner string `json:"owner"`
	// Recipe is the recipe a task runs, empty for a note.
	Recipe  string    `json:"recipe"`
	Created time.Time `json:"created,omitzero"`
	Revised time.Time `json:"revised,omitzero"`
	// DesignFile and DecisionFile are empty when the file does not exist.
	DesignFile   string `json:"design_file"`
	DecisionFile string `json:"decision_file"`
	// Revision is the revision of the stored entry.
	Revision string `json:"revision"`
}

// EntryRevision returns the revision of t: the hex SHA-256 of its encoding, unknown keys included.
// Every stored field counts, the status included.
func EntryRevision(t Task) (string, error) {
	encoded, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("encode entry %q: %w", t.Slug, err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// BoardRevision returns the hex SHA-256 of the board.json bytes under boardPath, or "" when there is no board.json.
func BoardRevision(boardPath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(boardPath, boardFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read board revision: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// buildView projects the board as of now.
// rendered are the entries as the board files show them.
// stored are the same entries as board.json holds them, which each entry's revision is taken over.
func buildView(stored, rendered []Task, primeOwner string, out Outputs, now time.Time) (BoardView, error) {
	ordered, err := RenderOrder(rendered)
	if err != nil {
		return BoardView{}, err
	}
	board := newBoardView(ordered, primeOwner, out, now)

	view := BoardView{
		Rendered:   now.UTC().Format(time.RFC3339),
		PrimeOwner: primeOwner,
		Types:      append([]string{}, out.Types...),
		Areas:      append([]string{}, out.Labels...),
		Projects: []ViewProject{{
			Handle: rootProject.Handle,
			Slug:   RootSubject,
			Title:  rootProject.Title,
			Labels: []string{},
		}},
		Entries: []ViewEntry{},
	}

	var addChildren func(project Task, handle string)
	addChildren = func(project Task, handle string) {
		for _, child := range board.childProjects(project) {
			view.Projects = append(view.Projects, ViewProject{
				Handle:     child.Handle,
				Slug:       child.Slug,
				Title:      child.Title,
				Brief:      child.Brief,
				Labels:     append([]string{}, child.Labels...),
				Parent:     handle,
				DesignFile: designFileName(child.Task, out.DesignPrefix),
			})
			addChildren(child.Task, child.Handle)
		}
	}
	addChildren(rootProject, rootProject.Handle)

	storedBySlug := make(map[string]Task, len(stored))
	for _, task := range stored {
		storedBySlug[task.Slug] = task
	}
	used := map[string]bool{}
	for _, twl := range ordered {
		if twl.Kind == KindProject {
			continue
		}
		entry, err := board.viewEntry(twl, storedBySlug[twl.Slug])
		if err != nil {
			return BoardView{}, err
		}
		used[entry.Section] = true
		view.Entries = append(view.Entries, entry)
	}
	view.Sections = usedSections(used, out.Types)
	return view, nil
}

// viewEntry projects one task or note, taking its revision over stored.
func (b boardView) viewEntry(twl TaskWithLayer, stored Task) (ViewEntry, error) {
	revision, err := EntryRevision(stored)
	if err != nil {
		return ViewEntry{}, err
	}
	entry := ViewEntry{
		Handle:     twl.Handle,
		Slug:       twl.Slug,
		Title:      twl.Title,
		Kind:       twl.Kind,
		Types:      []string{},
		Areas:      []string{},
		Status:     statusText(twl.Status),
		Project:    rootProject.Handle,
		Section:    sectionName(twl, b.out.Types),
		WaitsOn:    []string{},
		Brief:      twl.Brief,
		Body:       twl.Body,
		Owner:      b.ownerOf(twl.Slug),
		Created:    twl.Created,
		Revised:    twl.Revised,
		DesignFile: designFileName(twl.Task, b.out.DesignPrefix),
		Revision:   revision,
	}
	for _, label := range twl.Labels {
		if slices.Contains(b.out.Types, label) {
			entry.Types = append(entry.Types, label)
		} else {
			entry.Areas = append(entry.Areas, label)
		}
	}
	if project, onBoard := b.bySlug[twl.Project]; onBoard {
		entry.Project = project.Handle
	}
	for _, dep := range b.unmetDependencies(twl.Task) {
		if depended, found := b.bySlug[dep]; found && depended.Handle != "" {
			dep = depended.Handle
		}
		entry.WaitsOn = append(entry.WaitsOn, dep)
	}
	if twl.Kind == KindTask {
		entry.Recipe = b.out.recipeOf(twl.Task)
	}
	if twl.Discussed {
		entry.DecisionFile = DecisionFileName(twl.Slug)
	}
	return entry, nil
}

// designFileName is the name of t's design file, or "" when t has no body and so no design file.
func designFileName(t Task, designPrefix string) string {
	if t.Body == "" {
		return ""
	}
	return designPrefix + t.Slug + ".md"
}

// sectionName is the name of the README section a task or note stands in.
func sectionName(twl TaskWithLayer, types []string) string {
	switch {
	case isDone(twl.Task):
		return doneSection.name
	case twl.Kind == KindNote:
		return noteSectionName(noteType(twl.Task, types))
	case twl.Layer == runningLayer:
		return runningSection.name
	case twl.Layer == readyLayer:
		return readySection.name
	}
	return layerSection(twl.Layer).name
}

// noteSectionName is the Notes subsection name of a type label, "" being the label of an unconfigured type.
func noteSectionName(typeLabel string) string {
	if typeLabel == "" {
		return otherNotesHeading
	}
	return typeHeading(typeLabel)
}

// usedSections returns the section names in used, in README order.
func usedSections(used map[string]bool, types []string) []string {
	order := []string{runningSection.name, readySection.name}
	for letter := 'A'; letter <= 'Y'; letter++ {
		order = append(order, layerSection(string(letter)).name)
	}
	order = append(order, layerSection(isolatedLayer).name, layerSection(draftLayer).name)
	for _, typeLabel := range notesTypeOrder(types) {
		order = append(order, noteSectionName(typeLabel))
	}
	order = append(order, doneSection.name)

	sections := []string{}
	for _, name := range order {
		if used[name] && !slices.Contains(sections, name) {
			sections = append(sections, name)
		}
	}
	return sections
}

// boardDataScript renders view as the script of BoardDataFile.
// The encoder escapes <, > and &, so no field can close the script element that holds it.
func boardDataScript(view BoardView) (string, error) {
	encoded, err := json.Marshal(view)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", BoardDataFile, err)
	}
	return boardDataPrefix + string(encoded) + boardDataSuffix, nil
}

// View returns the board's view projection as of now and persists nothing.
// The entries are read as the next write would load them, so each revision is the one a write checks.
func (b *Board) View() (BoardView, error) {
	store, err := b.loadStore()
	if err != nil {
		return BoardView{}, err
	}
	migrateLoaded(store)
	loaded := store.Tasks()
	return buildView(loaded, withMissingRecordsUndiscussed(b.boardPath, loaded), store.primeOwner, b.out, b.now())
}
