// configtemplate.go — loom.yaml template accessor.
//
// ConfigTemplate provides the default YAML template for loom's config
// module, embedded directly from template.yaml at build time, mirroring
// builderengine's and perchengine's embed-and-accessor pattern.

package loomengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for loom's config
// module: the discussion role model-spec and the discussion_timeout_min
// knob the discussion producer consults.
func ConfigTemplate() string {
	return configTemplate
}
