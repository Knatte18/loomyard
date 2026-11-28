// template.go — language.yaml template accessor.
//
// The file embeds template.yaml at build time and serves it as the default language.yaml template.

package languageconfig

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default language.yaml template: an empty skills list and an empty rename_tools list.
func ConfigTemplate() string {
	return configTemplate
}
