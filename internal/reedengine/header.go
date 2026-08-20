// header.go implements Engine.HeaderText and Engine.ValidateHeader: the header pane's
// text-rendering pipeline over internal/tokenvocab,
// and the eager, loud validation hook the boot path (batch 4) runs before the session comes up.

package reedengine

import "github.com/Knatte18/loomyard/internal/tokenvocab"

// HeaderText renders this hub's header-pane text.
func (e *Engine) HeaderText() (string, error) {
	template := []byte(e.cfg.Header.Template)
	if len(template) == 0 {
		template = HeaderTemplate()
	}

	ctx := tokenvocab.Ctx{RepoName: e.geom.RepoName, HubPath: e.geom.HubPath}
	rendered, err := tokenvocab.Render(template, ctx)
	if err != nil {
		return "", err
	}
	return string(rendered), nil
}

// ValidateHeader reports whether this hub's configured header template renders cleanly.
func (e *Engine) ValidateHeader() error {
	_, err := e.HeaderText()
	return err
}
