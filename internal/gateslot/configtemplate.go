// configtemplate.go embeds the default gate.yaml template from template.yaml at build time.
//
// ConfigTemplate exposes that template to the gate config module.

package gateslot

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for the gate config module.
func ConfigTemplate() string {
	return configTemplate
}
