// template.go — mux.yaml template accessor.
//
// Provides the default YAML template for mux configuration, embedded
// directly from template.yaml at build time. The template uses
// ${env:VAR:-default} syntax for environment-based overrides, mirroring
// warpengine's config pattern.

package muxengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for mux configuration.
// The template uses ${env:VAR:-default} syntax for the machine tool paths
// (psmux/pwsh/claude), allowing environment-based overrides while
// preserving defaults when not set; the layout-tuning keys (width, height,
// collapsed_strip_rows, top_band_rows, min_full_rows, strand_name) are
// plain literals.
func ConfigTemplate() string {
	return configTemplate
}
