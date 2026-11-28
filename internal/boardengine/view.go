// view.go builds the board's view projection, which is the data the viewer page reads.
// It also computes the revisions that tell two states apart.
// The projection comes from the same board view the README renders from, so a reader of it recomputes no layer or order.

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

// BoardDataFile is the name of the board file that holds the view projection as a script.
const BoardDataFile = "board-data.js"

// boardDataPrefix and boardDataSuffix wrap the projection's JSON in BoardDataFile.
const (
	boardDataPrefix = "window.BOARD = "
	boardDataSuffix = ";\n"
)

// BoardView is the whole board as a viewer reads it.
type BoardView struct {
	// Rendered is the RFC 3339 UTC time the projection was built.
	Rendered   string `json:"rendered"`
	PrimeOwner string `json:"prime_owner"`
	// Types and Areas are the configured type and area labels, in configured order.
	Types []string `json:"types"`
	Areas []string `json:"areas"`
	// Sections are the README section names the entries fall under, in README order.
	Sections []string      `json:"sections"`
	Projects []ViewProject `json:"projects"`
	Entries  []ViewEntry   `json:"entries"`
}

// ViewProject is one open project of a BoardView, or the root project.
type ViewProject struct {
	Handle string   `json:"handle"`
	Slug   string   `json:"slug"`
	Title  string   `json:"title"`
	Brief  string   `json:"brief"`
	Labels []string `json:"labels"`
	// Parent is the handle of the project this one sits under, empty for the root.
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
	// Project is the handle of the entry's project.
	Project string `json:"project"`
	Section string `json:"section"`
	// WaitsOn holds the handles of the unmet dependencies, the slug for one the board holds no handle for.
	WaitsOn []string  `json:"waits_on"`
	Brief   string    `json:"brief"`
	Body    string    `json:"body"`
	Owner   string    `json:"owner"`
	Recipe  string    `json:"recipe"`
	Created time.Time `json:"created,omitzero"`
	Revised time.Time `json:"revised,omitzero"`
	// DesignFile and DecisionFile are empty when the entry has no such file.
	DesignFile   string `json:"design_file"`
	DecisionFile string `json:"decision_file"`
	// Revision is the revision of the stored entry, which a write states to say which state it read.
	Revision string `json:"revision"`
}

// EntryRevision returns the revision of t: the hex SHA-256 of its encoding, unknown keys included.
// Every stored field, status included, changes it.
func EntryRevision(t Task) (string, error) {
	encoded, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("encode entry %q: %w", t.Slug, err)
	}
	return hexSHA256(encoded), nil
}

// BoardRevision returns the revision of the board at boardPath: the hex SHA-256 of its board.json.
// A board with no board.json has the empty revision.
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

// View returns the board's view projection as of now, and persists nothing.
// The entries are migrated in memory as a write would migrate them, so their revisions are the ones a write checks.
func (b *Board) View() (BoardView, error) {
	store, err := b.loadStore()
	if err != nil {
		return BoardView{}, err
	}
	migrateLoaded(store)
	tasks := store.Tasks()
	return buildView(tasks, withMissingRecordsUndiscussed(b.boardPath, tasks), store.primeOwner, b.out, b.now())
}

// boardDataScript returns the content of BoardDataFile for view.
// The encoding's HTML escaping stays on, so no field of the view can end the script element that loads it.
func boardDataScript(view BoardView) (string, error) {
	encoded, err := json.Marshal(view)
	if err != nil {
		return "", fmt.Errorf("encode board view: %w", err)
	}
	return boardDataPrefix + string(encoded) + boardDataSuffix, nil
}

// buildView builds the projection of the entries rendered, taking each entry's revision over its stored form in stored.
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

	view := BoardView{
		Rendered:   now.UTC().Format(time.RFC3339),
		PrimeOwner: primeOwner,
		Types:      append([]string{}, out.Types...),
		Areas:      append([]string{}, out.Labels...),
		Projects:   board.viewProjects(),
		Entries:    []ViewEntry{},
	}
	for _, twl := range ordered {
		if twl.Kind != KindTask && twl.Kind != KindNote {
			continue
		}
		revision, err := EntryRevision(storedBySlug[twl.Slug])
		if err != nil {
			return BoardView{}, err
		}
		entry := board.viewEntry(twl)
		entry.Revision = revision
		view.Entries = append(view.Entries, entry)
	}
	view.Sections = board.viewSections(view.Entries)
	return view, nil
}

// viewProjects returns the root project and then every open project in README order, each naming the one it sits under.
func (b boardView) viewProjects() []ViewProject {
	projects := []ViewProject{{Handle: rootProject.Handle, Slug: RootSubject, Title: rootProject.Title, Labels: []string{}}}
	var walk func(parent Task, parentHandle string)
	walk = func(parent Task, parentHandle string) {
		for _, child := range b.childProjects(parent) {
			projects = append(projects, ViewProject{
				Handle:     child.Handle,
				Slug:       child.Slug,
				Title:      child.Title,
				Brief:      child.Brief,
				Labels:     append([]string{}, child.Labels...),
				Parent:     parentHandle,
				DesignFile: designFileName(child.Task, b.out.DesignPrefix),
			})
			walk(child.Task, child.Handle)
		}
	}
	walk(rootProject, rootProject.Handle)
	return projects
}

// viewEntry returns the projection of twl without its revision.
func (b boardView) viewEntry(twl TaskWithLayer) ViewEntry {
	entry := ViewEntry{
		Handle:     twl.Handle,
		Slug:       twl.Slug,
		Title:      twl.Title,
		Kind:       twl.Kind,
		Types:      []string{},
		Areas:      []string{},
		Project:    rootProject.Handle,
		Section:    b.sectionName(twl),
		WaitsOn:    []string{},
		Brief:      twl.Brief,
		Body:       twl.Body,
		Owner:      b.ownerOf(twl.Slug),
		Created:    twl.Created,
		Revised:    twl.Revised,
		DesignFile: designFileName(twl.Task, b.out.DesignPrefix),
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
	if project, onBoard := b.bySlug[twl.Project]; onBoard {
		entry.Project = project.Handle
	}
	for _, dep := range b.unmetDependencies(twl.Task) {
		if on, onBoard := b.bySlug[dep]; onBoard && on.Handle != "" {
			dep = on.Handle
		}
		entry.WaitsOn = append(entry.WaitsOn, dep)
	}
	if twl.Kind == KindTask {
		entry.Recipe = b.out.recipeOf(twl.Task)
	}
	if twl.Discussed {
		entry.DecisionFile = DecisionFileName(twl.Slug)
	}
	return entry
}

// sectionName is the name of the README section that lists the task or note twl.
func (b boardView) sectionName(twl TaskWithLayer) string {
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

// viewSections returns the section names entries fall under, in README order.
// The Ready section is always listed, since the README always writes it.
func (b boardView) viewSections(entries []ViewEntry) []string {
	held := map[string]bool{readySection.name: true}
	for _, entry := range entries {
		held[entry.Section] = true
	}
	order := []string{runningSection.name, readySection.name}
	for letter := 'A'; letter <= 'Y'; letter++ {
		order = append(order, layerSection(string(letter)).name)
	}
	order = append(order, layerSection(isolatedLayer).name, layerSection(draftLayer).name)
	for _, typeLabel := range notesTypeOrder(b.out.Types) {
		order = append(order, noteSectionName(typeLabel))
	}
	order = append(order, doneSection.name)
	return slices.DeleteFunc(dedupe(order), func(name string) bool { return !held[name] })
}

// designFileName is the name of the design file of t, empty when t has no body and so no design file.
func designFileName(t Task, designPrefix string) string {
	if t.Body == "" {
		return ""
	}
	return designPrefix + t.Slug + designFileSuffix
}
