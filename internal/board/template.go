// template.go — board.yaml template generator.
//
// Provides the default YAML template for board configuration via embedded
// template.yaml file with environment variable resolution.

package board

import _ "embed"

// configTemplate is the embedded YAML template for board configuration.
// It contains environment variable placeholders with defaults.
//
//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for board configuration.
// The template uses ${env:VAR:-default} syntax for configuration values,
// allowing environment-based overrides while preserving defaults when not set.
func ConfigTemplate() string {
	return configTemplate
}
