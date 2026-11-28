// view.go builds the board's view projection, which board-data.js and a live reader of the board show.
// It also computes the revisions of an entry and of the board.

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

// BoardDataFile names the render output that holds the view projection as a script.
const BoardDataFile = "board-data.js"

// boardDataPrefix and boardDataSuffix wrap the projection's JSON in BoardDataFile.
const (
	boardDataPrefix = "window.BOARD = "
	boardDataSuffix = ";\n"
)

// BoardView is the whole board as a viewer shows it, with every placement already decided as the README decides it.
type BoardView struct {
	// Rendered is the RFC 3339 UTC time the projection was built.
	Rendered   string `json:"rendered"`
	PrimeOwner string `json:"prime_owner"`
	// Types and Areas are the configured type labels and area labels, in configured order.
	Types []string `json:"types"`
	Areas []string `json:"areas"`
	// Sections names the sections the entries stand in, in README order.
	Sections []string `json:"sections"`
	// Projects holds the root project first, then every open project.
	Projects []ViewProject `json:"projects"`
	// Entries holds every task and note, done ones included, in render order.
	Entries []ViewEntry `json:"entries"`
}

// ViewProject is one open project, or the root, of a BoardView.
type ViewProject struct {
	Handle string   `json:"handle"`
	Slug   string   `json:"slug"`
	Title  string   `json:"title"`
	Brief  string   `json:"brief"`
	Labels []string `json:"labels"`
	// Parent is the handle of the project this one is nested under, empty for the root.
	Parent string `json:"parent"`
	// DesignFile names the project's design file, empty when it has none.
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
	Section string `json:"section"`
	// WaitsOn holds the handles of the unmet dependencies, the slug for one with no handle.
	WaitsOn []string  `json:"waits_on"`
	Brief   string    `json:"brief"`
	Body    string    `json:"body"`
	Owner   string    `json:"owner"`
	Recipe  string    `json:"recipe"`
	Created time.Time `json:"created,omitzero"`
	Revised time.Time `json:"revised,omitzero"`
	// DesignFile and DecisionFile are empty when the file does not exist.
	DesignFile   string `json:"design_file"`
	DecisionFile string `json:"decision_file"`
	// Revision identifies the stored entry, so a write can name the state it was made against.
	Revision string `json:"revision"`
}

// EntryRevision returns the lowercase hex SHA-256 of the entry's own JSON encoding, unknown keys included.
// It changes with every stored field, status included.
func EntryRevision(t Task) (string, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("encode entry %q: %w", t.Slug, err)
	}
	return hexSHA256(data), nil
}

// BoardRevision returns the hex SHA-256 of the bytes of board.json under boardPath.
// It returns "" with no error when the file is absent.
func BoardRevision(boardPath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(boardPath, boardFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read board revision: %w", err)
	}
	return hexSHA256(data), nil
}

// hexSHA256 returns the lowercase hex SHA-256 of data.
func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// buildView builds the projection of the entries as of now.
// stored holds the entries as stored, which the revisions are taken over;
// rendered holds the same entries as the board files show them, which everything else is read from.
func buildView(stored, rendered []Task, primeOwner string, out Outputs, now time.Time) (BoardView, error) {
	ordered, err := RenderOrder(rendered)
	if err != nil {
		return BoardView{}, err
	}
	board := newBoardView(ordered, primeOwner, out, now)

	revisions := make(map[string]string, len(stored))
	for _, task := range stored {
		if revisions[task.Slug], err = EntryRevision(task); err != nil {
			return BoardView{}, err
		}
	}

	view := BoardView{
		Rendered:   now.UTC().Format(time.RFC3339),
		PrimeOwner: primeOwner,
		Types:      orEmpty(out.Types),
		Areas:      orEmpty(out.Labels),
		Projects: []ViewProject{{
			Handle: rootProject.Handle,
			Slug:   RootSubject,
			Title:  rootProject.Title,
			Labels: []string{},
		}},
		Entries: []ViewEntry{},
	}
	view.Projects = append(view.Projects, board.viewProjectsUnder(rootProject, rootProject.Handle)...)

	used := map[string]bool{}
	for _, twl := range ordered {
		if twl.Kind == KindProject {
			continue
		}
		entry := board.viewEntry(twl)
		entry.Revision = revisions[twl.Slug]
		used[entry.Section] = true
		view.Entries = append(view.Entries, entry)
	}
	view.Sections = slices.DeleteFunc(sectionOrder(out.Types), func(name string) bool { return !used[name] })
	return view, nil
}

// viewProjectsUnder returns every open project below project, each before its own sub-projects, in README order.
// handle is project's handle, which its direct children name as their parent.
func (b boardView) viewProjectsUnder(project Task, handle string) []ViewProject {
	var projects []ViewProject
	for _, child := range b.childProjects(project) {
		projects = append(projects, ViewProject{
			Handle:     child.Handle,
			Slug:       child.Slug,
			Title:      child.Title,
			Brief:      child.Brief,
			Labels:     orEmpty(child.Labels),
			Parent:     handle,
			DesignFile: b.designFileName(child.Task),
		})
		projects = append(projects, b.viewProjectsUnder(child.Task, child.Handle)...)
	}
	return projects
}

// viewEntry is the projection of one task or note, without its revision.
func (b boardView) viewEntry(twl TaskWithLayer) ViewEntry {
	types, areas := []string{}, []string{}
	for _, label := range twl.Labels {
		if slices.Contains(b.out.Types, label) {
			types = append(types, label)
		} else {
			areas = append(areas, label)
		}
	}
	project := rootProject.Handle
	if twl.Project != "" {
		project = b.bySlug[twl.Project].Handle
	}
	waitsOn := []string{}
	for _, dep := range b.unmetDependencies(twl.Task) {
		if handle := b.bySlug[dep].Handle; handle != "" {
			waitsOn = append(waitsOn, handle)
		} else {
			waitsOn = append(waitsOn, dep)
		}
	}
	recipe := ""
	if twl.Kind == KindTask {
		recipe = b.out.recipeOf(twl.Task)
	}
	decisionFile := ""
	if twl.Discussed {
		decisionFile = DecisionFileName(twl.Slug)
	}
	return ViewEntry{
		Handle:       twl.Handle,
		Slug:         twl.Slug,
		Title:        twl.Title,
		Kind:         twl.Kind,
		Types:        types,
		Areas:        areas,
		Status:       statusText(twl.Status),
		Project:      project,
		Section:      b.viewSection(twl),
		WaitsOn:      waitsOn,
		Brief:        twl.Brief,
		Body:         twl.Body,
		Owner:        b.ownerOf(twl.Slug),
		Recipe:       recipe,
		Created:      twl.Created,
		Revised:      twl.Revised,
		DesignFile:   b.designFileName(twl.Task),
		DecisionFile: decisionFile,
	}
}

// designFileName is the name of the design file of t, or "" when t has no body and so no such file.
func (b boardView) designFileName(t Task) string {
	if t.Body == "" {
		return ""
	}
	return b.out.DesignPrefix + t.Slug + designFileSuffix
}

// viewSection names the README section a task or note stands in.
func (b boardView) viewSection(twl TaskWithLayer) string {
	switch {
	case isDone(twl.Task):
		return doneSection.name
	case twl.Kind == KindNote:
		return noteSectionName(noteType(twl.Task, b.out.Types))
	case twl.Layer == runningLayer:
		return runningSection.name
	case twl.Layer == readyLayer:
		return readySection.name
	}
	return layerSection(twl.Layer).name
}

// sectionOrder returns every section name an entry can stand in, in README order.
func sectionOrder(types []string) []string {
	order := []string{runningSection.name, readySection.name}
	for letter := 'A'; letter <= 'Y'; letter++ {
		order = append(order, layerSection(string(letter)).name)
	}
	order = append(order, layerSection(isolatedLayer).name, layerSection(draftLayer).name)
	for _, typeLabel := range notesTypeOrder(types) {
		order = append(order, noteSectionName(typeLabel))
	}
	return append(order, doneSection.name)
}

// boardDataScript is the content of BoardDataFile for view.
// The JSON keeps its HTML escaping, so no field's text can end a script element.
func boardDataScript(view BoardView) (string, error) {
	data, err := json.Marshal(view)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", BoardDataFile, err)
	}
	return boardDataPrefix + string(data) + boardDataSuffix, nil
}

// View returns the projection of the board as it stands, persisting nothing.
// The entries are migrated as a write migrates them, so each revision is the one a write checks.
func (b *Board) View() (BoardView, error) {
	store, err := b.loadStore()
	if err != nil {
		return BoardView{}, err
	}
	migrateLoaded(store)
	tasks := store.Tasks()
	return buildView(tasks, withMissingRecordsUndiscussed(b.boardPath, tasks), store.primeOwner, b.out, b.now())
}
