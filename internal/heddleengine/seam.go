// seam.go implements fabric's worktree seam for heddle.
// The file targets are rendered when a worktree is made.
// They are re-rendered when a take-in changes a path they are built from.

package heddleengine

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
)

// Seam renders heddle's file targets on the events fabric fires for a worktree.
// A change to the store itself is never an event.
// A Bolt pull therefore re-renders nothing, and the next take-in or a hand render takes the store as it then stands.
type Seam struct {
	Renderer Renderer
}

var _ fabricengine.WorktreeSeam = Seam{}

// Made renders every file target into the event's anchor.
func (s Seam) Made(ev fabricengine.WorktreeEvent) error {
	return s.render(requestOf(ev, nil))
}

// TookIn renders the file targets whose re-render paths match one of the event's root-relative paths.
// Each re-render path is taken under the event's anchor.
// With none selected it renders nothing and takes no lock.
func (s Seam) TookIn(ev fabricengine.WorktreeEvent) error {
	anchorRel, err := filepath.Rel(ev.Root, ev.Anchor)
	if err != nil {
		return fmt.Errorf("place the anchor %s within the worktree %s: %w", ev.Anchor, ev.Root, err)
	}
	anchorPrefix := filepath.ToSlash(anchorRel)
	var selected []string
	for _, target := range targetTable {
		if target.Delivery != DeliveryFile {
			continue
		}
		if slices.ContainsFunc(target.ReRender, func(rel string) bool { return slices.Contains(ev.Paths, path.Join(anchorPrefix, rel)) }) {
			selected = append(selected, target.Name)
		}
	}
	if len(selected) == 0 {
		return nil
	}
	return s.render(requestOf(ev, selected))
}

// render runs the render and logs each of its warnings at Warn, naming the anchor.
func (s Seam) render(req RenderRequest) error {
	report, err := s.Renderer.Render(req)
	for _, warning := range report.Warnings {
		logger.Warn("heddleengine: render warning (non-fatal)", "anchor", req.Anchor, "warning", warning)
	}
	return err
}

// requestOf copies the event's anchor, scratch directory, Bolt checkout and exclude list into a render request.
// The only list names the targets to render, and nil renders every target.
func requestOf(ev fabricengine.WorktreeEvent, only []string) RenderRequest {
	return RenderRequest{Anchor: ev.Anchor, ScratchDir: ev.ScratchDir, BoltDir: ev.BoltDir, Excluded: ev.Excluded, Only: only}
}
