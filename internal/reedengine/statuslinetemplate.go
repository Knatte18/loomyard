// statuslinetemplate.go embeds the default status-line text template asset, status-line.md.
// The asset is rendered via tokenvocab.Render (internal/tokenvocab/render.go:12),
// which is itself a thin wrapper over stencil.Fill.
// This asset is deliberately outside the stencil mechanism: it is a tmux status-line display
// banner, not a producer prompt, so it stays embedded here and is never seeded, stamped, or read
// from the hub's stencils directory.

package reedengine

import _ "embed"

//go:embed status-line.md
var statusLineTemplate []byte

// StatusLineTemplate returns the embedded default status-line text template's raw bytes.
func StatusLineTemplate() []byte {
	return statusLineTemplate
}
