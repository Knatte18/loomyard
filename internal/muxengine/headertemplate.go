// headertemplate.go embeds the default header-pane text template asset. The
// asset is named header-template.md (not template.yaml, the config-template
// convention) per the builderengine *-template.md precedent for prompt/text
// assets rendered via internal/stencil rather than parsed as YAML.

package muxengine

import _ "embed"

//go:embed header-template.md
var headerTemplate []byte

// HeaderTemplate returns the embedded default header-pane text template's raw
// bytes: a leading `<!-- ... -->` banner comment (stripped by stencil.Fill)
// followed by the one-line body `hub: {{.hub}}`. Engine.HeaderText renders
// this template via tokenvocab.Render whenever Config.Header.Template is
// empty.
func HeaderTemplate() []byte {
	return headerTemplate
}
