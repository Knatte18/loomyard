// write.go decodes the page's write requests and runs each as one board write against the revision it read.

package boardserve

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// The operations a write request names.
const (
	opCreate  = "create"
	opEdit    = "edit"
	opRelease = "release"
	opPromote = "promote"
	opDraft   = "draft"
	opUndraft = "undraft"
)

// operations lists the operations in the order a refusal names them.
var operations = []string{opCreate, opEdit, opRelease, opPromote, opDraft, opUndraft}

// operationFields lists the fields each operation takes; an operation not listed takes none.
var operationFields = map[string][]string{
	opCreate: {"kind", "title", "labels", "brief", "body", "project", "depends_on"},
	opEdit:   {"title", "labels", "brief", "body", "project", "depends_on"},
}

// writeRequest is the body of one write.
type writeRequest struct {
	Op   string `json:"op"`
	Slug string `json:"slug"`
	// Revision is the entry revision the writer read, which a create has none of.
	Revision string         `json:"revision"`
	Fields   map[string]any `json:"fields"`
}

// requestKeys are the keys a write request may hold.
var requestKeys = []string{"op", "slug", "revision", "fields"}

// decodeWriteRequest decodes body, refusing a key a write request does not hold,
// so a mistyped request is never run as a smaller one.
func decodeWriteRequest(body []byte) (writeRequest, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		return writeRequest{}, fmt.Errorf("the write is not a JSON object: %w", err)
	}
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		if !slices.Contains(requestKeys, key) {
			return writeRequest{}, fmt.Errorf("the write holds the unknown key %q: way forward: send only %s", key, strings.Join(requestKeys, ", "))
		}
	}
	var request writeRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return writeRequest{}, fmt.Errorf("decode the write: %w", err)
	}
	return request, nil
}

// applyWrite runs request and returns the written entry as the board's projection then shows it.
// A request made against a stale entry returns a *boardengine.StaleEntryError and writes nothing.
func (s *Server) applyWrite(request writeRequest) (*boardengine.ViewEntry, error) {
	if !slices.Contains(operations, request.Op) {
		return nil, fmt.Errorf("unknown op %q: way forward: send one of %s", request.Op, strings.Join(operations, ", "))
	}
	allowed := operationFields[request.Op]
	for _, field := range slices.Sorted(maps.Keys(request.Fields)) {
		if slices.Contains(allowed, field) {
			continue
		}
		takes := "no fields"
		if len(allowed) > 0 {
			takes = "only " + strings.Join(allowed, ", ")
		}
		return nil, fmt.Errorf("%s takes no field %q: way forward: send %s", request.Op, field, takes)
	}
	if request.Op != opCreate && request.Revision == "" {
		return nil, fmt.Errorf("%s of entry %q carries no revision, so nothing was written: way forward: reload the board and act on what it holds now", request.Op, request.Slug)
	}

	var err error
	switch request.Op {
	case opCreate:
		err = s.create(request)
	case opEdit:
		err = s.upsert(request)
	case opPromote:
		err = s.promote(request)
	default:
		err = s.changeStatus(request)
	}
	if err != nil {
		return nil, err
	}
	_, entry, err := s.viewEntry(request.Slug)
	return entry, err
}

// viewEntry returns the board's current projection and its task or note of slug, nil when it holds none.
func (s *Server) viewEntry(slug string) (boardengine.BoardView, *boardengine.ViewEntry, error) {
	view, err := s.config.Board.View()
	if err != nil {
		return boardengine.BoardView{}, nil, err
	}
	if i := slices.IndexFunc(view.Entries, func(entry boardengine.ViewEntry) bool { return entry.Slug == slug }); i >= 0 {
		return view, &view.Entries[i], nil
	}
	return view, nil, nil
}

// create files a new entry.
// A slug the board already holds is refused as stale, so a create never edits an entry.
func (s *Server) create(request writeRequest) error {
	_, existing, err := s.viewEntry(request.Slug)
	if err != nil {
		return err
	}
	if existing != nil {
		return &boardengine.StaleEntryError{Slug: request.Slug, Fresh: existing}
	}
	return s.upsert(request)
}

// upsert writes the request's fields to its entry, expecting the request's revision.
func (s *Server) upsert(request writeRequest) error {
	fields := map[string]any{"slug": request.Slug}
	maps.Copy(fields, request.Fields)
	_, err := s.config.Board.Expecting(request.Slug, request.Revision).UpsertTaskReporting(fields)
	return err
}

// promote makes a note a task.
func (s *Server) promote(request writeRequest) error {
	_, entry, err := s.viewEntry(request.Slug)
	if err != nil {
		return err
	}
	if entry != nil && entry.Kind == boardengine.KindTask {
		return fmt.Errorf("entry %q is a task already, and only a note is promoted: way forward: edit it, or promote a note", request.Slug)
	}
	_, err = s.config.Board.Expecting(request.Slug, request.Revision).PromoteTo(request.Slug, "", nil)
	return err
}

// changeStatus runs a release, draft or undraft.
// Each is judged against the entry at the revision the write then expects, so the rule and the write see one state.
func (s *Server) changeStatus(request writeRequest) error {
	view, entry, err := s.viewEntry(request.Slug)
	if err != nil {
		return err
	}
	if entry == nil {
		if slices.ContainsFunc(view.Projects, func(project boardengine.ViewProject) bool { return project.Slug == request.Slug }) {
			return notATask(request, boardengine.KindProject)
		}
		return &boardengine.StaleEntryError{Slug: request.Slug, Revision: request.Revision, Removed: true}
	}
	if entry.Revision != request.Revision {
		return &boardengine.StaleEntryError{Slug: request.Slug, Revision: request.Revision, Fresh: entry}
	}
	if entry.Kind != boardengine.KindTask {
		return notATask(request, entry.Kind)
	}

	var status *string
	held := boardengine.IsRunStatus(&entry.Status)
	switch request.Op {
	case opRelease:
		if held {
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
		case held:
			return fmt.Errorf("entry %q is held by a run (status %q): way forward: clear it with lym board set-status", request.Slug, entry.Status)
		default:
			return fmt.Errorf("undraft takes a draft task; entry %q has status %q: way forward: release it with the page's release", request.Slug, entry.Status)
		}
	}
	_, err = s.config.Board.Expecting(request.Slug, request.Revision).SetStatusReleasingProject(request.Slug, status)
	return err
}

// notATask is the refusal of a status operation on an entry of another kind.
func notATask(request writeRequest, kind string) error {
	return fmt.Errorf("entry %q is a %s, and the page's %s takes a task: way forward: set a note's or project's status with lym board set-status", request.Slug, kind, request.Op)
}
