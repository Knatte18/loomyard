// template.go — models.yaml seed template accessor.
// Mirrors shuttleengine's embed pattern: the seed content lives in template.yaml and is embedded verbatim at build time, with ConfigTemplate as the sole accessor configreg consumes for materialization.

package modelspec

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the seed content for models.yaml: the live-keys registry (sonnet/opus/haiku/fable with effort defaults) that configreg materializes when models.yaml is absent.
func ConfigTemplate() string {
	return configTemplate
}
