// write.go decodes a write of the page and runs it as one board write, with the checks the page's writes add.

package boardserve

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// The ops a write may name.
const (
	opCreate  = "create"
	opEdit    = "edit"
	opRelease = "release"
	opPromote = "promote"
	opDraft   = "draft"
	opUndraft = "undraft"
)

// writeOps are the ops in the order a refusal names them.
var writeOps = []string{opCreate, opEdit, opRelease, opPromote, opDraft, opUndraft}

// opFields holds the fields each op takes; an op absent here takes none.
var opFields = map[string][]string{
	opCreate: {"kind", "title", "labels", "brief", "body", "project", "depends_on"},
	opEdit:   {"title", "labels", "brief", "body", "project", "depends_on"},
}

// writeRequest is one write the page sends.
type writeRequest struct {
	Op   string `json:"op"`
	Slug string `json:"slug"`
	// Revision is the revision of the entry the page read, empty for a create.
	Revision string         `json:"revision"`
	Fields   map[string]any `json:"fields"`
}

// decodeWriteRequest decodes body, refusing a key writeRequest does not declare.
func decodeWriteRequest(body []byte) (writeRequest, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		return writeRequest{}, fmt.Errorf("invalid json: %w", err)
	}
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		if !slices.Contains([]string{"op", "slug", "revision", "fields"}, key) {
			return writeRequest{}, fmt.Errorf("unknown key %q: a write holds op, slug, revision and fields", key)
		}
	}
	var request writeRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return writeRequest{}, fmt.Errorf("invalid json: %w", err)
	}
	return request, nil
}

// applyWrite runs the one board write of request and returns the entry as the board then holds it.
// The entry is nil when the board lists none under the slug.
// The warning is empty unless the write succeeded with something the operator has to follow up.
// A write against an entry that moved since the page read it returns a *boardengine.StaleEntryError.
func (s *Server) applyWrite(request writeRequest) (entry *boardengine.ViewEntry, warning string, err error) {
	if !slices.Contains(writeOps, request.Op) {
		return nil, "", fmt.Errorf("unknown op %q: the ops are %s", request.Op, strings.Join(writeOps, ", "))
	}
	for _, field := range slices.Sorted(maps.Keys(request.Fields)) {
		if allowed := opFields[request.Op]; !slices.Contains(allowed, field) {
			list := strings.Join(allowed, ", ")
			if list == "" {
				list = "none"
			}
			return nil, "", fmt.Errorf("%s takes no field %q: its fields are %s", request.Op, field, list)
		}
	}
	if request.Slug == "" {
		return nil, "", errors.New("slug must be a non-empty string")
	}
	if request.Op != opCreate && request.Revision == "" {
		return nil, "", fmt.Errorf("%s of entry %q carries no revision, so nothing was written: way forward: reload the board and act on what it holds now", request.Op, request.Slug)
	}

	switch request.Op {
	case opCreate:
		err = s.create(request)
	case opEdit:
		err = s.edit(request)
	case opPromote:
		warning, err = s.promote(request)
	default:
		err = s.setStatus(request)
	}
	if err != nil {
		return nil, "", err
	}

	view, err := s.config.Board.View()
	if err != nil {
		return nil, "", err
	}
	return entryOf(view, request.Slug), warning, nil
}

// entryOf returns the entry of view under slug, or nil when there is none.
func entryOf(view boardengine.BoardView, slug string) *boardengine.ViewEntry {
	i := slices.IndexFunc(view.Entries, func(entry boardengine.ViewEntry) bool { return entry.Slug == slug })
	if i < 0 {
		return nil
	}
	return &view.Entries[i]
}

// create adds a new task or note, refusing a slug the board already holds as stale.
func (s *Server) create(request writeRequest) error {
	if kind, given := request.Fields["kind"]; given && kind != boardengine.KindTask && kind != boardengine.KindNote {
		return fmt.Errorf("the page creates a task or a note, not kind %v: way forward: create any other entry with lym board upsert", kind)
	}
	view, err := s.config.Board.View()
	if err != nil {
		return err
	}
	if existing := entryOf(view, request.Slug); existing != nil {
		return &boardengine.StaleEntryError{Slug: request.Slug, Fresh: existing}
	}
	return s.upsert(request, "")
}

// edit changes a task or note, which must stand at the revision of the request.
func (s *Server) edit(request writeRequest) error {
	stored, found, err := s.config.Board.GetTask(request.Slug)
	if err != nil {
		return err
	}
	if found && stored.Kind == boardengine.KindProject {
		return fmt.Errorf("entry %q is a project, and the page edits a task or a note: way forward: edit the project with lym board upsert", request.Slug)
	}
	return s.upsert(request, request.Revision)
}

// upsert writes the request's fields to its entry, which must stand at revision.
func (s *Server) upsert(request writeRequest, revision string) error {
	fields := maps.Clone(request.Fields)
	if fields == nil {
		fields = map[string]any{}
	}
	fields["slug"] = request.Slug
	_, err := s.config.Board.Expecting(request.Slug, revision).UpsertTaskReporting(fields)
	return err
}

// promote turns a note into a task and returns the board's warning about its decision record, if any.
func (s *Server) promote(request writeRequest) (string, error) {
	view, err := s.config.Board.View()
	if err != nil {
		return "", err
	}
	if entry := entryOf(view, request.Slug); entry != nil && entry.Kind == boardengine.KindTask {
		return "", fmt.Errorf("entry %q is a task already, and only a note is promoted: way forward: edit it, or promote a note", request.Slug)
	}
	result, err := s.config.Board.Expecting(request.Slug, request.Revision).PromoteTo(request.Slug, "", nil)
	return result.RecordWarning, err
}

// setStatus runs release, draft or undraft, whose rules read the entry's kind and status.
// The rules are judged on one view of the entry, and the write is held to the revision of that view.
func (s *Server) setStatus(request writeRequest) error {
	view, err := s.config.Board.View()
	if err != nil {
		return err
	}
	notATask := func(kind string) error {
		return fmt.Errorf("entry %q is a %s, and the page's %s takes a task: way forward: set a note's or project's status with lym board set-status", request.Slug, kind, request.Op)
	}
	if slices.ContainsFunc(view.Projects, func(project boardengine.ViewProject) bool { return project.Slug == request.Slug }) {
		return notATask(boardengine.KindProject)
	}
	entry := entryOf(view, request.Slug)
	if entry == nil || entry.Revision != request.Revision {
		return &boardengine.StaleEntryError{Slug: request.Slug, Revision: request.Revision, Removed: entry == nil, Fresh: entry}
	}
	if entry.Kind != boardengine.KindTask {
		return notATask(entry.Kind)
	}

	var status *string
	switch request.Op {
	case opRelease:
		if boardengine.IsRunStatus(&entry.Status) {
			return fmt.Errorf("entry %q is held by a run (status %q), and a release from the page would let its parent's dobby start it while the run holds its pair: way forward: release it with lym board set-status", request.Slug, entry.Status)
		}
	case opDraft:
		if entry.Status != "" {
			return fmt.Errorf("draft takes a task with no status; entry %q has status %q: way forward: release it first, or leave it", request.Slug, entry.Status)
		}
		draft := boardengine.StatusDraft
		status = &draft
	case opUndraft:
		switch {
		case entry.Status == boardengine.StatusDraft:
		case entry.Status == "":
			return fmt.Errorf("entry %q has no status, so it is released already and nothing was written", request.Slug)
		case boardengine.IsRunStatus(&entry.Status):
			return fmt.Errorf("entry %q is held by a run (status %q): way forward: clear it with lym board set-status", request.Slug, entry.Status)
		default:
			return fmt.Errorf("undraft takes a draft task; entry %q has status %q: way forward: release it with the page's release", request.Slug, entry.Status)
		}
	}
	_, err = s.config.Board.Expecting(request.Slug, request.Revision).SetStatusReleasingProject(request.Slug, status)
	return err
}
