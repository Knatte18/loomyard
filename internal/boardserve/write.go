// write.go applies the six writes the viewer page sends: create, edit, release, promote, draft and undraft.
// Each goes through one exported method of the board, so the board's own checks all run.

package boardserve

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// The ops of a write request.
const (
	opCreate  = "create"
	opEdit    = "edit"
	opRelease = "release"
	opPromote = "promote"
	opDraft   = "draft"
	opUndraft = "undraft"
)

// writeOps lists the ops in the order refusals name them.
var writeOps = []string{opCreate, opEdit, opRelease, opPromote, opDraft, opUndraft}

// opFields is the fields each op may carry; an op absent here takes none.
var opFields = map[string][]string{
	opCreate: {"kind", "title", "labels", "brief", "body", "project", "depends_on"},
	opEdit:   {"title", "labels", "brief", "body", "project", "depends_on"},
}

// writeRequest is the body of POST /api/write.
// Revision is the entry revision the page read, empty for a create.
type writeRequest struct {
	Op       string         `json:"op"`
	Slug     string         `json:"slug"`
	Revision string         `json:"revision"`
	Fields   map[string]any `json:"fields"`
}

// requestKeys are the keys a write request may carry.
var requestKeys = []string{"op", "slug", "revision", "fields"}

// decodeWriteRequest decodes body, one JSON object, into request, and refuses a key the request does not define.
func decodeWriteRequest(body []byte, request *writeRequest) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		return err
	}
	for key := range keys {
		if !slices.Contains(requestKeys, key) {
			return fmt.Errorf("key %q is not part of a write request: way forward: send only %s", key, strings.Join(requestKeys, ", "))
		}
	}
	return json.Unmarshal(body, request)
}

// staleView is a stale refusal the server derives from its own view of the board, for the ops whose rule reads the entry.
// It answers like the board's own *boardengine.StaleEntryError.
type staleView struct {
	message string
	fresh   *boardengine.ViewEntry
}

// Error returns the refusal's message.
func (e *staleView) Error() string { return e.message }

// staleEntry reports whether err is a stale refusal, and returns the fresh entry it carries, nil when the entry is gone.
func staleEntry(err error) (*boardengine.ViewEntry, bool) {
	var fromBoard *boardengine.StaleEntryError
	if errors.As(err, &fromBoard) {
		return fromBoard.Fresh, true
	}
	var fromView *staleView
	if errors.As(err, &fromView) {
		return fromView.fresh, true
	}
	return nil, false
}

// entryOf returns the entry slug of view and whether the view holds it.
func entryOf(view boardengine.BoardView, slug string) (boardengine.ViewEntry, bool) {
	index := slices.IndexFunc(view.Entries, func(e boardengine.ViewEntry) bool { return e.Slug == slug })
	if index < 0 {
		return boardengine.ViewEntry{}, false
	}
	return view.Entries[index], true
}

// checkRequest refuses an op or field the page does not send, before any write.
// It also refuses a missing revision, and a revision on a create.
func checkRequest(request writeRequest) error {
	if !slices.Contains(writeOps, request.Op) {
		return fmt.Errorf("unknown op %q: way forward: send one of %s", request.Op, strings.Join(writeOps, ", "))
	}
	allowed := opFields[request.Op]
	for field := range request.Fields {
		if !slices.Contains(allowed, field) {
			list := "no fields"
			if len(allowed) > 0 {
				list = "only " + strings.Join(allowed, ", ")
			}
			return fmt.Errorf("field %q is not allowed for %s: way forward: send %s", field, request.Op, list)
		}
	}
	if request.Op != opCreate && request.Revision == "" {
		return fmt.Errorf("%s of entry %q carries no revision, so nothing was written: way forward: reload the board and act on what it holds now", request.Op, request.Slug)
	}
	if request.Op == opCreate && request.Revision != "" {
		return fmt.Errorf("create of entry %q carries a revision, so nothing was written: way forward: send a create with no revision", request.Slug)
	}
	return nil
}

// applyWrite runs the one board write of request.Op and returns the entry as the board holds it afterwards.
// The write runs on a board that expects request.Revision.
// Its write section refuses a stale revision under the write lock.
// A stale refusal is a *boardengine.StaleEntryError or a *staleView; any other error is the board's own refusal.
func (s *Server) applyWrite(request writeRequest) (boardengine.ViewEntry, error) {
	if err := checkRequest(request); err != nil {
		return boardengine.ViewEntry{}, err
	}
	expecting := s.cfg.Board.Expecting(request.Slug, request.Revision)
	var err error
	switch request.Op {
	case opCreate, opEdit:
		fields := map[string]any{"slug": request.Slug}
		for key, value := range request.Fields {
			fields[key] = value
		}
		_, err = expecting.UpsertTaskReporting(fields)
	case opPromote:
		err = s.promote(expecting, request)
	default:
		err = s.setStatus(expecting, request)
	}
	if err != nil {
		return boardengine.ViewEntry{}, err
	}
	view, err := s.cfg.Board.View()
	if err != nil {
		return boardengine.ViewEntry{}, fmt.Errorf("read the board after the write: %w", err)
	}
	entry, found := entryOf(view, request.Slug)
	if !found {
		return boardengine.ViewEntry{}, fmt.Errorf("entry %q is not on the board after the write", request.Slug)
	}
	return entry, nil
}

// promote makes the note request.Slug a task.
// A task is refused before the write; the write section decides a stale revision and refuses a project.
func (s *Server) promote(expecting *boardengine.Board, request writeRequest) error {
	view, err := s.cfg.Board.View()
	if err != nil {
		return fmt.Errorf("read the board: %w", err)
	}
	if entry, found := entryOf(view, request.Slug); found && entry.Kind == boardengine.KindTask {
		return fmt.Errorf("entry %q is a task already, and only a note is promoted: way forward: edit it, or promote a note", request.Slug)
	}
	_, err = expecting.PromoteTo(request.Slug, "", nil)
	return err
}

// setStatus runs release, draft or undraft of the task request.Slug.
// Their rule reads the entry's kind and status, so it reads them from the current view,
// and the write section re-checks the revision of the entry the rule judged.
func (s *Server) setStatus(expecting *boardengine.Board, request writeRequest) error {
	view, err := s.cfg.Board.View()
	if err != nil {
		return fmt.Errorf("read the board: %w", err)
	}
	entry, found := entryOf(view, request.Slug)
	if !found {
		return &staleView{message: fmt.Sprintf("entry %q was removed since the page read it, so nothing was written: way forward: reload the board and act on what it holds now", request.Slug)}
	}
	if entry.Revision != request.Revision {
		return &staleView{
			message: fmt.Sprintf("entry %q changed since the page read it, so nothing was written: way forward: compare the fresh entry and send the edit again", request.Slug),
			fresh:   &entry,
		}
	}
	if entry.Kind != boardengine.KindTask {
		return fmt.Errorf("entry %q is a %s, and the page's %s takes a task: way forward: set a note's or project's status with lym board set-status", request.Slug, entry.Kind, request.Op)
	}
	status := entry.Status
	held := boardengine.IsRunStatus(&status)

	var target *string
	switch request.Op {
	case opRelease:
		if held {
			return fmt.Errorf("entry %q is held by a run (status %q), and a release from the page would let its parent's dobby start it while the run holds its pair: way forward: release it with lym board set-status", request.Slug, status)
		}
	case opDraft:
		if status != "" {
			return fmt.Errorf("draft takes a task with no status; entry %q has status %q: way forward: release it first, or leave it", request.Slug, status)
		}
		draft := boardengine.StatusDraft
		target = &draft
	case opUndraft:
		switch {
		case status == "":
			return fmt.Errorf("entry %q has no status, so it is released already and nothing was written", request.Slug)
		case held:
			return fmt.Errorf("entry %q is held by a run (status %q): way forward: clear it with lym board set-status", request.Slug, status)
		case status != boardengine.StatusDraft:
			return fmt.Errorf("undraft takes a draft task; entry %q has status %q: way forward: release it with the page's release", request.Slug, status)
		}
	}
	_, err = expecting.SetStatusReleasingProject(request.Slug, target)
	return err
}
