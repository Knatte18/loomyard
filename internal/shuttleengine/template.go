// template.go provides the default shuttle.yaml template, embedded from template.yaml at build time.

package shuttleengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for shuttle configuration.
// The template uses ${env:VAR:-default} syntax for run_dir.
// It allows an environment-based override and keeps the default when the variable is not set.
// The other keys are plain literals.
func ConfigTemplate() string {
	return configTemplate
}
