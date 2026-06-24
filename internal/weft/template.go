// template.go — weft.yaml template accessor.
//
// Provides the default YAML template for weft configuration. The template
// itself lives in the dependency-free internal/configtmpl leaf package; this
// accessor delegates to it so that configreg can build its module registry
// without importing the weft package. Weft templates use literal values
// (no env-marker substitution).

package weft

import "github.com/Knatte18/loomyard/internal/configtmpl"

// ConfigTemplate returns the default YAML template for weft configuration.
// The template uses literal values that are passed through unchanged by
// yamlengine.Resolve (no ${env:...} markers are present).
func ConfigTemplate() string {
	return configtmpl.Weft()
}
