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
// module: the discussion and plan role model-specs and the
// discussion_timeout_min / plan_timeout_min knobs the discussion and
// plan producers consult.
func ConfigTemplate() string {
	return configTemplate
}
