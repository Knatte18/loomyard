// header.go implements Engine.HeaderText and Engine.ValidateHeader: the
// header pane's text-rendering pipeline over internal/tokenvocab, and the
// eager, loud validation hook the boot path (batch 4) runs before the
// session comes up.

package muxengine

import "github.com/Knatte18/loomyard/internal/tokenvocab"

// HeaderText renders this hub's header-pane text: the configured
// cfg.Header.Template when non-empty, otherwise the embedded default
// (HeaderTemplate). It builds a tokenvocab.Ctx from e.layout and delegates to
// tokenvocab.Render, surfacing any stencil unfilled-top-level-marker error
// unchanged so a bad template is a loud, early failure rather than a
// silently blank pane. HeaderText reads only e.cfg/e.layout — no lock, no
// tmux call — so it is safe to call before the session boots.
func (e *Engine) HeaderText() (string, error) {
	template := []byte(e.cfg.Header.Template)
	if len(template) == 0 {
		template = HeaderTemplate()
	}

	ctx := tokenvocab.Ctx{Layout: e.layout}
	rendered, err := tokenvocab.Render(template, ctx)
	if err != nil {
		return "", err
	}
	return string(rendered), nil
}

// ValidateHeader reports whether this hub's configured header template
// renders cleanly, discarding the rendered text. It is the eager validation
// hook the boot path runs at up/config-load time so a bad template or an
// unresolvable token surfaces loudly, before the session boots, rather than
// only when the header pane itself is spawned.
func (e *Engine) ValidateHeader() error {
	_, err := e.HeaderText()
	return err
}
