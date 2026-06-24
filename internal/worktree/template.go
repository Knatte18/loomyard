// template.go — worktree.yaml template accessor.
//
// Provides the default YAML template for worktree configuration. The template
// itself lives in the dependency-free internal/configtmpl leaf package; this
// accessor delegates to it so that configreg can build its module registry
// without importing the worktree package.

package worktree

import "github.com/Knatte18/loomyard/internal/configtmpl"

// ConfigTemplate returns the default YAML template for worktree configuration.
// The template uses ${env:VAR:-default} syntax for configuration values,
// allowing environment-based overrides while preserving defaults when not set.
func ConfigTemplate() string {
	return configtmpl.Worktree()
}
