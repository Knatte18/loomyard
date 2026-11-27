// write.go runs one page write against the board: its field rules, its revision and the board method of its op.

package boardserve

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// The ops a write request may name.
const (
	opCreate  = "create"
	opEdit    = "edit"
	opRelease = "release"
	opPromote = "promote"
	opDraft   = "draft"
	opUndraft = "undraft"
)

// opFields lists the fields each op accepts.
var opFields = map[string][]string{
	opCreate:  {"kind", "title", "labels", "brief", "body", "project", "depends_on"},
	opEdit:    {"title", "labels", "brief", "body", "project", "depends_on"},
	opRelease: nil,
	opPromote: nil,
	opDraft:   nil,
	opUndraft: nil,
}

// writeRequest is the body of one write: an op on the entry slug, whose revision is the one the page read.
type writeRequest struct {
	Op       string         `json:"op"`
	Slug     string         `json:"slug"`
	Revision string         `json:"revision"`
	Fields   map[string]any `json:"fields"`
}

// writeRequestKeys are the keys a write request's body may hold.
var writeRequestKeys = []string{"op", "slug", "revision", "fields"}

// decodeWriteRequest decodes body as one write request, and refuses a key a write request does not have.
// The page writes the body, so a key the server would ignore is a change the operator asked for and would lose.
func decodeWriteRequest(body []byte) (writeRequest, error) {
	var request writeRequest
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		return request, err
	}
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		if !slices.Contains(writeRequestKeys, key) {
			return request, fmt.Errorf("unknown key %q", key)
		}
	}
	err := json.Unmarshal(body, &request)
	return request, err
}

// applyWrite runs request's write and returns the entry as it left it.
// A stale revision is a *boardengine.StaleEntryError, and every refusal writes nothing.
func (s *Server) applyWrite(request writeRequest) (*boardengine.ViewEntry, error) {
	op, slug := request.Op, request.Slug
	allowed, known := opFields[op]
	if !known {
		return nil, fmt.Errorf("unknown op %q, so nothing was written: way forward: send one of %s, %s, %s, %s, %s or %s",
			op, opCreate, opEdit, opRelease, opPromote, opDraft, opUndraft)
	}
	if op != opCreate && request.Revision == "" {
		return nil, fmt.Errorf("%s of entry %q carries no revision, so nothing was written: way forward: reload the board and act on what it holds now", op, slug)
	}
	for _, field := range slices.Sorted(maps.Keys(request.Fields)) {
		if !slices.Contains(allowed, field) {
			list := "no field"
			if len(allowed) > 0 {
				list = "only " + strings.Join(allowed, ", ")
			}
			return nil, fmt.Errorf("%s takes %s, not %q, so nothing was written", op, list, field)
		}
	}

	var err error
	switch op {
	case opCreate:
		err = s.create(slug, request.Fields)
	case opEdit:
		err = s.upsert(slug, request.Revision, request.Fields)
	case opPromote:
		err = s.promote(slug, request.Revision)
	default:
		err = s.changeStatus(op, slug, request.Revision)
	}
	if err != nil {
		return nil, err
	}
	_, entry, err := s.viewEntry(slug)
	return entry, err
}

// viewEntry returns the current view and its task or note slug, nil when it holds none.
func (s *Server) viewEntry(slug string) (boardengine.BoardView, *boardengine.ViewEntry, error) {
	view, err := s.config.Board.View()
	if err != nil {
		return view, nil, err
	}
	index := slices.IndexFunc(view.Entries, func(e boardengine.ViewEntry) bool { return e.Slug == slug })
	if index < 0 {
		return view, nil, nil
	}
	return view, &view.Entries[index], nil
}

// upsert writes fields onto the entry slug while it has revision, "" for an entry that does not exist yet.
func (s *Server) upsert(slug, revision string, fields map[string]any) error {
	payload := maps.Clone(fields)
	if payload == nil {
		payload = map[string]any{}
	}
	payload["slug"] = slug
	_, err := s.config.Board.Expecting(slug, revision).UpsertTaskReporting(payload)
	return err
}

func (s *Server) create(slug string, fields map[string]any) error {
	if kind := fields["kind"]; kind != boardengine.KindTask && kind != boardengine.KindNote {
		return fmt.Errorf("the page creates a task or a note, not kind %q, so nothing was written: way forward: create it with lym board upsert", fmt.Sprint(kind))
	}
	_, existing, err := s.viewEntry(slug)
	if err != nil {
		return err
	}
	if existing != nil {
		return &boardengine.StaleEntryError{Slug: slug, Create: true, Fresh: existing}
	}
	return s.upsert(slug, "", fields)
}

// promote makes the note slug a task. A task is refused here, since the board's own promote leaves one unchanged.
func (s *Server) promote(slug, revision string) error {
	_, entry, err := s.viewEntry(slug)
	if err != nil {
		return err
	}
	if entry != nil && entry.Kind == boardengine.KindTask {
		return fmt.Errorf("entry %q is a task already, and only a note is promoted: way forward: edit it, or promote a note", slug)
	}
	_, err = s.config.Board.Expecting(slug, revision).PromoteTo(slug, "", nil)
	return err
}

// changeStatus runs a release, draft or undraft.
// Its rule reads the entry's kind and status in a view, and the write then expects that view's revision.
func (s *Server) changeStatus(op, slug, revision string) error {
	view, entry, err := s.viewEntry(slug)
	if err != nil {
		return err
	}
	notATask := func(kind string) error {
		return fmt.Errorf("entry %q is a %s, and the page's %s takes a task: way forward: set a note's or project's status with lym board set-status", slug, kind, op)
	}
	if entry == nil {
		if slices.ContainsFunc(view.Projects, func(p boardengine.ViewProject) bool { return p.Slug == slug }) {
			return notATask(boardengine.KindProject)
		}
		return &boardengine.StaleEntryError{Slug: slug, Removed: true}
	}
	if entry.Revision != revision {
		return &boardengine.StaleEntryError{Slug: slug, Fresh: entry}
	}
	if entry.Kind != boardengine.KindTask {
		return notATask(entry.Kind)
	}

	status, err := statusAfter(op, slug, entry.Status)
	if err != nil {
		return err
	}
	_, err = s.config.Board.Expecting(slug, revision).SetStatusReleasingProject(slug, status)
	return err
}

// statusAfter is the status op gives the task slug whose status is current, nil for none, or the op's refusal.
func statusAfter(op, slug, current string) (*string, error) {
	running := boardengine.IsRunStatus(&current)
	switch op {
	case opRelease:
		if running {
			return nil, fmt.Errorf("entry %q is held by a run (status %q), and a release from the page would let its parent's dobby start it while the run holds its pair: way forward: release it with lym board set-status", slug, current)
		}
		return nil, nil
	case opDraft:
		if current != "" {
			return nil, fmt.Errorf("draft takes a task with no status; entry %q has status %q: way forward: release it first, or leave it", slug, current)
		}
		draft := boardengine.StatusDraft
		return &draft, nil
	}
	switch {
	case current == boardengine.StatusDraft:
		return nil, nil
	case current == "":
		return nil, fmt.Errorf("entry %q has no status, so it is released already and nothing was written", slug)
	case running:
		return nil, fmt.Errorf("entry %q is held by a run (status %q): way forward: clear it with lym board set-status", slug, current)
	}
	return nil, fmt.Errorf("undraft takes a draft task; entry %q has status %q: way forward: release it with the page's release", slug, current)
}
