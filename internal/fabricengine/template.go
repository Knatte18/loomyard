// template.go — the default fabric.yaml template, embedded from template.yaml at build time.

package fabricengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for fabric configuration.
// The template uses ${env:VAR:-default} syntax for environment-based overrides.
func ConfigTemplate() string {
	return configTemplate
}
