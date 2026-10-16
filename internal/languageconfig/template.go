// template.go — language.yaml template accessor.
//
// ConfigTemplate provides the default YAML template for the hub's language skills, embedded directly from template.yaml at build time, matching loggerconfig's embed-and-accessor pattern.

package languageconfig

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for language configuration: an empty skills list and an empty verify_tiers list.
func ConfigTemplate() string {
	return configTemplate
}
