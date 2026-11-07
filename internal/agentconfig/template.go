// template.go embeds template.yaml and serves it as the default agents.yaml.

package agentconfig

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default agents.yaml: the effort table and every role's skill list.
func ConfigTemplate() string {
	return configTemplate
}
