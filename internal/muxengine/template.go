// template.go — mux.yaml template accessor.
//
// Provides the default YAML template for mux configuration. The template
// itself is embedded by GOOS-selected files (template_windows.go,
// template_posix.go) into the package-level configTemplate var; this file
// keeps only the untagged accessor so callers never need to know which
// build tag supplied it.

package muxengine

// ConfigTemplate returns the default YAML template for mux configuration.
// The template uses ${env:VAR:-default} syntax for the machine tool paths
// (psmux/pwsh/claude) and debug_log, allowing environment-based overrides
// while preserving defaults when not set; the layout-tuning keys (width,
// height, collapsed_strip_rows, min_full_rows, strand_name) are plain
// literals. On Windows the psmux/pwsh defaults are the machine's
// pinned psmux.exe/pwsh.exe paths; on every other GOOS they are the
// PATH-resolved POSIX names tmux/bash (see template_windows.go /
// template_posix.go for which embedded YAML backs configTemplate on a
// given GOOS).
func ConfigTemplate() string {
	return configTemplate
}
