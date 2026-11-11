// template.go provides the default orch.yaml template.
//
// The template is embedded from template.yaml at build time.

package orchengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for orch configuration.
// Every key is a plain literal.
// Model and effort are set, so the orch session never falls back to the provider default of the machine it starts on.
func ConfigTemplate() string {
	return configTemplate
}
