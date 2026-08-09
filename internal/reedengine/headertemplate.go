// headertemplate.go embeds the default header-pane text template asset.
// The asset is named header-template.md (not template.yaml, the config-template convention):
// prompt/text assets ship as embedded `*-template.md` files rendered via internal/stencil rather
// than as Go string literals or parsed as YAML.

package reedengine

import _ "embed"

//go:embed header-template.md
var headerTemplate []byte

// HeaderTemplate returns the embedded default header-pane text template's raw bytes.
func HeaderTemplate() []byte {
	return headerTemplate
}
