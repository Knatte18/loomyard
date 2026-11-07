// template.go — board.yaml template accessor.
//
// It provides the default YAML template for board configuration.
// The template is embedded directly from template.yaml at build time.

package boardengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for board configuration.
// The template uses ${env:VAR:-default} syntax for overrides.
func ConfigTemplate() string {
	return configTemplate
}
