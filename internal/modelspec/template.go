// template.go holds the models.yaml seed template, embedded verbatim from template.yaml at build time.
// ConfigTemplate is the accessor configreg uses to materialize that seed.

package modelspec

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the seed content for models.yaml.
// It is the live-keys registry with its effort defaults, which configreg materializes when models.yaml is absent.
func ConfigTemplate() string {
	return configTemplate
}
