// write.go turns one write of the viewer page into the one board write it stands for.

package boardserve

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// writeRequest is the body of one write.
type writeRequest struct {
	Op   string `json:"op"`
	Slug string `json:"slug"`
	// Revision is the revision of the entry the page read, which a create does not send.
	Revision string         `json:"revision"`
	Fields   map[string]any `json:"fields"`
}

// writeKeys are the keys the body of a write may hold.
var writeKeys = []string{"op", "slug", "revision", "fields"}

// decodeWriteRequest decodes the body of one write.
// It refuses an unknown key, since a write that dropped part of what the page sent would pass for saved.
func decodeWriteRequest(body []byte) (writeRequest, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		return writeRequest{}, err
	}
	for key := range keys {
		if !slices.Contains(writeKeys, key) {
			return writeRequest{}, fmt.Errorf("unknown key %q", key)
		}
	}
	var request writeRequest
	err := json.Unmarshal(body, &request)
	return request, err
}

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

// opFields are the fields each op accepts; an op absent here takes none.
var opFields = map[string][]string{
	opCreate: {"kind", "title", "labels", "brief", "body", "project", "depends_on"},
	opEdit:   {"title", "labels", "brief", "body", "project", "depends_on"},
}

// applyWrite runs the board write request stands for, and returns the entry as the board then holds it.
// The entry is nil when the written slug is neither a task nor a note.
// A request whose entry moved on returns a *boardengine.StaleEntryError, and nothing is written.
func (s *Server) applyWrite(request writeRequest) (*boardengine.ViewEntry, error) {
	if err := checkRequest(request); err != nil {
		return nil, err
	}
	if err := s.runWrite(request); err != nil {
		return nil, err
	}
	view, err := s.config.Board.View()
	if err != nil {
		return nil, err
	}
	return viewedEntry(view, request.Slug), nil
}

// checkRequest refuses a request that is malformed on its face, before the board is read.
func checkRequest(request writeRequest) error {
	if !slices.Contains(writeOps, request.Op) {
		return fmt.Errorf("unknown op %q: way forward: send one of %s", request.Op, strings.Join(writeOps, ", "))
	}
	allowed := opFields[request.Op]
	for field := range request.Fields {
		if slices.Contains(allowed, field) {
			continue
		}
		if len(allowed) == 0 {
			return fmt.Errorf("%s takes no field %q: way forward: send it without fields", request.Op, field)
		}
		return fmt.Errorf("%s takes no field %q: way forward: send only %s", request.Op, field, strings.Join(allowed, ", "))
	}
	if request.Op != opCreate && request.Revision == "" {
		return fmt.Errorf("%s of entry %q carries no revision, so nothing was written: way forward: reload the board and act on what it holds now", request.Op, request.Slug)
	}
	return nil
}

// runWrite makes the board write of a checked request.
func (s *Server) runWrite(request writeRequest) error {
	slug := request.Slug
	fields := map[string]any{"slug": slug}
	for name, value := range request.Fields {
		fields[name] = value
	}
	expecting := s.config.Board.Expecting(slug, request.Revision)

	switch request.Op {
	case opCreate:
		view, err := s.config.Board.View()
		if err != nil {
			return err
		}
		if entry := viewedEntry(view, slug); entry != nil {
			return &boardengine.StaleEntryError{Slug: slug, Fresh: entry}
		}
		_, err = s.config.Board.Expecting(slug, "").UpsertTaskReporting(fields)
		return err
	case opEdit:
		_, err := expecting.UpsertTaskReporting(fields)
		return err
	case opPromote:
		view, err := s.config.Board.View()
		if err != nil {
			return err
		}
		if entry := viewedEntry(view, slug); entry != nil && entry.Kind == boardengine.KindTask {
			return fmt.Errorf("entry %q is a task already, and only a note is promoted: way forward: edit it, or promote a note", slug)
		}
		_, err = expecting.PromoteTo(slug, "", nil)
		return err
	}

	// The other ops judge the entry's kind and status, read at the revision the write section then re-checks.
	status, err := s.taskStatus(request)
	if err != nil {
		return err
	}
	var next *string
	switch request.Op {
	case opRelease:
		if boardengine.IsRunStatus(&status) {
			return fmt.Errorf("entry %q is held by a run (status %q), and a release from the page would let its parent's dobby start it while the run holds its pair: way forward: release it with lym board set-status", slug, status)
		}
	case opDraft:
		if status != "" {
			return fmt.Errorf("draft takes a task with no status; entry %q has status %q: way forward: release it first, or leave it", slug, status)
		}
		draft := boardengine.StatusDraft
		next = &draft
	case opUndraft:
		switch {
		case status == "":
			return fmt.Errorf("entry %q has no status, so it is released already and nothing was written", slug)
		case boardengine.IsRunStatus(&status):
			return fmt.Errorf("entry %q is held by a run (status %q): way forward: clear it with lym board set-status", slug, status)
		case status != boardengine.StatusDraft:
			return fmt.Errorf("undraft takes a draft task; entry %q has status %q: way forward: release it with the page's release", slug, status)
		}
	}
	_, err = expecting.SetStatusReleasingProject(slug, next)
	return err
}

// taskStatus returns the status of the task the request names, as the page read it.
// It refuses a request whose entry is gone or moved on, and one whose entry is no task.
func (s *Server) taskStatus(request writeRequest) (string, error) {
	view, err := s.config.Board.View()
	if err != nil {
		return "", err
	}
	notTask := func(kind string) error {
		return fmt.Errorf("entry %q is a %s, and the page's %s takes a task: way forward: set a note's or project's status with lym board set-status", request.Slug, kind, request.Op)
	}
	entry := viewedEntry(view, request.Slug)
	if entry == nil {
		if slices.ContainsFunc(view.Projects, func(p boardengine.ViewProject) bool { return p.Slug == request.Slug }) {
			return "", notTask(boardengine.KindProject)
		}
		return "", &boardengine.StaleEntryError{Slug: request.Slug, Expected: request.Revision, Removed: true}
	}
	if entry.Revision != request.Revision {
		return "", &boardengine.StaleEntryError{Slug: request.Slug, Expected: request.Revision, Fresh: entry}
	}
	if entry.Kind != boardengine.KindTask {
		return "", notTask(entry.Kind)
	}
	return entry.Status, nil
}

// viewedEntry returns the task or note of view with slug, nil when it holds none.
func viewedEntry(view boardengine.BoardView, slug string) *boardengine.ViewEntry {
	i := slices.IndexFunc(view.Entries, func(e boardengine.ViewEntry) bool { return e.Slug == slug })
	if i < 0 {
		return nil
	}
	return &view.Entries[i]
}
