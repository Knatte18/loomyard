// template.go — warp.yaml template accessor.
//
// Provides the default YAML template for warp configuration, embedded
// directly from template.yaml at build time. The template uses
// ${env:VAR:-default} syntax for environment-based overrides.

package warp

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for warp configuration.
// The template uses ${env:VAR:-default} syntax for configuration values,
// allowing environment-based overrides while preserving defaults when not set.
func ConfigTemplate() string {
	return configTemplate
}
