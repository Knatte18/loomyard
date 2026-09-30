// template.go — orch.yaml template accessor.
//
// Provides the default YAML template for orch configuration, embedded directly from
// template.yaml at build time.

package orchengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for orch configuration.
// Every key is a plain literal; model and effort are empty, meaning the provider default.
func ConfigTemplate() string {
	return configTemplate
}
