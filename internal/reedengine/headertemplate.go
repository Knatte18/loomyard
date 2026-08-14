// headertemplate.go embeds the default header-pane text template asset, console-header.md.
// The asset is rendered via tokenvocab.Render (internal/tokenvocab/render.go:12),
// which is itself a thin wrapper over stencil.Fill.
// This asset is deliberately outside the stencil mechanism: it is a tmux pane display banner,
// not a producer prompt, so it stays embedded here and is never seeded, stamped, or read from the hub's stencils directory.

package reedengine

import _ "embed"

//go:embed console-header.md
var headerTemplate []byte

// HeaderTemplate returns the embedded default header-pane text template's raw bytes.
func HeaderTemplate() []byte {
	return headerTemplate
}
