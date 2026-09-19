// statusline.go implements Engine.StatusLineText and Engine.ValidateStatusLine: the status-line's
// text-rendering pipeline over internal/tokenvocab,
// and the eager, loud validation hook the boot path (batch 4) runs before the session comes up.
// ValidateStatusLine is now more load-bearing rather than less: a template that fails to render
// would otherwise reach "set-option status-left", where every failure is non-fatal and merely
// logged.

package reedengine

import "github.com/Knatte18/loomyard/internal/tokenvocab"

// StatusLineText renders this hub's tmux status-line text.
func (e *Engine) StatusLineText() (string, error) {
	template := []byte(e.cfg.StatusLine.Template)
	if len(template) == 0 {
		template = StatusLineTemplate()
	}

	ctx := tokenvocab.Ctx{RepoName: e.geom.RepoName, HubPath: e.geom.HubPath, WorktreeName: e.geom.WorktreeName}
	rendered, err := tokenvocab.Render(template, ctx)
	if err != nil {
		return "", err
	}
	return string(rendered), nil
}

// ValidateStatusLine reports whether this hub's configured status-line template renders cleanly.
func (e *Engine) ValidateStatusLine() error {
	_, err := e.StatusLineText()
	return err
}
