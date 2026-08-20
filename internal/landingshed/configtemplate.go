// configtemplate.go — landing.yaml template accessor.
//
// ConfigTemplate provides the default YAML template for landing's config module, embedded directly
// from template.yaml at build time, mirroring loomengine's embed-and-accessor pattern.

package landingshed

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for landing's config module.
func ConfigTemplate() string {
	return configTemplate
}
