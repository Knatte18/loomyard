// view.go builds the board's view projection: the entries as the README places them, as data a viewer reads.
// It also computes the revisions that identify one stored entry and one state of the entry store.

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

// BoardDataFile is the name of the rendered file that holds the view projection as a script.
const BoardDataFile = "board-data.js"

// boardDataPrefix opens BoardDataFile's one statement.
const boardDataPrefix = "window.BOARD = "

// EntryStoreFile is the name of the entry store in the board directory, for a caller that watches it for changes.
const EntryStoreFile = boardFile

// BoardView is the whole board as a viewer shows it, placing every entry where the README does.
type BoardView struct {
	// Rendered is the RFC 3339 UTC time the view was built.
	Rendered   string `json:"rendered"`
	PrimeOwner string `json:"prime_owner"`
	// Types and Areas are the configured type labels and area labels, each in configured order.
	Types []string `json:"types"`
	Areas []string `json:"areas"`
	// Sections names the sections the entries sit in, in the README's order.
	Sections []string `json:"sections"`
	// Projects holds the root first, then every open project in README order.
	Projects []ViewProject `json:"projects"`
	// Entries holds every task and note, done ones included, in render order.
	Entries []ViewEntry `json:"entries"`
}

// ViewProject is one project of a BoardView.
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
	// Status is empty for an entry with no status.
	Status string `json:"status"`
	// Project is the handle of the entry's project, the root's for none.
	Project string `json:"project"`
	Section string `json:"section"`
	// WaitsOn holds the handles of the dependencies not yet met, or the slug of one with no handle.
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
	// Revision identifies the stored entry, so a write can tell whether it changed since the view was built.
	Revision string `json:"revision"`
}

// EntryRevision returns the lowercase hex SHA-256 of t's JSON encoding, unknown keys included.
// It changes with any stored field of the entry.
func EntryRevision(t Task) (string, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("encode entry %q: %w", t.Slug, err)
	}
	return hexSHA256(data), nil
}

// BoardRevision returns the lowercase hex SHA-256 of the entry store's bytes in the board directory boardPath.
// It returns "" and no error when the store file is absent.
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

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// View returns the board's view projection as of now, and persists nothing.
// The entries are migrated as a write migrates them, so each revision matches the one a write judges.
func (b *Board) View() (BoardView, error) {
	store, err := b.loadStore()
	if err != nil {
		return BoardView{}, err
	}
	migrateLoaded(store)
	tasks := store.Tasks()
	return buildView(tasks, withMissingRecordsUndiscussed(b.boardPath, tasks), store.primeOwner, b.out, b.now())
}

// buildView projects the entries as the board files render them, every time as of now.
// rendered is stored with the discussed flag cleared for each entry whose decision record is missing:
// placement and file names follow rendered, and each entry's revision follows stored.
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
		Types:      orEmpty(out.Types),
		Areas:      orEmpty(out.Labels),
		Sections:   board.sectionNames(),
		Projects:   []ViewProject{{Handle: rootProject.Handle, Slug: RootSubject, Title: rootProject.Title, Labels: []string{}}},
		Entries:    []ViewEntry{},
	}

	var addProjectsUnder func(parent Task, parentHandle string)
	addProjectsUnder = func(parent Task, parentHandle string) {
		for _, child := range board.childProjects(parent) {
			view.Projects = append(view.Projects, ViewProject{
				Handle:     child.Handle,
				Slug:       child.Slug,
				Title:      child.Title,
				Brief:      child.Brief,
				Labels:     orEmpty(child.Labels),
				Parent:     parentHandle,
				DesignFile: designFileName(child.Task, out.DesignPrefix),
			})
			addProjectsUnder(child.Task, child.Handle)
		}
	}
	addProjectsUnder(rootProject, rootProject.Handle)

	for _, twl := range ordered {
		if twl.Kind == KindProject {
			continue
		}
		revision, err := EntryRevision(storedBySlug[twl.Slug])
		if err != nil {
			return BoardView{}, err
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
			Section:    board.sectionName(twl),
			WaitsOn:    []string{},
			Brief:      twl.Brief,
			Body:       twl.Body,
			Owner:      board.ownerOf(twl.Slug),
			Created:    twl.Created,
			Revised:    twl.Revised,
			DesignFile: designFileName(twl.Task, out.DesignPrefix),
			Revision:   revision,
		}
		for _, label := range twl.Labels {
			if slices.Contains(out.Types, label) {
				entry.Types = append(entry.Types, label)
			} else {
				entry.Areas = append(entry.Areas, label)
			}
		}
		if project, onBoard := board.bySlug[twl.Project]; onBoard {
			entry.Project = project.Handle
		}
		for _, dep := range board.unmetDependencies(twl.Task) {
			if waited, onBoard := board.bySlug[dep]; onBoard && waited.Handle != "" {
				dep = waited.Handle
			}
			entry.WaitsOn = append(entry.WaitsOn, dep)
		}
		if twl.Kind == KindTask {
			entry.Recipe = out.recipeOf(twl.Task)
		}
		if twl.Discussed {
			entry.DecisionFile = DecisionFileName(twl.Slug)
		}
		view.Entries = append(view.Entries, entry)
	}
	return view, nil
}

// designFileName is the name of t's design file, empty when t has no body and so no design file.
func designFileName(t Task, designPrefix string) string {
	if t.Body == "" {
		return ""
	}
	return designPrefix + t.Slug + designFileSuffix
}

// sectionName is the board files' section that holds the task or note twl.
func (b boardView) sectionName(twl TaskWithLayer) string {
	switch {
	case isDone(twl.Task):
		return doneSection.name
	case twl.Kind == KindNote:
		if typeLabel := noteType(twl.Task, b.out.Types); typeLabel != "" {
			return typeHeading(typeLabel)
		}
		return otherNotesHeading
	case twl.Layer == runningLayer:
		return runningSection.name
	case twl.Layer == readyLayer:
		return readySection.name
	default:
		return layerSection(twl.Layer).name
	}
}

// sectionNames lists the sections that hold a task or note, once each, in the board files' order:
// the task sections, then the note sections, then Done.
func (b boardView) sectionNames() []string {
	names := []string{}
	add := func(twl TaskWithLayer) {
		if name := b.sectionName(twl); !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	// Render order already runs the open tasks by section, which it does not do for notes.
	for _, twl := range b.ordered {
		if twl.Kind == KindTask && !isDone(twl.Task) {
			add(twl)
		}
	}
	for _, typeLabel := range notesTypeOrder(b.out.Types) {
		for _, twl := range b.ordered {
			if twl.Kind == KindNote && !isDone(twl.Task) && noteType(twl.Task, b.out.Types) == typeLabel {
				add(twl)
			}
		}
	}
	for _, twl := range b.ordered {
		if twl.Kind != KindProject && isDone(twl.Task) {
			add(twl)
		}
	}
	return names
}

// boardDataScript is the content of BoardDataFile: one statement assigning view's JSON.
// The JSON keeps HTML escaping, so no field can close a script element that holds it.
func boardDataScript(view BoardView) (string, error) {
	data, err := json.Marshal(view)
	if err != nil {
		return "", fmt.Errorf("encode board view: %w", err)
	}
	return boardDataPrefix + string(data) + ";\n", nil
}
