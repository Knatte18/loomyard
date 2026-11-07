// configtemplate.go declares ConfigTemplate, the accessor for the landing.yaml template embedded from template.yaml.

package landingshed

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default landing.yaml template, embedded in the binary at build time.
func ConfigTemplate() string {
	return configTemplate
}
