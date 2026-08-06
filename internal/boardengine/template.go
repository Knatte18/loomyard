// template.go — board.yaml template accessor.
//
// Provides the default YAML template for board configuration, embedded directly from template.yaml
// at build time.
// The template uses ${env:VAR:-default} syntax for environment-based overrides.

package boardengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for board configuration, with
// ${env:VAR:-default} syntax for overrides.
func ConfigTemplate() string {
	return configTemplate
}
