// template.go holds the untagged accessor for the reed.yaml template.
// The text is embedded into configTemplate by template_posix.go or template_windows.go, depending on GOOS;
// callers use ConfigTemplate and need not know which file supplied it.

package reedengine

// ConfigTemplate returns the default reed.yaml template text.
// Keys written as ${env:VAR:-default} take an environment override and keep their default when it is unset:
// the tmux and shell tool paths, debug_log, mouse, watchdog and tmux_conf.
// The session size keys (width, height), the segment_colors block and the buttons block are plain literals.
// The template names no provider tool: reed stays provider-invariant per the Shuttle Provider-Seam Invariant,
// so a claude path belongs in shuttle's template.
// On Windows the tmux and shell defaults are tmux and pwsh, resolved through PATH;
// on every other GOOS they are tmux and bash.
func ConfigTemplate() string {
	return configTemplate
}
