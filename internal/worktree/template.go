// template.go — worktree.yaml template generator.
//
// Provides the default YAML template for worktree configuration via embedded
// template.yaml file with environment variable resolution.

package worktree

import _ "embed"

// configTemplate is the embedded YAML template for worktree configuration.
// It contains an environment variable placeholder with an empty default.
//
//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for worktree configuration.
// The template uses ${env:VAR:-default} syntax for configuration values,
// allowing environment-based overrides while preserving defaults when not set.
func ConfigTemplate() string {
	return configTemplate
}
