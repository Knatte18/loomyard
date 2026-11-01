// seam.go implements fabric's worktree seam: the file targets are rendered when a worktree is made, and re-rendered when a take-in changes a path they are built from.

package heddleengine

import (
	"slices"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
)

// Seam renders heddle's file targets on the events fabric fires for a worktree.
// A change to the store itself is never an event, so a Bolt pull re-renders nothing and the next take-in or a hand render takes the store as it then stands.
type Seam struct {
	Renderer Renderer
}

var _ fabricengine.WorktreeSeam = Seam{}

// Made renders every file target into the event's root.
func (s Seam) Made(ev fabricengine.WorktreeEvent) error {
	return s.render(requestOf(ev, nil))
}

// TookIn renders the file targets whose re-render paths include one of the event's paths.
// With none selected it renders nothing and takes no lock.
func (s Seam) TookIn(ev fabricengine.WorktreeEvent) error {
	var selected []string
	for _, target := range targetTable {
		if target.Delivery != DeliveryFile {
			continue
		}
		if slices.ContainsFunc(ev.Paths, func(path string) bool { return slices.Contains(target.ReRender, path) }) {
			selected = append(selected, target.Name)
		}
	}
	if len(selected) == 0 {
		return nil
	}
	return s.render(requestOf(ev, selected))
}

// render runs the render and logs each of its warnings at Warn, naming the worktree.
func (s Seam) render(req RenderRequest) error {
	report, err := s.Renderer.Render(req)
	for _, warning := range report.Warnings {
		logger.Warn("heddleengine: render warning (non-fatal)", "worktree", req.Root, "warning", warning)
	}
	return err
}

// requestOf copies the event's root, scratch directory, Bolt checkout and exclude list into a render request for the targets only names.
func requestOf(ev fabricengine.WorktreeEvent, only []string) RenderRequest {
	return RenderRequest{Root: ev.Root, ScratchDir: ev.ScratchDir, BoltDir: ev.BoltDir, Excluded: ev.Excluded, Only: only}
}
