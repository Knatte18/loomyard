// configtemplate.go — gate.yaml template accessor.
//
// ConfigTemplate provides the default YAML template for the gate config module, embedded directly from template.yaml at build time.

package gateslot

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for the gate config module.
func ConfigTemplate() string {
	return configTemplate
}
