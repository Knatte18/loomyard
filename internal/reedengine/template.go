// template.go — reed.yaml template accessor.
//
// Provides the default YAML template for reed configuration.
// The template itself is embedded by GOOS-selected files (template_windows.go, template_posix.go)
// into the package-level configTemplate var;
// this file keeps only the untagged accessor so callers never need to know which build tag supplied
// it.

package reedengine

// ConfigTemplate returns the default YAML template for reed configuration.
// Exactly five keys use the ${env:VAR:-default} syntax, allowing environment-based overrides while
// preserving defaults when not set: the two machine tool paths (tmux, shell) plus debug_log, mouse,
// and watchdog.
// The layout-tuning keys (width, height, collapsed_strip_rows, min_full_rows, strand_name) and the
// header block are plain literals.
// No provider tool is named here: reed stays provider-invariant per the Shuttle Provider-Seam
// Invariant, so a claude path belongs to shuttle's template, never this one.
// On Windows the tmux/shell defaults are the machine's pinned psmux.exe/pwsh.exe paths;
// on every other GOOS they are the PATH-resolved POSIX names tmux/bash (see template_windows.go /
// template_posix.go for which embedded YAML backs configTemplate on a given GOOS).
func ConfigTemplate() string {
	return configTemplate
}
