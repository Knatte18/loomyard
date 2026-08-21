// configtemplate.go — loom.yaml template accessor.
//
// ConfigTemplate provides the default YAML template for loom's config module, embedded directly
// from template.yaml at build time, mirroring reedengine's embed-and-accessor
// pattern.

package loomengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for loom's config module.
func ConfigTemplate() string {
	return configTemplate
}
